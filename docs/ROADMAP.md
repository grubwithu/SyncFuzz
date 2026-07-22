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

但这里要明确校准：当前 Phase 4 更接近

```text
预定义场景空间
  -> 候选执行
  -> 反馈排序
  -> 跨轮去重
```

而不是

```text
较小语义原语
  -> 自动结构化变异
  -> 研究者未直接编码的新场景
  -> 新的状态转换与漏洞
```

也就是说，SyncFuzz 已经不是普通参数化测试，但是否已经成为真正意义上的状态型 Fuzzer，取决于它能否从较小的语义原语自动组合出研究者没有手写进任务列表的新 testcase。

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
- `--feedback-from <matrix-result.json>` / `--candidate-limit N`：用上一轮 candidate summary 对当前 matrix 排序，并按预算执行高优先级候选；当仍有未探索候选时，调度会优先展开新的 task / contract surface baseline probe，而不是先把同一 task 的不同 prompt profile 跑满。
- `syncfuzz campaign`：自动执行多轮 matrix / feedback-ranked matrix suite，并写出 `campaign-result.json`。
- campaign-level exploration/dedup：`candidate-limit` 每轮生效，优先跳过已执行候选，记录 `unique_candidates` 与 `repeated_candidates`。
- `double-fork-daemon` 已从 planned primitive 转为 executable primitive，并进入 `orphan-process` 默认 matrix。
- `open-fd` 已从 planned primitive 转为 executable primitive，并通过 local process snapshot 中的 workspace-related FD probe 进入 `partial-filesystem-rollback` matrix。

Phase 4 当前短板：

- 变异仍主要发生在研究者预定义的 candidate catalog 上；
- 场景表达仍偏 testcase-centric，缺少可组合的 Scenario IR；
- novelty 仍主要围绕 candidate / task / rule / surface，而不是跨层状态转换；
- 自动 minimization 还没有真正形成闭环。

当前最需要补的不是更多字段，而是实验性证据：必须尽快验证 feedback-guided campaign 是否真的比 `uniform random` 或固定枚举更快到达高价值 activation。建议先在一个受控 IR 空间里，用相同预算比较 `random / fixed enumeration / feedback-guided / full SyncFuzz` 的 `plant rate`、`survive rate`、`activation rate`、`time-to-first-impact` 和 `unique mismatch signature`。

Phase 4 之后：

- 把 real-target 与 synthetic candidate 都逐步重构到可组合的 Scenario IR，而不是继续累积越来越多的手写 testcase 名称；
- 继续把 `unix-socket`、`concurrent-file-replacement` 转为 executable primitive；
- 新增 `future-state orphan process` 这一类 active-execution primitive；
- 实现结构化 mutation：primitive substitution、lifecycle splice、phase shift、activation substitution、跨 testcase crossover；
- 把 guidance 从候选新颖度升级到状态转换新颖度、causal phase novelty、activation progress；
- 增加自动 minimization：删除 prompt instruction、lifecycle event、delay、primitive、activation step，保留最小可复现 PoC；
- 将 campaign 接到真实 Target Adapter，并逐步支持“组合发现模式”，从 seed 中抽取 `plant / lifecycle / activation / oracle` 重新组合。

## Phase 5：真实 Target Adapter

目标：从最小 harness 迁移到真实 Agent runtime。

状态：Phase 5A 观察里程碑已冻结，Phase 5B 现在正式转入 **contract-aware validation**。当前已经有一条稳定的 observation-first 路线：通用 `command` target adapter 把任意本地或容器内可见的真实 Agent CLI 放进 SyncFuzz workspace 运行，并复用 Phase 2 的 filesystem/process/state-trace artifact contract。它已经可以把 `target-prompt.txt` 和 `target-task.json` 直接写进 workspace、传递 prompt/task file path、捕获 stdout/stderr、通过 `--observe-delay` 等待 immediate observation、通过 `--late-observe-delay` 捕获 delayed effect、检查 expected files、写出 `target-result.json`，为 LangGraph、MAF、OpenHands 的专用 lifecycle adapter 打底。

### FSE A–O 路线重置（当前优先级）

