declare const require: any;
declare const process: any;
declare const Buffer: any;

const http = require("http");
const fs = require("fs");
const path = require("path");

type EffectResource = {
  id: string;
  requestId?: string;
  kind: string;
  payload: unknown;
  createdAt: string;
};

type EffectCommit = {
  id: string;
  operationId: string;
  marker?: string;
  source?: string;
  payload: unknown;
  createdAt: string;
};

type AuthorityToken = {
  token: string;
  scope: string;
  subject: string;
  consumed: boolean;
  consumedBy?: string;
  consumedAt?: string;
  issuedAt: string;
};

type State = {
  effects: {
    resources: EffectResource[];
    events: unknown[];
    commits: EffectCommit[];
  };
  authority: {
    tokens: AuthorityToken[];
  };
};

const port = Number(process.env.SYNCFUZZ_MOCK_PORT || "8910");
const dbPath = process.env.SYNCFUZZ_MOCK_DB || path.join(process.cwd(), "syncfuzz-mock-state.json");

// This process represents state outside the agent sandbox. In real experiments,
// this would be a cloud API, approval service, issue tracker, CI system, etc.
let state: State = loadState();

const server = http.createServer(async (req: any, res: any) => {
  try {
    const url = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`);

    if (req.method === "GET" && url.pathname === "/health") {
      return send(res, 200, { ok: true, service: "syncfuzz-mock-servers" });
    }

    if (req.method === "POST" && url.pathname === "/reset") {
      state = emptyState();
      persistState();
      return send(res, 200, { ok: true });
    }

    if (req.method === "GET" && url.pathname === "/state") {
      return send(res, 200, state);
    }

    if (req.method === "POST" && url.pathname === "/effect/resources") {
      const body = await readBody(req);
      const requestId = asOptionalString(body.requestId);
      if (requestId) {
        const existing = state.effects.resources.find((resource) => resource.requestId === requestId);
        if (existing) {
          // Idempotency is scoped to requestId. If a replay generates a new
          // requestId, the service will commit a second external resource.
          return send(res, 200, { resource: existing, idempotentReplay: true });
        }
      }

      const resource: EffectResource = {
        id: `res_${state.effects.resources.length + 1}`,
        requestId,
        kind: asOptionalString(body.kind) || "generic",
        payload: body.payload ?? null,
        createdAt: new Date().toISOString()
      };
      state.effects.resources.push(resource);
      persistState();
      const operationId = operationIdFromPayload(resource.payload);
      const count = operationId
        ? state.effects.resources.filter((item) => operationIdFromPayload(item.payload) === operationId).length
        : state.effects.resources.length;
      return send(res, 201, { resource, idempotentReplay: false, count });
    }

    if (req.method === "POST" && url.pathname === "/effect/events") {
      const body = await readBody(req);
      state.effects.events.push({ ...body, receivedAt: new Date().toISOString() });
      persistState();
      return send(res, 201, { ok: true });
    }

    if (req.method === "POST" && url.pathname === "/effect/commits") {
      const body = await readBody(req);
      const operationId = asOptionalString(body.operationId) || asOptionalString(body.operation_id);
      if (!operationId) {
        return send(res, 400, { error: "missing_operation_id" });
      }

      const commit: EffectCommit = {
        id: `commit_${state.effects.commits.length + 1}`,
        operationId,
        marker: asOptionalString(body.marker),
        source: asOptionalString(body.source),
        payload: body.payload ?? null,
        createdAt: new Date().toISOString()
      };
      state.effects.commits.push(commit);
      persistState();
      const count = state.effects.commits.filter((item) => item.operationId === operationId).length;
      return send(res, 201, { commit, count });
    }

    if (req.method === "POST" && url.pathname === "/authority/tokens") {
      const body = await readBody(req);
      const token: AuthorityToken = {
        token: `tok_${state.authority.tokens.length + 1}_${Math.random().toString(16).slice(2)}`,
        scope: asOptionalString(body.scope) || "default",
        subject: asOptionalString(body.subject) || "agent",
        consumed: false,
        issuedAt: new Date().toISOString()
      };
      state.authority.tokens.push(token);
      persistState();
      return send(res, 201, { token });
    }

    if (req.method === "POST" && url.pathname === "/authority/consume") {
      const body = await readBody(req);
      const tokenValue = asOptionalString(body.token);
      const operation = asOptionalString(body.operation) || "unknown";
      const token = state.authority.tokens.find((candidate) => candidate.token === tokenValue);
      if (!token) {
        return send(res, 404, { error: "token_not_found" });
      }
      if (token.consumed) {
        // This negative response is security-relevant: it tells SyncFuzz that
        // agent state and authority state disagree about token freshness.
        return send(res, 409, { error: "token_already_consumed", token });
      }

      token.consumed = true;
      token.consumedBy = operation;
      token.consumedAt = new Date().toISOString();
      persistState();
      return send(res, 200, { token });
    }

    return send(res, 404, { error: "not_found", path: url.pathname });
  } catch (error) {
    return send(res, 500, { error: "internal_error", message: String(error) });
  }
});

server.listen(port, () => {
  console.log(`syncfuzz mock servers listening on http://127.0.0.1:${port}`);
  console.log(`state file: ${dbPath}`);
});

function emptyState(): State {
  return {
    effects: {
      resources: [],
      events: [],
      commits: []
    },
    authority: {
      tokens: []
    }
  };
}

function loadState(): State {
  try {
    if (!fs.existsSync(dbPath)) {
      return emptyState();
    }
    return normalizeState(JSON.parse(fs.readFileSync(dbPath, "utf8")));
  } catch {
    return emptyState();
  }
}

function normalizeState(value: any): State {
  const empty = emptyState();
  return {
    effects: {
      resources: Array.isArray(value?.effects?.resources) ? value.effects.resources : empty.effects.resources,
      events: Array.isArray(value?.effects?.events) ? value.effects.events : empty.effects.events,
      commits: Array.isArray(value?.effects?.commits) ? value.effects.commits : empty.effects.commits
    },
    authority: {
      tokens: Array.isArray(value?.authority?.tokens) ? value.authority.tokens : empty.authority.tokens
    }
  };
}

function persistState(): void {
  fs.mkdirSync(path.dirname(dbPath), { recursive: true });
  fs.writeFileSync(dbPath, JSON.stringify(state, null, 2));
}

function send(res: any, status: number, value: unknown): void {
  const body = JSON.stringify(value);
  res.writeHead(status, {
    "content-type": "application/json",
    "content-length": Buffer.byteLength(body)
  });
  res.end(body);
}

function readBody(req: any): Promise<any> {
  return new Promise((resolve, reject) => {
    const chunks: any[] = [];
    req.on("data", (chunk: any) => chunks.push(chunk));
    req.on("error", reject);
    req.on("end", () => {
      const raw = Buffer.concat(chunks).toString("utf8");
      if (raw.trim() === "") {
        return resolve({});
      }
      try {
        resolve(JSON.parse(raw));
      } catch (error) {
        reject(new Error(`invalid JSON body: ${String(error)}`));
      }
    });
  });
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function operationIdFromPayload(value: unknown): string | undefined {
  if (!value || typeof value !== "object") {
    return undefined;
  }
  const record = value as { operation_id?: unknown; operationId?: unknown };
  return asOptionalString(record.operation_id) || asOptionalString(record.operationId);
}
