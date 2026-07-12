# SyncFuzz 项目定位

SyncFuzz 的目标是把前期关于 Agent、OS、安全边界、事务语义和主动漏洞挖掘的讨论收束成一个可执行项目：

> **面向 Shell-Enabled Agent 的跨层状态失同步漏洞自动化挖掘。**

本项目不优先构建新的 Agent Transaction 防御系统，而是主动攻击现有 Agent runtime 的 lifecycle 语义，寻找 checkpoint、retry、cancel、replay、fork、timeout、crash、resume 过程中出现的状态裂缝。

## 核心观察

OS 安全依赖可寻址、可中介、可判定的对象空间。Agent 的危险则来自开放语义空间：自然语言、repo 内容、shell 输出、tool response 和历史轨迹都可能改变模型如何使用真实权限。

因此，SyncFuzz 不试图给自然语言语义空间建立完整保护边界，而是关注真实副作用的状态投影：

```text
Agent logical state
OS execution state
External effect state
Authority state
```

当一次 lifecycle fault 让这些状态层对同一 effect 产生矛盾认知，就可能形成漏洞。

## 研究问题

SyncFuzz 优先回答一个可实验的问题：

> 在 Terminal Agent 中，攻击者能否通过可控环境、输出、延迟、异常或故障时序，让 Agent state 已经恢复或分叉，但 OS、external 或 authority state 没有同步恢复，从而产生可复现安全影响？

## 漏洞族

第一阶段关注四类 oracle：

- **Rollback Residue**：声称回滚后仍残留文件、进程、shell 状态、socket 或权限变化。
- **Forgotten External Effect**：外部 effect 已提交，但 Agent 回滚后忘记 receipt 并重复执行。
- **Authority Resurrection**：单次授权或 capability 已消费，但 Agent 恢复出旧授权状态。
- **Branch Leakage**：被 discard 的 speculative branch 影响最终 committed branch。

这些不是普通“模型被诱导”的 prompt 问题，而是跨越 Agent runtime、shell、OS、外部服务和授权系统的状态一致性问题。

## 最小闭环

MVP 不接真实 LLM，也不先测复杂框架。它用 deterministic harness 先证明：

```text
seed primitive
  -> lifecycle/fault boundary
  -> state snapshot
  -> differential oracle
  -> reproducible mismatch signature
```

只要这个闭环稳定，后续才能安全地扩展到 LangGraph persistent shell、MAF Workflow / GitHubCopilotAgent、OpenHands sandbox，以及真实 LLM 诱导阶段。AutoGen 保留为历史架构对照，而不是第二个主线 target。

## 运行环境策略

当前 MVP 默认使用 `local` environment backend，优先保证调试速度、artifact 可读性和 deterministic seed 的稳定性。同时已经支持 `container` backend：对 workspace 型 run 启动短生命周期 Docker 容器，把 workspace 挂载到 `/workspace`，禁用网络并设置基础资源限制，然后通过 `docker exec` 执行 shell primitive。后续在真实 Agent 或高风险 fuzz payload 阶段，再考虑 VM / microVM 隔离。

在跨层观测上，Phase 2 已形成统一 artifact contract：每个 run 都会生成 `agent-state.json` 和 `state-trace.json`，把 Agent、OS、External、Authority 四层观测映射到统一 lifecycle phase。所有 workspace-backed seed 都会输出 process snapshot、`process-lineage.json` 和 `filesystem-metadata.json`；container backend 会从容器 namespace 内部采集进程信息。External 与 Authority seed 则通过 mock service state snapshot 纳入同一 `state-trace.json` 索引。

Phase 3 已开始把故障注入从 case 内部的隐含逻辑提升为结构化调度输入：每个 run 会生成 `fault-plan.json`，记录 selected known-answer fault、inject phase、相关状态层、expected impact 和 deterministic timing profile。当前还新增了 `control` / `fault` pair 执行，`differential-report.json` 会比较两次 run 的 oracle 结果，并从 `state-trace.json` 汇总 observation coverage。`suite --differential` 可以批量执行 pair，并把 security-relevant differential discovery 写入 corpus。后续 feedback-guided mutation 留到 Phase 4。

Phase 4 第一版已经形成 deterministic feedback loop：`primitives` 命令列出已实现与 planned mutation primitive，`matrix` 命令枚举 `case x primitive x timing` 候选，`suite --matrix` 可以执行当前已实现候选并写出 `schedule-matrix.json` / `matrix-result.json`。每个发现会携带 `candidate_id` 和 `primitive_id`，每个候选会汇总 novelty、复现率、耗时和 artifact size。后续 run 可以通过 `--feedback-from` 和 `--candidate-limit` 用上一轮结果筛选候选；`campaign` 则自动执行多轮反馈调度、跨轮跳过已执行候选并写出 `campaign-result.json`。`double-fork-daemon` 已经从 planned primitive 转为 executable primitive。