论文的主问题收敛为 recovery consistency：`S = <A, O>`，其中
`O = <N, Pi, H, E>`。本文的核心方法不再同时宣称 eBPF、LLM、feedback
或大规模 fuzzing，而是：**typed lifecycle query -> resource footprint ->
state-probe plan -> deterministic differential/root-cause evidence**。

首个可执行里程碑已经落地为离线的 `resource-footprint.json` 和
`observation-plan.json`：它从 Scenario IR、filesystem snapshot、process
lineage 与 state trace 推导 query-specific state surfaces，并对 socket/FD
等资源施加显式 dependency closure。当前计划是 artifact/IR-guided，保留
`expand-once-then-full-probe` fallback；它不是 eBPF collector。target runner
现已可通过 `--observation-plan` 在 shadow mode 消费计划，将 plan-selected
path/process/FD objects 写入 `targeted-probe-report.json`，同时保留 broad
snapshot 作为 correctness fallback。`--observation-mode pruned-filesystem`
已经把重复的 workspace snapshot 切为 exact planned paths，并在最终状态保留
一次 `snapshot-full-fallback.json`；fallback 中的未规划路径会写回 report。
本地 `--observation-mode pruned` 进一步按 plan selector 收集 process/FD：先做
轻量 process identity 匹配，仅为命中的 PID 遍历 FD，并保留最终 broad
filesystem/process fallback。container backend 暂不支持该 selected process
collector，因此该模式显式要求 local。
`target refine-plan` 已能将这些路径（socket 同时补齐 filesystem/process/FD
dependency）确定性地扩展一次，之后强制保留 full-probe policy。

generic command adapter 现已提供 opt-in 的 `$SYNCFUZZ_LIFECYCLE_MARKER`：目标在
真实 plant / recovery / activation 完成后调用对应 marker，runner 在命令仍运行时
轮询并验证 JSONL marker，分别采集 P4/P6/P7 filesystem/process artifact，并在
capture 完成后才 ack helper，因而目标无法越过 marker 先执行下一阶段。未调用
marker 必须严格遵循 plant -> recovery -> activation 顺序；未调用 `after-plant`
marker 的命令仍诚实保留 P5 partial coverage。

每个 target run 现在还会输出 `target-checkpoint-differential.json`：固定 P0
baseline，选择每个 post-plant checkpoint 的 marker 或显式 fallback artifact，
再复用 deterministic filesystem metadata 与 process lineage analysis。它是后续
differential/root-cause 的 artifact evidence，不是 causal verdict 或 oracle。

两份 artifact 现在共享 `syncfuzz.lifecycle-query.v1`：
`q = <Init, Plant, Boundary, Recovery, Activation, Witness>`。每个阶段保留
Scenario IR component identity 与 kind；`violation_hypothesis` 只声明待验证
的 recovery-consistency relation，绝不替代 deterministic oracle 或 contract
interpretation。

紧接着的开发顺序是：

1. 固化 lifecycle query 与 violation signature 的 typed schema；
2. 在 controlled campaign 中量化 local plan-selected process/FD probe、三类 lifecycle marker 与 refine-once 后的 fallback coverage，并量化无 marker 时 P5 partial coverage；
3. 已可用 matching-query 的 `target compare` 产出 control/target pair evidence；下一步将经人工/contract 约束的 root-cause 输出绑定到这些 checkpoint；
4. 仅在环境和评估支持时，加入作为同一 evidence source 的 eBPF trace；
5. 让 LLM 仅从源码/契约生成 probe 或 contract 候选，绝不担当 oracle。

最新校准：

- LangGraph 实验已经证明：真实 runtime 中确实存在可稳定观测的 shell / workspace / process residue；
- 但 residue 的存在不自动等于漏洞，它可能是既定持久化语义、contract violation，或仅仅是尚未激活的风险状态；
- 因此，Phase 5B 的重心从“继续证明 residue 存在”转向“把 residue 放回 target 的恢复契约里解释，并让真实 target 开始消费 fuzz candidate”。
- 当前最大的风险不再是“框架能力不够”，而是“功能继续增加，但自动发现新缺陷的证据没有同步增强”。后续路线必须围绕 Scenario IR、semantic mutation、portable multi-target 和 minimization 收敛。

