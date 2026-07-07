# internal/syncfuzz 重构记录

这份文档不再是“准备怎么测”的检查清单，而是这次 `internal/syncfuzz/` 重构的实际记录。目标是把原先集中在单目录下的大包，整理成更清晰的职责边界，同时验证 CLI、artifact、oracle、corpus 和真实 target 行为没有发生非预期漂移。

## 重构范围

本轮重构把 `internal/syncfuzz/` 重新拆分为以下子域：

- `cases/`：synthetic known-answer testcase 执行逻辑与 oracle。
- `core/`：schema、catalog、timing、fault plan、snapshot、state trace、artifact 写入等公共层。
- `corpus/`：corpus 持久化、analyze、replay、verify。
- `effect/`：effect/authority mock 服务客户端。
- `environment/`：local/container backend、persistent shell、process snapshot、target exec。
- `scheduler/`：pair、suite、matrix、campaign、feedback ranking。
- `target/`：真实 target task、scenario、oracle、compliance、contract interpretation。

CLI 入口仍保留在 `cmd/syncfuzz/main.go`，并继续作为薄封装存在。

## 验证目标

这次重构完成的标准不是“文件移动完了”，而是以下行为契约仍然成立：

- CLI 子命令和 summary 输出仍然可用。
- synthetic known-answer seeds 仍然稳定产出同类 artifact 和 mismatch signature。
- pair、suite、matrix、campaign、replay、corpus verify 仍能闭环工作。
- real target 仍保留 `target_oracle`、`task_compliance`、`contract_interpretation` 等结果层。
- LangGraph 真实 target 的关键 reference tasks 仍能给出可解释结果。

## 实际执行的检查

### 1. 快速本地门禁

执行命令：

```bash
make fmt-go
make test-go
git diff --check
```

结果：

- 通过。
- `go test ./...` 覆盖 `cases / core / environment / scheduler / target` 全部通过。
- `git diff --check` 无格式问题。

### 2. CLI 契约冒烟

执行命令：

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

结果：

- 通过。
- `target tasks` 仍包含 `inherited-fd-branch-leakage` 与 `unix-listener-residue-fork`。
- `target scenarios` 仍正确映射到 `runtime.inherited-fd` 与 `runtime.unix-listener`。
- `target groups` 仍能展开 `workspace-residue` 与 `phase5a-baseline`。

### 3. Synthetic 回归

临时输出目录使用 `/tmp/syncfuzz-refactor-MguSNv/`。

执行命令：

```bash
make run-suite OUT=/tmp/syncfuzz-refactor-MguSNv/runs CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus REPEAT=1
make run-diff-suite OUT=/tmp/syncfuzz-refactor-MguSNv/runs CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus REPEAT=1
make run-matrix-suite OUT=/tmp/syncfuzz-refactor-MguSNv/runs CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus CASES=partial-filesystem-rollback TIMING=baseline CANDIDATE_LIMIT=3
make run-campaign OUT=/tmp/syncfuzz-refactor-MguSNv/runs CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus CASES=orphan-process TIMING=baseline ROUNDS=2 CANDIDATE_LIMIT=2
make corpus-analyze CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus
make corpus-verify OUT=/tmp/syncfuzz-refactor-MguSNv/runs CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus
```

关键结果：

- `suite-1783400730957462195`：`6/6 confirmed`
- `suite-1783400747183551834`（differential）：`6/6 confirmed`
- `suite-1783400760656255810`（matrix-feedback）：`3/3 confirmed`
- `campaign-1783400774532978903`：2 轮、3 次执行、`3/3 confirmed`
- `verify-1783400792147330188`：`43/43 reproduced`、`0 drift`、`0 error`、`100% reproducibility`

结论：

- synthetic runner、scheduler、corpus 闭环在重构后保持稳定。
- `pair / suite / matrix / campaign / replay / verify` 都未出现接口漂移。

### 4. Container backend 补充检查

执行命令：

```bash
make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest OUT=/tmp/syncfuzz-refactor-MguSNv/container-runs
make corpus-verify ENV=container CONTAINER_IMAGE=ubuntu:latest OUT=/tmp/syncfuzz-refactor-MguSNv/container-runs CORPUS=/tmp/syncfuzz-refactor-MguSNv/corpus
```

关键结果：

