# PRD: MAF Target Integration

## Goal

把 Microsoft Agent Framework（MAF）接入 SyncFuzz，作为第二个框架级真实 target。

这条线的目标不是重复 LangGraph 的 persistent shell residue，而是验证：

```text
executor effect
  -> superstep barrier
  -> checkpoint
  -> resume / rehydrate
```

之间是否会出现跨层状态失同步。

## Why MAF

MAF 的官方 Workflow 模型提供了与 LangGraph 明显不同的恢复语义：

- `Executor`
- `Message + Edge`
- `Superstep`
- `CheckpointStorage`

其中最关键的是：

> checkpoint 发生在 superstep 完成之后，而不是每个 tool call 之后。

这让 MAF 很适合测试：

- external effect replay
- partial commit across parallel executors
- pending approval / request replay
- same-instance resume vs new-instance rehydrate

## Non-Goals

- 不自己实现一个复杂 Shell runtime
- 不把 MAF 接入做成新的 exploit framework
- 不把 documented unsafe reuse 直接当成未知漏洞

## Target Ladder

### MAF-1

对象：

- 官方 `GitHubCopilotAgent` shell sample

目的：

- 验证官方 shell-enabled target 可以被 SyncFuzz `command` adapter 稳定驱动
- 明确 artifact contract、workspace 绑定、stdout/stderr、prompt/task 注入方式

最小验收：

- 能稳定跑通至少一个 shell task
- 能产出 `target-result.json`
- 能确认 shell command 的实际执行痕迹

### MAF-2

对象：

- 官方 shell sample
- 官方 session restore

目的：

- 区分 Copilot session state 与 OS/workspace state
- 建立 `same logical session, different runtime object` 这条实验线

最小验收：

- 能保存并恢复 session identity
- 恢复前后 artifact 可比较
- 至少有一条 clean negative 或 residue evidence

当前落地状态：

- 已新增 `maf-session-continuity`，作为 MAF-2 的第一条 smoke path；
- wrapper 在第一轮 MAF turn 后序列化 `AgentSession`，再恢复到新构造的 `GitHubCopilotAgent` runtime object；
- `maf-session.json` 记录原始 / restored session id、serialized session hash、runtime 是否重建、前后 response hash；
- oracle / compliance 会结合 `maf-lifecycle.json` 判断 witness 是否来自后续 check call，而不是同一 shell call 里顺手重建。

这一步还不是 MAF Workflow checkpoint；它的作用是先把 `same logical session, different runtime object` 变成 SyncFuzz 可运行、可聚合、可回归的 target task。

### MAF-3

对象：

- 最小 Workflow
- `CheckpointStorage`

目的：

- 正式进入 MAF 的 `superstep / checkpoint / resume / rehydrate` 语义

当前落地状态：

- 已新增 `maf-workflow-checkpoint` target，先不经过 LLM；
- wrapper 使用官方 `WorkflowBuilder`、`Executor`、`WorkflowContext` 和 `FileCheckpointStorage` 构造一个两节点 workflow；
- 第一节点写入 `maf-workflow-effect.txt` 并发出消息，文件 checkpoint 记录 pending workflow state；
- wrapper 随后重建 workflow object，从 checkpoint restore，并由第二节点写出 `maf-workflow-continuity-check.txt`；
- `maf-workflow-summary.json` 记录 checkpoint ids、selected checkpoint、restore 是否发生、runtime object 是否重建，以及 post-restore witness。
- 已新增 `maf-workflow-external-effect-replay`，同样使用官方 Workflow checkpoint restore，但把 passive workspace observation 替换成 non-idempotent ledger append，用于观测一次 logical operation 在 restore 后是否产生重复 external effect。
- 已新增 `maf-workflow-http-effect-replay`，让 restored executor 通过 HTTP commit 外部状态；默认使用 in-process fallback，设置 `MAF_WORKFLOW_EFFECT_SERVICE_URL` 后可切换到 repo mock server 这样的独立服务进程，用于把本地 ledger replay 推进到 cross-process service boundary replay。
- 已新增 `maf-workflow-resource-replay`，沿用同一个 HTTP service boundary，但把 commit append 换成 external resource creation，用于观测 checkpoint restore 是否会让一个 logical operation 创建多个外部资源。
- 已新增 `maf-workflow-authority-token-replay`，在 token issue 之后、token consume 之前设置 checkpoint；初始分支先消费 token，restore 后再次消费同一个 token，观测独立 authority service 是否返回 `token_already_consumed`。
- 已新增 `maf-workflow-partial-commit-replay`，先让一个 executor 写入 external ledger，再让下游 executor 失败，随后从 pre-effect checkpoint restore，观察 partially committed effect 是否被再次执行。
- 已新增 `maf-workflow-approval-pending-replay`，使用官方 functional Workflow `request_info` / `responses` 路径，把 pending approval checkpoint restore 后的 response replay 映射为 duplicate external ledger effect。
- 已新增 `maf-workflow-rehydrate-divergence`，先用同一个 workflow instance 消费 pending approval，再从同一 checkpoint 重建 workflow object 并重放相同 response，用于直接比较 same-instance resume 与 checkpoint rehydrate 的 effect 差异。

这一步是 MAF-3 的最小 smoke path：它先证明 SyncFuzz 可以驱动官方 Workflow checkpoint restore，并把结果放进 target oracle / task compliance / suite 体系。当前 external-effect / partial-commit / approval-pending replay 仍以本地 ledger 为主；`maf-workflow-http-effect-replay`、`maf-workflow-resource-replay` 和 `maf-workflow-authority-token-replay` 已支持通过 `MAF_WORKFLOW_EFFECT_SERVICE_URL` 调用独立 mock service process，同时保留 in-process fallback，`maf-workflow-rehydrate-divergence` 则把 same-instance resume 和 recreated workflow rehydrate 放入同一条实验。下一步是把 authority/token 继续扩展到更接近真实 SSH / OAuth / CI token 的服务端状态。

首批场景：

1. `external-effect-replay`
2. `partial-commit-replay`
3. `approval-pending-replay`
4. `resume-vs-rehydrate-divergence`

最小验收：

- 至少一个场景能稳定给出 contract-aware result
- 至少一个场景能形成 honest negative 或 clean replay 基线

## Oracle Plan

MAF 线的 oracle 分三层：

1. `target_oracle`
   是否观测到 residue / replay / replay-negative
2. `task_compliance`
   任务是否真的按要求执行
3. `contract_interpretation`
   是否违反 MAF Workflow / session / checkpoint 的恢复契约

需要单独记录：

- `resume_mode`: `same-instance` / `rehydrate`
- `checkpoint_backend`
- `superstep_boundary`
- `effect_phase`

## Deliverables

1. `targets/maf_*` 目录下的最小接入对象
2. 一份 MAF `Recovery Contract Profile`
3. 至少一组 `target task`
4. 对应 suite / campaign 可消费的 target metadata
5. 文档与 README 更新

## Priority

1. MAF-1 smoke path
2. MAF-2 session restore
3. MAF-3 workflow checkpoint
4. contract-aware oracle refinement

## Exit Criteria

- SyncFuzz 已经不只覆盖 LangGraph
- 第二个 target 的架构差异明确来自 `superstep/checkpoint`
- 至少有一条可复现的 MAF contract-aware 结果