详细设计见 [PHASE5B_STRATEGY.md](PHASE5B_STRATEGY.md)。

顺序：

1. LangGraph + persistent shell；
2. Microsoft Agent Framework（MAF）Workflow / GitHubCopilotAgent；
3. OpenHands runtime/sandbox；
4. AutoGen command executor；
5. Crab、Shepherd、Cordon、DeltaBox 等研究原型。

每个 adapter 至少统一：

- run/reset；
- checkpoint/replay；
- cancel/resume；
- fork/discard；
- lifecycle events；
- workspace/sandbox binding。

已实现：

- `syncfuzz target list`：列出真实 target adapter，当前 `command` 已可用，LangGraph / MAF / AutoGen / OpenHands 为 planned；
- `syncfuzz target run --command ...` / `--command-file ...`：在 SyncFuzz workspace 中运行真实 target 命令；
- target task 环境变量：`SYNCFUZZ_PROMPT`、`SYNCFUZZ_PROMPT_FILE`、`SYNCFUZZ_TASK_FILE`、`SYNCFUZZ_REPO_ROOT`、`SYNCFUZZ_WORKSPACE`、`SYNCFUZZ_RUN_ID`、`SYNCFUZZ_TARGET_ID`；
- target artifacts：`target-task.json`、`target-prompt.txt`、`target-output.txt`、`target-result.json`；
- target observation：运行前后 filesystem snapshot、process snapshot、process lineage、filesystem metadata、`agent-state.json`、`state-trace.json`；
- `--observe-delay` / `--late-observe-delay` / `--expect-files`：对真实 delayed-effect target run 做最小可自动判定的 target oracle。
- 首个真实对象：`targets/langgraph_shell_react/`，使用官方 `create_agent(...) + ShellToolMiddleware(...)`，并导出 `langgraph-history.json`、`langgraph-run-summary.json`，以及按需导出的 replay/fork summary artifact。
- 首个真实对象：`targets/langgraph_shell_react/`，使用官方 `create_agent(...) + ShellToolMiddleware(...)`，并导出 `langgraph-history.json`、`langgraph-run-summary.json`、`langgraph-lifecycle.json`，以及按需导出的 replay/fork summary artifact。
- `orphan-process-long-delay`：为真实 Agent 增加更强的长延迟后台进程任务，不要求 `late-effect` 立即出现，并把 process lineage summary、late observation summary 和 task-specific `target_oracle` 摘入 `target-result.json`，用于直接判断 target command boundary 后是否仍有 workspace 相关进程，以及 delayed effect 是否在晚期观测窗口内出现。
- `persistent-shell-poisoning`：对真实 LangGraph shell target 使用 transcript-backed oracle；当 `shell-poison-check.txt` 里只有 workspace-local shim marker 时，也必须有 `langgraph-history.json` 证明它来自“后续 shell call 无需再次 export PATH 仍继承先前 PATH override”的场景。
- `persistent-shell-poisoning-replay`：内建 replay 任务会自动选择 `before-path-export` semantic checkpoint；当前 oracle 会把 replay 结果细分为 `runtime-preserved-residue`、`legitimate-reexecution`、`external-state-smuggling`、`clean-replay` 和 `unknown-causal-path`，并把 honest clean replay 固化为可回归的负结果。
- `persistent-shell-poisoning-fork`：内建 fork 任务会自动从 `before-path-export` semantic checkpoint 分叉；当前 oracle 既能确认 workspace-local PATH residue，也能把“fork 后干净回到 system git”的 honest 结果固化为 `clean-fork` 负样本。
- `file-residue-fork`：把真实攻击面从 PATH 扩到 workspace filesystem；内建 fork 任务会自动从 `before-file-drop` semantic checkpoint 分叉，并用 `branch-note.txt` / `file-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实文件残留、fork 侧重建和 clean fork。
- `directory-residue-fork`：继续沿 workspace filesystem 扩面；内建 fork 任务会自动从 `before-directory-create` semantic checkpoint 分叉，并用 `branch-dir` / `directory-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实目录残留、fork 侧重建和 clean fork。
- `delete-residue-fork`：继续推进 filesystem rollback 语义；内建 fork 任务会自动从 `before-file-delete` semantic checkpoint 分叉，并用 `branch-delete-note.txt` / `delete-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实删除残留、clean fork 对齐和 fork 侧误修改。
- `symlink-residue-fork`：继续沿 workspace filesystem 扩面；内建 fork 任务会自动从 `before-symlink-create` semantic checkpoint 分叉，并用 `branch-link.txt` / `symlink-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实 symlink 残留、fork 侧重建和 clean fork。
- `rename-residue-fork`：继续沿 workspace namespace 扩面；内建 fork 任务会自动从 `before-file-rename` semantic checkpoint 分叉，并用 `branch-rename-src.txt` / `branch-rename-dst.txt` / `rename-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实 rename 残留、clean fork 恢复和 fork 侧误修改。
- `mode-residue-fork`：继续沿 workspace metadata 扩面；内建 fork 任务会自动从 `before-file-chmod` semantic checkpoint 分叉，并用 `branch-mode-note.txt` / `mode-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实权限残留、clean fork 回滚和 fork 侧 chmod 重建。
- `append-residue-fork`：继续沿 workspace content 扩面；内建 fork 任务会自动从 `before-file-append` semantic checkpoint 分叉，并用 `branch-append-note.txt` / `append-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实内容追加残留、clean fork 内容回滚和 fork 侧重写。
- `hardlink-residue-fork`：继续沿 workspace object-type 扩面；内建 fork 任务会自动从 `before-hardlink-create` semantic checkpoint 分叉，并用 `branch-hardlink.txt` / `hardlink-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实 hardlink 残留、clean fork 回滚和 fork 侧重建。
- `fifo-residue-fork`：继续沿 workspace special-file 扩面；内建 fork 任务会自动从 `before-fifo-create` semantic checkpoint 分叉，并用 `branch-fifo` / `fifo-residue-fork-check.txt` / `langgraph-fork-summary.json` 区分真实 named pipe 残留、clean fork 回滚和 fork 侧重建。
- `open-fd-residue-fork` / `deleted-open-fd-residue-fork`：把真实攻击面从普通 workspace diff 扩到 resource-access capability；内建 fork 任务会自动从 fd-holder 之前的 semantic checkpoint 分叉，并用 `/proc/<pid>/fd/9` witness 区分真实 FD 残留、clean fork 和 fork 侧重建。
- `inherited-fd-branch-leakage`：沿 open-FD capability 继续推进到 discarded branch leakage；内建 fork 任务会从 fd-holder 之前分叉，并验证 successor branch 是否能通过继承的 `/proc/<pid>/fd/9` 读回 discarded branch secret。
- `unix-listener-residue-fork`：把 residue 从 resource-access capability 推进到 active IPC endpoint；内建 fork 任务会从 Unix listener 启动前分叉，并验证 successor branch 是否还能连接 discarded branch 留下的 `branch-listener.sock` 服务。
- `unix-listener-residue-fork` 已形成有效阳性：fork follow-up 不重复 listener launch，只执行 witness command，仍能收到 `SYNCFUZZ_UNIX_LISTENER_RESPONSE`；同时 lifecycle trace 已纳入 oracle / compliance，避免 fork 侧重建被误判为 runtime residue。
- durable checkpointer：真实 LangGraph target 新增 `disk` backend，并把 backend 元数据写入 `langgraph-checkpointer.json`；replay / fork / file-residue 这些 lifecycle 任务默认切到 durable backend，后续可以继续推进到跨进程恢复实验。
- split-process lifecycle mode：真实 LangGraph target 现在还能把 initial branch 与 replay/fork follow-up 拆到两个 Python 进程里执行，并复用同一个 durable checkpoint 目录；内置 replay/fork 任务默认启用该模式。phase artifact 与 merged artifact 都会保留，便于后续比较“同进程 replay/fork”和“跨进程 checkpoint-consume”的差异。
- 重构回归清单已沉淀为 [REFACTOR_TESTING.md](REFACTOR_TESTING.md)，覆盖 CLI contract、synthetic suite、matrix/campaign、corpus verify、LangGraph target smoke 和 active IPC gate。