但这里也要保持一个更严格的表述：当前 SyncFuzz 已明显超过普通参数化测试，也已经具备 feedback-guided candidate exploration；不过它距离“能够自动合成研究者未直接编码的新缺陷场景”的成熟状态型 Fuzzer 还有一段距离。当前最大的缺口不在 scheduler，也不在 corpus，而在 mutation 是否真的能创造新的执行语义，而不是仅仅从人工准备的候选列表中重排和筛选。

Phase 5 已经开始接入真实 target：第一版提供 `command` adapter，把任意本地或容器内可见的真实 Agent CLI 放进 SyncFuzz workspace 中运行，并通过 `SYNCFUZZ_PROMPT`、`SYNCFUZZ_PROMPT_FILE`、`SYNCFUZZ_TASK_FILE`、`SYNCFUZZ_REPO_ROOT`、`SYNCFUZZ_WORKSPACE` 等环境变量传递任务上下文。`target-prompt.txt` 和 `target-task.json` 会直接写进 workspace，因此真实 Agent 可以按文件路径读取任务契约；复杂命令则优先通过 `--command-file` 传入。每次 target run 会写出 `target-task.json`、`target-output.txt`、`target-result.json`，在 `--observe-delay` 后复用 filesystem/process snapshot、`agent-state.json` 和 `state-trace.json`。

首个仓库内置真实 target 也已经落地：`targets/langgraph_shell_react/`。它尽量贴近官方标准路径，只做最小的 `create_agent(...) + ShellToolMiddleware(...)` 组合，并在同一进程内保留 LangGraph checkpointer 与 thread history。这样我们可以先把 Shell session、thread state、replay/fork 语义放进 SyncFuzz 的 artifact contract 里观察，再决定是否需要更深的 runtime adapter。

## 当前阶段的研究校准

到目前为止，LangGraph 目标已经给了我们一批真实 residue 证据：persistent shell residue、workspace filesystem residue、orphan process residue 都能稳定落到 artifact 里。这说明 SyncFuzz 的 real-target 路线是对的，框架确实已经能观测真实 runtime 的跨层状态残留。

但这里需要一个重要校准：

> **观测到 residue，并不自动等于观测到漏洞。**

有些 residue 可能只是 runtime 的既定持久化语义；有些才是 replay / fork / discard / resume 的 lifecycle contract 被破坏；还有一些即使存在，也要等后续 trusted execution 消费它们之后，才会变成真正的安全后果。

因此，SyncFuzz 现在不应该把所有正结果都直接叙述成“漏洞”，而应该先把真实 target 结果分成三层：

1. residue evidence：有没有真实状态残留、分叉或干净负结果；
2. contract interpretation：它是否违反 target 的恢复/分叉契约；
3. activation consequence：它是否会被后续可信执行激活成安全后果。

框架主线优先负责前两层；第三层只做少量高价值验证实验，不把 exploit generation 变成主任务。

为此，下一阶段会引入 `Recovery Contract Profile`：按 target 记录 graph state、workspace、shell session、child process、external effect、authority state 在 replay / fork / resume / discard 等 lifecycle edge 上究竟应该 `preserve`、`reset`，还是 `unspecified`，以及这个判断来自显式文档、隐式代码语义，还是当前仍然未知。更详细的设计见 [PHASE5B_STRATEGY.md](PHASE5B_STRATEGY.md)。

同时，下一轮状态扩展也会从“继续补更多文件对象类型”转向“补更多仍然携带安全能力的残留状态”。也就是说，后续分类会优先围绕：

- storage capability；
- execution context；
- active execution；
- resource access；
- communication；
- authority；
- external effect；
- isolation topology。

这意味着 `cwd`、`umask` 这类 shell context 补充项仍然会做，但真正的主攻方向会逐步转到 open FD、Unix socket、authority cache 以及更强的 future-state process residue。

再往前一步，下一阶段还要把 testcase 从“越来越多独立任务名”提升为可组合的 Scenario IR。也就是说，后续 mutation 不应只体现在增加 `*-residue-fork`，而应逐步支持：

- state primitive substitution；
- lifecycle splice；
- phase shift；
- activation substitution；
- cross-seed crossover；
- automatic minimization。

只有当 testcase 可以由这些结构化 mutation 自动组合出来，而不是由研究者手写定义，SyncFuzz 才能更有力地证明自己具备自动发现新漏洞家族的能力。

## 路线校准

当前路线仍然保持在主动漏洞挖掘主线上，没有滑向通用防御系统或 prompt benchmark。判断依据是：

- 每个 seed 都有明确攻击者可控状态原语，而不是只测试模型是否“听话”。
- 每个 seed 都围绕 Agent lifecycle 语义：checkpoint、replay、rollback、fork、discard 或 persistent runtime。
- 每个 oracle 都基于确定性状态差分，而不是 LLM judge。
- 每个结果都输出可复现 artifact、mismatch signature 和 manifest。

因此，SyncFuzz 当前阶段的目标不是轻率地证明某个 Agent “不安全”，而是先建立一组可复现实验，确保 runner、trace、snapshot、oracle 和 artifact 格式能稳定表达跨层状态失同步现象，并进一步判断这些现象是否构成 lifecycle contract violation。
