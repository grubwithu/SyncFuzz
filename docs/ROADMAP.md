# SyncFuzz 分阶段路线

## Phase 0：项目创立与问题收束

目标：把方向从泛泛的 Agent Security 收束到主动漏洞挖掘。

产物：

- 项目定位：跨层状态失同步漏洞挖掘。
- 技术边界：Shell-enabled Agent，暂不做 GUI CUA、通用 Agent Transaction、完整 prompt fuzzing。
- 状态模型：`S = (A, O, X, C)`，即 Agent、OS、External、Authority 四层状态。
- MVP 骨架：Go CLI、TypeScript mock server、文档路线。

## Phase 1：Known-Answer MVP

目标：不用 LLM，先跑通可复现的状态失同步样例。

必须实现：

- deterministic minimal harness；
- filesystem snapshot；
- structured trace JSONL；
- mismatch signature；
- `orphan-process` seed；
- `action-replay` seed；
- `authority-resurrection` seed；
- `persistent-shell-poisoning` seed；
- `partial-filesystem-rollback` seed；
- `branch-leakage` seed；
- EffectServer 与 AuthorityServer mock；
- suite runner、`suite-result.json` 汇总、`interesting.json` novelty 记录、`corpus/` 登记、corpus replay 与 corpus verify；
- `local` / `container` environment backend 与统一 `--env` 参数通道；
- `orphan-process`、`persistent-shell-poisoning`、`branch-leakage` 的 local/container process snapshot artifacts 与 `process-lineage.json` 摘要；

完成标准：

> 能自动复现 known-answer mismatch，导出最小 PoC artifacts，批量运行所有 seed 生成 suite-level summary、novelty discoveries 和 corpus entries，并能把 corpus 作为持续回归验证集批量检查复现性。

## Phase 2：Cross-Layer Tracing

目标：把单纯文件快照扩展为跨层状态观测。

状态：已完成第一版，可进入 Phase 3。

已实现：

- container-aware process lineage 扩展到所有 workspace-backed seeds，并沉淀为可回归验证的 artifact contract；
- persistent shell probe；
- filesystem metadata probe；
- mock external state probe；
- mock authority state probe；
- lifecycle event alignment。
- 每个 run 输出 `agent-state.json` 与 `state-trace.json`，统一索引 Agent、OS、External、Authority 四层 artifact。

第一版可使用 Go + `/proc` + shell probe。后续再引入 BCC/bpftrace 或 libbpf CO-RE。

完成标准：

> 每个 run 能生成 Agent、OS、External、Authority 的统一 trace 与 snapshot。

审查记录见 [PHASE2_REVIEW.md](PHASE2_REVIEW.md)。结论：方向正确，已修正 workspace-backed seed 在 container backend 下的执行语义一致性问题。

## Phase 3：Fault Scheduler 与 Differential Oracle

目标：系统性注入 lifecycle fault，并自动判断 mismatch 是否有安全意义。

状态：第一版已完成。已实现 deterministic fault-plan catalog、deterministic timing profiles、`fault-plan.json` artifact、`control` / `fault` run role、`differential-report.json` pair-level report，以及 `suite --differential` / corpus metadata。现在 known-answer seed 的隐含 fault phase 已经变成 scheduler 可消费、可对照执行、可批量登记、可稳定复现的结构化输入。

Fault phase：

- P0 before tool intent
- P1 after intent before dispatch
- P2 after shell receives command
- P3 after child process created
- P4 after first OS/external effect
- P5 after command finishes before result delivery
- P6 after result delivery before checkpoint persistence
- P7 after checkpoint persistence before acknowledgment
- P8 during replay/resume

Oracle：

- Rollback Residue
- Forgotten External Effect
- Authority Resurrection
- Branch Leakage
- Cancel/Resume Split-Brain

完成标准：

> 不依赖人工判断，能从 control run 与 fault run 生成结构化 mismatch signature。

已实现：

- 定义 `control` / `fault` run pair；
- 从 `state-trace.json` 自动读取 observation coverage；
- 生成 pair-level differential report；
- 把 pair-level report 接入 suite / corpus，使 discoveries 可记录 differential verdict；
- 引入 `baseline` / `tight` / `wide` deterministic timing profiles。

下一步：

- 将随机或 feedback-guided fault timing 留到 Phase 4。

## Phase 4：Feedback-Guided Fuzzing

目标：从固定 seed 进入状态原语组合搜索。

状态：第一版完成。已实现 deterministic mutation primitive catalog、scheduler matrix、matrix-backed suite execution、candidate scoring/cost metrics、feedback-ranked candidate selection、multi-round campaign，以及首个从 planned 转为 executable 的新增 primitive。当前可以枚举、执行、排序并按上一轮反馈筛选 `case x primitive x timing_profile` 候选；campaign 会按预算跨轮探索未执行候选，并在候选耗尽后允许重复利用高分候选。

第一批 mutation 原语：

- background process
- double-fork daemon
- delayed write
- PATH/cwd modification
- shell alias/function
- untracked file
- symlink
- chmod/xattr
- open FD
- Unix socket
- external API commit
- single-use capability
- concurrent file replacement

Feedback：

- 新 mismatch signature；
- 新状态层组合；
- 更接近 privileged effect；
- 更高复现率；
- 更小 PoC。

完成标准：

> 相比 random fault timing，更快发现 known-answer case，并开始发现未知 mismatch。

已实现：