Phase 5A 冻结内容：

- 真实 target run / suite / corpus / replay / verify 链路已经打通；
- 官方 LangGraph `create_agent + ShellToolMiddleware` 已经接入；
- `orphan-process-long-delay` 与 `persistent-shell-poisoning` 的 observation-first oracle 已稳定；
- `file-residue-fork` 的 transcript-backed filesystem oracle 已就位；
- `directory-residue-fork` 的 transcript-backed filesystem oracle 已就位；
- `delete-residue-fork` 的 transcript-backed deletion-residue oracle 已就位；
- `symlink-residue-fork` 的 transcript-backed filesystem oracle 已就位；
- `rename-residue-fork` / `mode-residue-fork` / `append-residue-fork` / `hardlink-residue-fork` / `fifo-residue-fork` 的 transcript-backed oracle 已纳入同一套 fork residue 框架；
- `open-fd-residue-fork` / `deleted-open-fd-residue-fork` / `inherited-fd-branch-leakage` 已把 workspace residue 从普通文件快照推进到 resource-access capability residue，并通过 `/proc/<pid>/fd/9` witness 区分真实 FD 残留、clean fork、fork 侧重建和 successor branch secret 读取；
- `unix-listener-residue-fork` 已进入真实 LangGraph target，开始覆盖 active IPC endpoint residue，并用 socket response witness 区分真实 listener 存活、clean fork 和 fork 侧重启；
- replay / fork 所需的历史 artifact、summary artifact 与 semantic checkpoint selector 已就位。
- durable checkpoint backend 已接入，workspace 内可直接审计 `langgraph-checkpoints/`。