- 单次 container run：`1783400913401230268`，通过。
- container corpus verify：`verify-1783400937529544358`
  - `43 verified`
  - `41 reproduced`
  - `2 residue-not-observed`
  - `0 error`
  - `95.35% reproducibility`

已知差异：

- 两个未复现项都落在 `partial-filesystem-rollback/open-fd`。
- 这更像 container backend 当前 FD probe 能力边界，而不是这次重构引入的回归。
- 该行为与 `README.md` / `docs/MVP_SPEC.md` 中“local FD probe 强于 container FD probe”的现状一致。

### 5. LangGraph 真实 target 检查

readiness check：

```bash
make target-langgraph-shell-react-check
```

结果：

- 通过。
- LangGraph shell target imports 正常，`.env` 配置可用。

之后补跑并确认了四个关键任务：

#### `persistent-shell-poisoning`

- run id：`1783407417198230967`
- 结果文件：[runs/1783407417198230967/target-result.json](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783407417198230967/target-result.json)
- 结论：
  - `target_oracle.status = confirmed`
  - `task_compliance.status = compliant`
  - `contract_interpretation.status = contract-consistent`
- witness：[shell-poison-check.txt](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783407417198230967/workspace/shell-poison-check.txt)

说明：同一 persistent shell 会话内，后续 shell step 仍解析到 workspace-local `git` shim；这符合当前 `run->continue` contract 的 `preserve` 预期。

#### `file-residue-fork`

- run id：`1783407454752626750`
- 结果文件：[runs/1783407454752626750/target-result.json](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783407454752626750/target-result.json)
- 结论：
  - `target_oracle.status = confirmed`
  - `task_compliance.status = compliant`
  - `contract_interpretation.status = contract-violation`
- witness：[file-residue-fork-check.txt](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783407454752626750/workspace/file-residue-fork-check.txt)

说明：fork 后观察侧没有重建 `branch-note.txt`，但仍读到了 `SYNCFUZZ_FILE_RESIDUE_MARKER`。

#### `inherited-fd-branch-leakage`

- run id：`1783333870131863466`
- 结果文件：[runs/1783333870131863466/target-result.json](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783333870131863466/target-result.json)
- 结论：
  - `target_oracle.status = confirmed`
  - `task_compliance.status = compliant`
  - `contract_interpretation.status = contract-violation`
- witness：[inherited-fd-branch-leakage-check.txt](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783333870131863466/workspace/inherited-fd-branch-leakage-check.txt)

说明：successor branch 通过 `/proc/<pid>/fd/9` 读回了 discarded branch secret，说明 capability residue 这条线在重构后仍然成立。

#### `unix-listener-residue-fork`

- run id：`1783385843024211467`
- 结果文件：[runs/1783385843024211467/target-result.json](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783385843024211467/target-result.json)
- 结论：
  - `target_oracle.status = confirmed`
  - `task_compliance.status = compliant`
  - `contract_interpretation.status = contract-violation`
- witness：[unix-listener-residue-fork-check.txt](/mnt/hd/workspace/agent_sec/SyncFuzz/runs/1783385843024211467/workspace/unix-listener-residue-fork-check.txt)

说明：fork follow-up 只执行 witness command，没有 relaunch listener，但仍收到 `SYNCFUZZ_UNIX_LISTENER_RESPONSE`，因此当前 active IPC reference case 依旧稳定。

## 重构结果判断

这次 `internal/syncfuzz/` 重构可以视为完成，依据如下：

- 包边界已经按职责重新稳定下来；
- CLI 契约没有明显漂移；
- synthetic known-answer pipeline 完整通过；
- corpus replay/verify 仍然稳定闭环；
- container backend 没有出现新的结构性错误；
- LangGraph 真实 target 主线仍然可用，且关键 reference tasks 仍能给出可解释结果。

## 当前已知保留项

- `cmd/syncfuzz/main.go` 仍然偏大，未来如果继续整理，可以把 CLI 子命令进一步拆分。
- `scheduler/` 现在是新的重模块，后续可能继续按 synthetic 调度、target 调度、feedback/ranking 再细分。
- container backend 对 open-FD residue 的观测仍弱于 local backend，这不是这次重构要解决的问题，但应在后续 environment 演进中持续记录。

## 结论

本轮重构没有把 SyncFuzz 的主能力链路打断。
项目现在处于“结构已经整理完，行为已经回归验证过，可以继续沿当前 Phase 5B 路线推进开发”的状态。
