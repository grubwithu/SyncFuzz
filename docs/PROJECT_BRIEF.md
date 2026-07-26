# SyncFuzz 项目定位

SyncFuzz 把前期关于 Agent、OS、安全边界、事务语义和主动漏洞挖掘的讨论收束成一个可执行项目：

> **面向 Shell-Enabled Agent 的跨层状态失同步漏洞自动化挖掘。**

本项目不优先构建新的 Agent Transaction 防御系统，而是主动攻击现有 Agent runtime 的 lifecycle 语义，寻找 checkpoint、retry、cancel、replay、fork、timeout、crash、resume 过程中出现的状态裂缝。

可执行的当前路线与术语以 [RESEARCH_PLAN.md](RESEARCH_PLAN.md) 为准；[RECOVERY_HAZARD_FUZZING.md](RECOVERY_HAZARD_FUZZING.md) 是当前 V3 设计规范。本文只做定位与边界说明。

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

当前的高层输入为：

```text
X = <Workload, EnvironmentProgram E, RecoveryPlan Σ>
```

其中 `Workload` 是经稳定性 gate 后冻结的正常任务对；`E` 是高频变异、可实际 materialize 的 typed resource-binding graph；`Σ` 是高频选择的 historical recovery plan。研究目标是形成可审计链：

```text
W -> R(C,H) -> U'
```

`W` 是一次已验证的 write/bind/capability-formation effect，`R(C,H)` 是保留 head OS state 的历史逻辑恢复，`U'` 是恢复后 Agent 正常工作中对资源的 typed resolve/use。这将“有静态 residue”与“恢复后的 Agent 真的依赖了 divergent object”分开。

已实现的 V2 闭环仍是必要底座：`StateObjective -> task synthesis -> profiling -> eBPF + state probe -> StateSeed -> frontier -> before/after/head recovery -> static A/O relation`。`fork` / `rewind` / `replay` 是 adapter mechanism 而非 discovery 维度。当前首先聚焦 Unix socket：已有一个 **fixture-only** 的 `EnvironmentProgram` materializer、run-local/semantic identity、`RecoveryUsePlan`、local `resolve -> connect -> I/O` 与 five-control hazard classifier；它只校准 IR 和判定逻辑，不能写成 LangGraph target 的 `U'`、finding 或 coverage。LangGraph profile 现可在首个 durable checkpoint 后于 target cgroup 内 materialize child-holder `E` 并保存 provenance；这还不是 eBPF frontier/head admission。target-side recovery-time eBPF trace 与 hazard scheduler 仍待实现。

状态基底按 state family 划分：`Namespace | Process | Handle/Capability | IPC | Execution Context | Metadata/Security`。V3 先从 Unix socket 的 name-to-listener binding 做窄而可审计的实例，再扩展到 executable resolution、FD/capability 或 context resource family。

## 历史基线

项目的 deterministic known-answer MVP 已完成（Phase 1–4 的 seed primitive、fault scheduler、differential oracle 与 feedback-guided matrix），它仍是回归验证集和 deterministic oracle 底座，但不再是新发现主张的基础。四类 oracle（Rollback Residue、Forgotten External Effect、Authority Resurrection、Branch Leakage）作为 known-answer seed 保留在 [MVP_SPEC.md](MVP_SPEC.md)；分阶段实现历史记录见 [archived/ROADMAP.md](archived/ROADMAP.md)。

[RESEARCH_PLAN.md](RESEARCH_PLAN.md) §2 已明确不再采用旧路线的 `primitive substitution`、`activation substitution`、`phase shift`、`cross-seed crossover` 与 Query genealogy：它们是独立状态样例、实验控制或 prompt presentation，不能说明系统产生了新的恢复状态，只保留为 regression fixture。

## 研究校准

> **观测到 static residue，并不自动等于 observed hazard，更不自动等于漏洞。**

有些 residue 只是 runtime 的既定持久化语义；有些才是 replay / fork / discard / resume 的 lifecycle contract 被破坏；还有一些即使存在，也要等后续可信执行消费之后才变成真正的安全后果。因此真实 target 结果分四层：

1. static A/O relation：有没有真实状态残留、分叉或干净负结果；
2. typed recovery dependence：恢复后的 Agent 是否实际 resolve/use 该对象（`U'`）；
3. contract interpretation：它是否违反 target 的恢复/分叉契约；
4. activation consequence：它是否会被后续可信执行激活成安全后果。

V3 首先负责前两层；后两层分别需要 contract 与更高层影响证据。它不把任意外部 consequence 或 exploit generation 作为主搜索任务。Recovery Contract（按 target 记录 graph state 与各 OS state surface 在 lifecycle edge 上应 `preserve` / `reset` / `unspecified`）仍是独立工作，其设计与 recovery semantics 见 [RESEARCH_PLAN.md](RESEARCH_PLAN.md) §9。

## 路线校准

当前路线保持在主动漏洞挖掘主线上，没有滑向通用防御系统或 prompt benchmark。判断依据是：

- 每个候选 `<Workload,E,Σ>` 都有可审计的 resource-binding mutation 与历史恢复，而不是只测试模型是否“听话”；
- 每个发现都围绕 Agent lifecycle 语义：checkpoint、replay、rollback、fork、discard 或 persistent runtime；
- 每个 verdict 都基于确定性 A/O relation 或 typed dependency evidence，而不是 LLM judge；
- 每个结果都输出可复现 artifact、mismatch signature 和 manifest。

因此，SyncFuzz 当前阶段的目标不是轻率地证明某个 Agent “不安全”，而是先建立一组可复现实验，确保 runner、trace、snapshot、oracle 和 artifact 格式能稳定表达跨层 A/O 状态失同步现象，并进一步判断这些现象是否构成 lifecycle contract violation。