Phase 5B 主线：

- 让 `Scenario IR` 成为 testcase 的主事实来源，而不是继续扩大手写 task catalog；
- 为 LangGraph 写出第一份 `Recovery Contract Profile`，并把 target 结果稳定拆成 residue observation、contract interpretation、activation consequence 三层；
- 已把完整 Scenario IR 接入 real-target matrix / suite / campaign，并形成第一组 portable same-run primitive-substitution candidate `persistent-shell-poisoning/primitive-shell-env-export` / `persistent-shell-poisoning/primitive-shell-function-define` / `persistent-shell-poisoning/primitive-shell-cwd-change` / `persistent-shell-poisoning/primitive-shell-umask-set`、`PATH replay -> env/function` primitive-substitution pair `persistent-shell-poisoning-replay/primitive-shell-env-export` / `persistent-shell-poisoning-replay/primitive-shell-function-define`、第一条 portable same-run trusted-activation candidate `unix-listener-residue/activation-trusted-action`、`orphan-process-long-delay/activation-trusted-action`、`PATH -> env/function` fork primitive-substitution family、Unix-listener / open-FD / deleted-open-FD / inherited-FD trusted-action activation substitution、process / open-FD / deleted-open-FD / inherited-FD trusted-action 的显式 `cross-seed-crossover` provenance、`unix-listener-residue-fork/lifecycle-splice-checkpoint-replay`，以及 `phase-shift-single-process`；后续重点是继续扩展 compatibility-aware family，再实现更广的 `lifecycle splice`、cross-seed family 与 `fault-phase / phase-shift mutation`；
- 把 `targets/langgraph_shell_react/` 从“单进程内 durable checkpointer”继续推进到“跨进程恢复可消费的 durable checkpointer”；
- 把当前 `LangGraph` / `MAF` 已共享的 same-run portable scenario 继续扩展到更高价值 family，并让 `MAF` 进入同一套 `campaign / replay / verify / minimize`；
- minimization 已从 prompt / execution-plan reduction 起步扩展到 concrete command line reduction、optional `Scenario IR component reduction`、component summary reduction、mutation provenance reduction、plant metadata reduction、impact-mode lifecycle / activation / oracle metadata reduction、fork activation message line reduction，并提供 `exact / semantic / impact` 三档 fidelity；下一步继续覆盖 semantic activation-command rewriting 与完整 lifecycle command rewriting；
- target suite / campaign 已支持 `--selection-policy fixed|random|explore|feedback` 与 deterministic `--random-seed`，并把选择策略写入 suite / matrix / campaign artifact；这让 `random / fixed enumeration / feedback-guided / full SyncFuzz` 能在同一候选宇宙和预算下对照；
- 把 replay / verify 的失败原因细化成 taxonomy，同时尽快实际跑完对照实验验证 feedback 的方法贡献。

