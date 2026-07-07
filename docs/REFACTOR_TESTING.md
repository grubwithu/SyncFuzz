# 重构测试指南

这份文档用于在整理 `internal/syncfuzz/` 目录之后验证项目行为没有漂移。重点不是“文件是否还在原位置”，而是 CLI、artifact、oracle、corpus 和真实 target 的行为契约是否保持稳定。

## 重构不变量

一次重构只有在以下契约保持稳定时才算安全：

- CLI 子命令仍能解析，并输出同样层级的列表或 summary。
- known-answer seeds 仍会生成 `result.json`、`state-trace.json`、`fault-plan.json`、process snapshot、filesystem metadata 和预期 mismatch signature。
- pair、suite、matrix、campaign、replay、corpus verify 的 schema 名称和 summary 字段不发生无意变化。
- target run 仍保留 `target-result.json`、`target_oracle`、`task_compliance`、`contract_interpretation` 和 LangGraph artifacts。
- 内置 target task id、checkpoint selector、witness 文件名、oracle attribution 字符串保持稳定；如果确实要变，必须同步更新 docs 和 tests。

## 快速本地门禁

每完成一小段文件移动或包拆分，先跑：

```bash
make fmt-go
make test-go
git diff --check
```

如果这里失败，先修复再继续移动更多代码。大部分 import、包边界、格式和单元测试问题都应该在这一层暴露。

## CLI 契约冒烟测试

这些命令应该快速完成，且不会写出大量 run artifacts：

```bash
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz list
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz fault-plans
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz timing-profiles
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz primitives
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz matrix --cases orphan-process --timing baseline,tight
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz target tasks
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz target scenarios
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz target groups
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz target prompt-profiles
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz target matrix --target langgraph-shell-react --group phase5a-baseline --prompt-profiles all
```

检查重点：

- `target tasks` 里仍有 `inherited-fd-branch-leakage` 和 `unix-listener-residue-fork`；
- `target scenarios` 里它们分别映射到 `runtime.inherited-fd` 和 `runtime.unix-listener`；
- `target groups` 里 `workspace-residue` 和 `phase5a-baseline` 仍会展开这些任务。

## Synthetic 回归门禁

使用 `/tmp` 下的临时目录，避免污染工作区：

```bash
make run-suite OUT=/tmp/syncfuzz-refactor-runs CORPUS=/tmp/syncfuzz-refactor-corpus REPEAT=1
make run-diff-suite OUT=/tmp/syncfuzz-refactor-runs CORPUS=/tmp/syncfuzz-refactor-corpus REPEAT=1
make run-matrix-suite OUT=/tmp/syncfuzz-refactor-runs CORPUS=/tmp/syncfuzz-refactor-corpus CASES=partial-filesystem-rollback TIMING=baseline CANDIDATE_LIMIT=3
make run-campaign OUT=/tmp/syncfuzz-refactor-runs CORPUS=/tmp/syncfuzz-refactor-corpus CASES=orphan-process TIMING=baseline ROUNDS=2 CANDIDATE_LIMIT=2
make corpus-analyze CORPUS=/tmp/syncfuzz-refactor-corpus
make corpus-verify OUT=/tmp/syncfuzz-refactor-runs CORPUS=/tmp/syncfuzz-refactor-corpus
```

预期形态：

- suite totals 非零，errors 为零；
- differential suite 写出 pair-level reports；
- matrix result 包含 `candidate_summaries`；
- campaign result 包含 `round_results`、`unique_candidates` 和 `repeated_candidates`；
- corpus verify 写出 `verification-result.json`，并包含 outcome taxonomy。

## LangGraph Target 门禁

先跑 readiness check。该命令会读取 `.env`，如果 `.env` 已经配置好模型和 endpoint，就不需要在命令行重复传 model：

```bash
make target-langgraph-shell-react-check
```

然后跑低成本真实 target smoke：

```bash
make target-langgraph-shell-react TARGET_TASK=persistent-shell-poisoning
make target-langgraph-shell-react TARGET_TASK=file-residue-fork
make target-langgraph-shell-react TARGET_TASK=inherited-fd-branch-leakage
make target-langgraph-shell-react TARGET_TASK=unix-listener-residue-fork
```

`unix-listener-residue-fork` 是当前 active IPC reference case。强阳性结果应该长这样：

```text
target_oracle.status = confirmed
target_oracle.attribution = runtime-preserved-residue
task_compliance.status = compliant
contract_interpretation.status = contract-violation
unix-listener-residue-fork-check.txt contains:
  PRESENT_BRANCH_UNIX_LISTENER
  SYNCFUZZ_UNIX_LISTENER_RESPONSE
```

还要人工看一眼 `workspace/langgraph-lifecycle.json`：resume/fork phase 应该只有一条 fork shell command，也就是 witness command；不能重复执行 listener-launch command。

## Target Suite 门禁

更宽一点但仍受控的真实 target 检查：

```bash
make target-langgraph-shell-react-suite TARGET_TASKS=persistent-shell-poisoning,file-residue-fork,inherited-fd-branch-leakage,unix-listener-residue-fork REPEAT=1
make target-langgraph-shell-react-matrix-suite TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=baseline CANDIDATE_LIMIT=3 REPEAT=1
```

预期形态：

- `target-suite-result.json` command errors 为零；
- 每个 result 都保留 `target_oracle`、`task_compliance` 和 `contract_interpretation`；
- suite summary 包含 attribution、compliance、contract 聚合；
- target matrix 输出 candidate ranking 和 contract metadata。

## 可选 Container 门禁

只有在修改 environment backend 或 process observation 时才需要跑：

```bash
make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest OUT=/tmp/syncfuzz-container-runs
make corpus-verify ENV=container CONTAINER_IMAGE=ubuntu:latest OUT=/tmp/syncfuzz-container-runs CORPUS=/tmp/syncfuzz-refactor-corpus
```

container backend 必须使用本地已有镜像，不能隐式拉取镜像。

## Artifact 人工排查顺序

门禁失败时，优先看这些文件：

- synthetic run：`result.json`、`state-trace.json`、`process-lineage.json`、`filesystem-metadata.json`；
- pair run：`differential-report.json`；
- suite / matrix / campaign：`suite-result.json`、`matrix-result.json`、`campaign-result.json`；
- target run：`target-result.json`、`target-output.txt`、`workspace/langgraph-history.json`、`workspace/langgraph-lifecycle.json`、`workspace/langgraph-fork-summary.json`；
- replay / verify：`replay-result.json`、`verification-result.json`。

先给失败分类，再改代码：

- command 或 adapter 没执行；
- task noncompliant；
- lifecycle edge 没触发；
- state 没植入；
- residue 没观测到；
- oracle inconclusive；
- 这是 honest clean negative；
- schema 或 artifact contract 发生了漂移。

## 大重构合并前门禁

合并大范围目录整理前，至少跑：

```bash
make fmt-go
make test-go
git diff --check
make run-suite OUT=/tmp/syncfuzz-final-runs CORPUS=/tmp/syncfuzz-final-corpus REPEAT=1
make corpus-verify OUT=/tmp/syncfuzz-final-runs CORPUS=/tmp/syncfuzz-final-corpus
make target-langgraph-shell-react TARGET_TASK=unix-listener-residue-fork
```

如果暂时没有 LangGraph 真实模型凭据，就记录“target gate 未运行”，先以 synthetic 和 unit-test gate 为准，等凭据可用后再补跑真实 target gate。