- `syncfuzz primitives`：列出已实现与 planned mutation primitive；
- `syncfuzz matrix`：枚举 deterministic scheduler candidate；
- matrix 默认只包含当前可执行 primitive，`--include-planned` 可查看未来搜索空间。
- `syncfuzz suite --matrix`：执行当前已实现的 scheduler candidates；
- `schedule-matrix.json` / `matrix-result.json`：记录 suite 使用的候选矩阵和每个 candidate 的执行结果；
- suite / discovery / corpus metadata 携带 `candidate_id` 与 `primitive_id`，为后续 minimization 和 feedback selection 提供稳定 handle。
- `candidate_summaries`：按 novelty、confirmed count、reproducibility 和 errors 对候选打分排序。
- 执行成本指标：每个 suite item 与 candidate summary 记录 duration、artifact bytes、artifact files 和 cost penalty。
- `--feedback-from <matrix-result.json>` / `--candidate-limit N`：用上一轮 candidate summary 对当前 matrix 排序，并按预算执行高优先级候选。
- `syncfuzz campaign`：自动执行多轮 matrix / feedback-ranked matrix suite，并写出 `campaign-result.json`。
- campaign-level exploration/dedup：`candidate-limit` 每轮生效，优先跳过已执行候选，记录 `unique_candidates` 与 `repeated_candidates`。
- `double-fork-daemon` 已从 planned primitive 转为 executable primitive，并进入 `orphan-process` 默认 matrix。

Phase 4 之后：

- 继续把 `open-fd`、`unix-socket`、`concurrent-file-replacement` 转为 executable primitive；
- 将 campaign 接到真实 Target Adapter，开始测试真实 Agent runtime。

## Phase 5：真实 Target Adapter

目标：从最小 harness 迁移到真实 Agent runtime。

状态：已启动。第一版先实现通用 `command` target adapter，用 observation-only 的方式把任意本地或容器内可见的真实 Agent CLI 放进 SyncFuzz workspace 运行，并复用 Phase 2 的 filesystem/process/state-trace artifact contract。它已经可以把 `target-prompt.txt` 和 `target-task.json` 直接写进 workspace、传递 prompt/task file path、捕获 stdout/stderr、通过 `--observe-delay` 等待 immediate observation、通过 `--late-observe-delay` 捕获 delayed effect、检查 expected files、写出 `target-result.json`，为 LangGraph、AutoGen、OpenHands 的专用 lifecycle adapter 打底。

顺序：

1. LangGraph + persistent shell；
2. AutoGen command executor；
3. OpenHands runtime/sandbox；
4. Crab、Shepherd、Cordon、DeltaBox 等研究原型。

每个 adapter 至少统一：

- run/reset；
- checkpoint/replay；
- cancel/resume；
- fork/discard；
- lifecycle events；
- workspace/sandbox binding。

已实现：

- `syncfuzz target list`：列出真实 target adapter，当前 `command` 已可用，LangGraph / AutoGen / OpenHands 为 planned；
- `syncfuzz target run --command ...` / `--command-file ...`：在 SyncFuzz workspace 中运行真实 target 命令；
- target task 环境变量：`SYNCFUZZ_PROMPT`、`SYNCFUZZ_PROMPT_FILE`、`SYNCFUZZ_TASK_FILE`、`SYNCFUZZ_REPO_ROOT`、`SYNCFUZZ_WORKSPACE`、`SYNCFUZZ_RUN_ID`、`SYNCFUZZ_TARGET_ID`；
- target artifacts：`target-task.json`、`target-prompt.txt`、`target-output.txt`、`target-result.json`；
- target observation：运行前后 filesystem snapshot、process snapshot、process lineage、filesystem metadata、`agent-state.json`、`state-trace.json`；
- `--observe-delay` / `--late-observe-delay` / `--expect-files`：对真实 delayed-effect target run 做最小可自动判定的 target oracle。
- 首个真实对象：`targets/langgraph_shell_react/`，使用官方 `create_agent(...) + ShellToolMiddleware(...)`，并导出 `langgraph-history.json`、`langgraph-run-summary.json`，以及按需导出的 replay/fork summary artifact。
- `orphan-process-long-delay`：为真实 Agent 增加更强的长延迟后台进程任务，不要求 `late-effect` 立即出现，并把 process lineage summary、late observation summary 和 task-specific `target_oracle` 摘入 `target-result.json`，用于直接判断 target command boundary 后是否仍有 workspace 相关进程，以及 delayed effect 是否在晚期观测窗口内出现。

下一步：

- 增加 LangGraph adapter wrapper，把 checkpoint/replay/cancel/resume 映射成 SyncFuzz lifecycle events；
- 把 `targets/langgraph_shell_react/` 从 in-process memory checkpointer 提升到 durable checkpointer；
- 为 AutoGen command executor 增加真实 shell tool 包装；
- 把 target run 接入 suite/campaign，使真实 runtime 能消费 Phase 4 matrix candidate。

完成标准：

> 同一个 seed 能在至少两个 runtime 上运行，并比较状态语义差异。

## Phase 6：漏洞确认与论文评估

目标：从测试框架变成漏洞挖掘工作。

实验：

- baseline：random timing、tool-boundary-only、syscall novelty、manual replay；
- metrics：unique vulnerabilities、time-to-first-vuln、false positive rate、reproducibility、PoC size、overhead；
- ablation：去掉 scheduler、去掉 state probe、去掉 feedback；
- real report：向维护者提交可复现 PoC。

完成标准：

> 至少形成若干真实系统的确认漏洞、确认风险语义或可维护者复现的安全报告。
