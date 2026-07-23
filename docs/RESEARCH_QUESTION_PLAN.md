# 问题驱动开发计划

## 决策

SyncFuzz 后续不再以「继续扩充 mutation catalog」或「提高 campaign 指标」作为
默认目标。每一项开发、实验和 artifact 必须先对应下列五个问题之一，并明确其
可证伪的结论、所需证据和停止条件。

论文的主张保持收敛：对 Shell-enabled agent 的 `S = <A, O>` recovery
consistency 进行系统测试。这里的 `O = <N, Pi, H, E>` 分别表示 namespace、
process/handle、IPC/capability 和 execution context。LLM、mutation、feedback
和 eBPF 都是支撑机制，不能因为已经实现就自动成为论文主贡献。

Phase 5B 的 policy comparison 与 semantic-mutator ablation 已冻结在
[`PHASE5B_STRATEGY.md`](PHASE5B_STRATEGY.md)。它们应保留为可复现实验记录，
但在出现新的、预注册的假设前，不再继续调参、补跑或扩充 mutation family。

## 五个问题与当前证据

| 问题 | 已有证据 | 仍不能声称的结论 | 完成标准 |
| --- | --- | --- | --- |
| P1：Motivation 的 OS 根因是什么？ | `unix-listener-residue-fork` 已在真实 LangGraph target 上得到 `confirmed`、`runtime-preserved-residue`、`compliant`、`contract-violation`；lifecycle trace、checkpoint differential、runtime pair 与 calibration artifact 已存在。 | 该结果尚未经实际 counterfactual control 排除 fresh-runtime、branch cleanup 或 namespace restore 等替代解释，不能宣称已证明因果根因。 | 对 2--3 个高价值 Query（Unix listener、process/FD）实际运行可复现 control/target pair；人工审阅每个 calibrated hypothesis；Case Study 报告 OS object、PID/lineage、checkpoint 和 activation 的时间顺序。 |
| P2：Mutator 如何把 `q1` 变为 `q2`？ | `q=<Init, Plant, Boundary, Recovery, Activation, Witness>`、Scenario IR、violation signature 与 query genealogy 已冻结；已有 phase shift、primitive/activation substitution、lifecycle splice 和受控 crossover。 | 现有两组消融没有证明 full semantic scope 的效率收益，不能主张「mutator 优于 seed-only/prompt-only」。 | 发布 root seed ledger、operator catalog 和每个 derived query 的 `parent/operator/parameters/semantic_diff`；仅以可追溯、可执行和 oracle-compatible 作为本阶段验收。若重新研究效率，须先做 family-stratified compliance calibration、prompt-only control 和预先定义的 time-to-impact/unique-signature 指标。 |
| P3：LLM 能否促进 Oracle/Contract 自动化？ | `contract-propose` 的受限 provider、source-span allowlist、citation validation 和 `automatic_profile_adoption=disabled` 已实现。 | 尚未测量 LLM proposal 的有效性、人工节省或对最终 contract/oracle 的效用。 | 固定 target/source bundle，建立人工 gold/review protocol；报告 citation validity、field precision/recall、review acceptance、review time 和被接受 proposal 对 profile/plan 的可执行效用。LLM 输出始终只做 proposal，不能直接改变 oracle verdict。 |
| P4：Violation seed 如何分类？ | `syncfuzz.target-violation-signature.v1` 已按 `<relation, resource, boundary, mechanism, consequence>` 标注，且进入 matrix/suite/campaign coverage。 | 该 taxonomy 目前是 Query intent label，不是已经验证的 finding/root-cause 分类；其质量和覆盖性未验证。 | 冻结标签指南；对所有 root seed 和确认 finding 做人工标注及差异审阅；报告每一轴的覆盖、歧义和 taxonomy gap。它首先服务 P1 的证据编码，而不是单独扩展成大而全的贡献。 |
| P5：是否需要 eBPF？ | 已完成低风险前置路径：`footprint -> observation plan -> targeted probe`，支持 shadow/pruned、final broad fallback、refine-once 和 lifecycle marker。 | 该路径不是 eBPF collector，不能声称 trace-guided/eBPF 贡献，也不能由 eBPF 单独判定 contract 或 logical branch 语义。 | 先比较 broad、shadow、pruned/refine-once：oracle/contract verdict agreement、collection cost、marker/P5 coverage、fallback/refine rate。只有它确实暴露「现有 snapshot 看不到的对象/lineage」且评估环境允许时，才引入固定 eBPF trace 作为同一 evidence source；不做 per-query hook/code generation。 |

## 接下来的执行顺序

### 1. P1/P4：建立可审计的根因 Case Study 数据包

先冻结标签指南和 root seed ledger，再选择 Unix listener、一个 process/FD
candidate 和一个 clean control。对每个 candidate 记录 `A` 侧 lifecycle trace，
`O` 侧 pathname/inode、PID/lineage、FD 或 peer identity，并运行实际的
fresh-runtime、branch-cleanup 或 namespace-restore control。`target runtime-pair`
和 `target pair-campaign` 只负责记录结果；control command 必须真实实施其
声明的 intervention，不能由 runner 猜测。

这个阶段的产物是 Case Study 所需的最小、可审计证据，不是更多 residue
catalog。Motivation 的叙事和最终人工根因分析仍由 Case Study 审阅完成。

### 2. P5：先评估 observation plan，而不是抢先实现 eBPF

对步骤 1 中同一批 Query 用相同 command/contract 分别执行 broad、shadow 和
pruned/refine-once。将 runtime、artifact size、planned-object coverage、P5
partial coverage、fallback/refine 和 oracle/contract agreement 汇总成一份
comparison artifact。若 pruned 路径没有保持 verdict 或没有明显降低代价，则停止
这条优化；若证明有价值，再设计固定 eBPF pilot 来补足未知 OS footprint。

### 3. P3：把 LLM Contract proposal 变成可评估实验

固定 source bundle 和任务集合，先写 human gold/review manifest，再运行多个
显式 model/temperature-free proposal trial。评估的对象是「source-grounded
proposal 是否帮助人工形成可复核 contract」，而不是让模型直接判断漏洞。结果须
与人工 review 分开保存，且 profile adoption 继续保持显式人工操作。

### 4. P2：把 Mutator 降回可解释的 Query 生成机制

在 P1--P3 的固定 Query/contract 基础上，输出 query genealogy 和 operator
coverage 报告，确保任何 `q1 -> q2` 都能通过语义 diff 解释。现有 Phase 5B
negative ablation 保留在论文中作为边界：semantic mutation 已可执行，但在当前
预算下未证明效率收益。除非该报告暴露一个明确且可修正的 compatibility gap，
否则不继续扩 mutation catalog。

## 统一的实验纪律

- 每个实验先写明对应 P1--P5、假设、control、预算、主要指标和停止条件。
- `confirmed residue`、`contract-violation`、`contract-calibrated hypothesis` 和
  人工确认的 OS root cause 必须严格区分。
- 只把 `<A,O>` 纳入 FSE 主结论；MAF 的 external effect/authority task 可保留为
  工程探索，但不能混入主统计。
- 新 artifact 必须同时记录 Query identity、signature、genealogy、target/contract
  version、control kind 和原始 run path，避免事后按结果重新定义类别。

## 暂停项

在上述完成前，暂停以下工作：继续优化 feedback ranking/prompt repair、增加普通
workspace residue seed、宣称 semantic mutator 的效果、实现动态 eBPF hook
pruning，以及把 LLM proposal 自动接入 contract profile 或 oracle。