Phase 5B 优先级重排：

- 当前 workspace residue family 已经足够丰富，后续不再以“补齐更多 Unix 对象类型”为主目标；
- 新增 testcase 的组织方式从对象类型逐步转向 **按能力分类**：storage capability、execution context、active execution、resource access、communication、authority、external effect、isolation topology；
- `cwd` 与 `umask` 仍然值得做，但定位为低成本补充项，而不是核心突破；
- 接下来两周的核心顺序是：
  1. 冻结 `Scenario IR` schema，并把 `3 - 5` 个 LangGraph testcase 迁移到纯 IR；
  2. 扩展已经落地的 `primitive substitution` 与 `activation substitution` family；
  3. 把当前 same-run portable scenario 扩展到更高价值的 replay / fork / trusted-activation family；
  4. 把已经出现的 `lifecycle splice` 与 FD trusted-action `cross-seed-crossover` 推广成更广的 generated family；
  5. 把 minimizer 从 concrete command line reduction、optional `IR component reduction`、component summary reduction、mutation provenance reduction、plant / lifecycle / activation / oracle metadata reduction 和 fork activation message reduction 继续扩展到 semantic activation-command rewriting；
  6. 用已落地的 selection policy 运行 `random / fixed enumeration / feedback-guided / full SyncFuzz` 四组小预算实验；
  7. 把已生成的 Unix-listener / process / FD trusted-action 场景扩展到更多 trusted consequence。

其中 process 线也已经从“命令返回后子进程仍存活”推进到第一条 `orphan-process-long-delay/activation-trusted-action`：残留执行主体会在 late observation window 中写入固定 trusted-action artifact。后续还要把它推进到 discarded branch / checkpoint 边界中的 future trusted state 实验。

Phase 5B 退出标准：

1. `Scenario IR` 成为 testcase 的主事实来源；
2. 至少形成四类 semantic mutation：`primitive substitution`、`activation substitution`、`lifecycle splice`、`fault-phase / phase-shift mutation`；
3. 至少支持一次 `cross-seed crossover`；当前已有 process、open-FD、deleted-open-FD 与 inherited-FD trusted-action crossover provenance，后续需要扩展到更系统的可枚举 family；
4. minimizer 可以删除 `IR component`，而不只删除 prompt 行或 `ExecutionPlan` 字段；
5. `LangGraph` 和 `MAF` 都能消费同一 portable scenario；
6. 两个 target 都能进入 `campaign / replay / verify / minimize`；
7. 至少一个 finding 满足：初始 seed 没有直接编码最终场景、由 mutation 或 crossover 自动生成、被 feedback 保留、经 oracle 确认、被 minimizer 缩减，并产生 trusted activation 后果。

完成标准：

> 同一个 portable scenario 能在至少两个 runtime 上运行并比较状态语义差异，同时真实 target 结果已经从“看见 residue”推进到“知道它是否违反恢复契约，并能否被后续 trusted execution 激活”。

## Phase 6：漏洞确认与论文评估

目标：从测试框架变成漏洞挖掘工作。

实验：

- baseline：`uniform random`、`fixed enumeration`、`feedback-guided`、`full SyncFuzz`，以及 `tool-boundary-only` / `manual replay` 对照；
- metrics：`plant rate`、`survive rate`、`activation rate`、`unique mismatch signature`、`time-to-first-impact`、`false positive rate`、`reproducibility`、`PoC size`、`overhead`；
- ablation：去掉 scheduler、去掉 state probe、去掉 feedback；
- real report：向维护者提交可复现 PoC。

完成标准：

> 至少形成若干真实系统的确认漏洞、确认风险语义或可维护者复现的安全报告。
