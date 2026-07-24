# SyncFuzz 项目定位

SyncFuzz 把前期关于 Agent、OS、安全边界、事务语义和主动漏洞挖掘的讨论收束成一个可执行项目：

> **面向 Shell-Enabled Agent 的跨层状态失同步漏洞自动化挖掘。**

本项目不优先构建新的 Agent Transaction 防御系统，而是主动攻击现有 Agent runtime 的 lifecycle 语义，寻找 checkpoint、retry、cancel、replay、fork、timeout、crash、resume 过程中出现的状态裂缝。

可执行的当前路线与术语以 [RESEARCH_PLAN.md](RESEARCH_PLAN.md) 为准；本文只做定位与边界说明。

## 核心观察

OS 安全依赖可寻址、可中介、可判定的对象空间。Agent 的危险则来自开放语义空间：自然语言、repo 内容、shell 输出、tool response 和历史轨迹都可能改变模型如何使用真实权限。

因此，SyncFuzz 不试图给自然语言语义空间建立完整保护边界，而是关注真实副作用的状态投影：

```text
Agent logical state (A)
OS state (O)
```

OS state 是 Agent 执行在操作系统、外部服务与授权系统中留下的可观测副作用面——进程、文件、socket、capability、已提交的外部 effect、authority 状态等都归入 OS state 这一侧。当一次 lifecycle fault 让 Agent logical state 与 OS state 对同一 effect 产生矛盾认知，就可能形成漏洞。

## 研究问题

SyncFuzz 优先回答一个可实验的问题（与 [RESEARCH_PLAN.md](RESEARCH_PLAN.md) §1 一致）：

> 当 shell-enabled Agent 已执行到 logical head `H` 并形成持久 OS state `O_H` 时，从严格早于 `H` 的历史 logical checkpoint `C` 恢复、而 relevant OS state 仍被保留，是否会得到不兼容的 Agent/OS 关系？

研究对象不是某个产品名为 `fork`、`rewind` 或 `replay` 的 API，而是 historical checkpoint cut `<A_C, O_H>`（`C ≺ H` 且 `ΔO(C,H) ≠ ∅`）。产品 API 只是实现该 cut 的 adapter mechanism；不同 mechanism 的 OS retention / re-execution 语义必须先验证一致，才能并入同一实验条件。

## 当前路线

新闭环为：

```text
State Objective
  -> task synthesis
  -> profiling execution
  -> eBPF + state-probe validation
  -> executable StateSeed
  -> checkpoint-effect frontier mining
  -> historical checkpoint recovery set (before / after / head)
  -> differential A/O classification
```

它包含两个相互独立、按顺序运行的搜索器：**State Fuzzer** 为未覆盖的 OS 状态目标合成自然任务并只保留经真实执行验证的 StateSeed；**Historical Checkpoint Recovery Fuzzer** 围绕已观测持久 OS 状态变化选 historical cut，在固定 OS retention policy 下测 frontier 前、frontier 后与 logical head。`fork` / `rewind` / `replay` 是 adapter mechanism 而非 discovery 维度。完整术语、IR、里程碑与迁移边界见 [RESEARCH_PLAN.md](RESEARCH_PLAN.md)。

状态基底按 state family 划分：`Namespace | Process | Handle/Capability | IPC | Execution Context | Metadata/Security`。当前主攻方向是 open FD、Unix socket、authority cache 这类仍携带安全能力的残留状态，而不是继续堆叠更多文件对象类型。

## 历史基线

项目的 deterministic known-answer MVP 已完成（Phase 1–4 的 seed primitive、fault scheduler、differential oracle 与 feedback-guided matrix），它仍是回归验证集和 deterministic oracle 底座，但不再是新发现主张的基础。四类 oracle（Rollback Residue、Forgotten External Effect、Authority Resurrection、Branch Leakage）作为 known-answer seed 保留在 [MVP_SPEC.md](MVP_SPEC.md)；分阶段实现历史记录见 [archived/ROADMAP.md](archived/ROADMAP.md)。

[RESEARCH_PLAN.md](RESEARCH_PLAN.md) §2 已明确不再采用旧路线的 `primitive substitution`、`activation substitution`、`phase shift`、`cross-seed crossover` 与 Query genealogy：它们是独立状态样例、实验控制或 prompt presentation，不能说明系统产生了新的恢复状态，只保留为 regression fixture。

## 研究校准

> **观测到 residue，并不自动等于观测到漏洞。**

有些 residue 只是 runtime 的既定持久化语义；有些才是 replay / fork / discard / resume 的 lifecycle contract 被破坏；还有一些即使存在，也要等后续 trusted execution 消费之后才变成真正的安全后果。因此真实 target 结果分三层：

1. residue evidence：有没有真实状态残留、分叉或干净负结果；
2. contract interpretation：它是否违反 target 的恢复/分叉契约；
3. activation consequence：它是否会被后续可信执行激活成安全后果。

框架主线优先负责前两层；第三层只做少量高价值验证实验，不把 exploit generation 变成主任务。Recovery Contract（按 target 记录 graph state 与各 OS state surface 在 lifecycle edge 上应 `preserve` / `reset` / `unspecified`）作为后续独立工作，其设计与 recovery semantics 见 [RESEARCH_PLAN.md](RESEARCH_PLAN.md) §9。

## 路线校准

当前路线保持在主动漏洞挖掘主线上，没有滑向通用防御系统或 prompt benchmark。判断依据是：

- 每个 StateSeed 都有明确攻击者可控状态原语，而不是只测试模型是否“听话”；
- 每个发现都围绕 Agent lifecycle 语义：checkpoint、replay、rollback、fork、discard 或 persistent runtime；
- 每个 oracle 都基于确定性 A/O 状态差分，而不是 LLM judge；
- 每个结果都输出可复现 artifact、mismatch signature 和 manifest。

因此，SyncFuzz 当前阶段的目标不是轻率地证明某个 Agent “不安全”，而是先建立一组可复现实验，确保 runner、trace、snapshot、oracle 和 artifact 格式能稳定表达跨层 A/O 状态失同步现象，并进一步判断这些现象是否构成 lifecycle contract violation。
