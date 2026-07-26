# SyncFuzz v2 研究计划：状态目标驱动的 Historical Checkpoint Recovery Fuzzing

状态：**当前路线**（2026-07-24）。本文取代旧路线中以 `primitive substitution`、`activation substitution`、`phase shift`、`cross-seed crossover` 为核心的变异计划。此前的设计与实验记录保留在历史分支和归档文档中，但不再作为新的实现或论文主张的基础。

本计划由 [ChatGPT-0723.md](ChatGPT-0723.md) 收束而来；后者是讨论记录，本文是可执行的规范。

## 1. 核心问题与新方法

SyncFuzz 要回答的问题是：

> 当 shell-enabled Agent 已执行到 logical head `H` 并形成持久 OS state `O_H` 时，从严格早于 `H` 的历史 logical checkpoint `C` 恢复、而 relevant OS state 仍被保留，是否会得到不兼容的 Agent/OS 关系？

形式化地，研究对象不是某个产品名为 `fork`、`rewind` 或 `replay` 的 API，而是 historical checkpoint cut：

```text
initial execution reaches H:        <A_H, O_H>
recover a historical C while retain: <A_C, O_H>
```

只有 `C ≺ H` 且 `ΔO(C,H) ≠ ∅` 时，logical rollback 才可能形成新的 A/O mixed state。产品 API 只是 adapter 用于实现该 cut 的 mechanism；若一个 mechanism 会重放 effect、销毁 relevant runtime 或复制 OS namespace，它具有不同的 OS retention semantics，不能与 retain-state recovery 混为同一个实验条件。

新的闭环为：

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

这包含两个相互独立、按顺序运行的搜索器：

1. **State Fuzzer**：为未覆盖的 OS 状态目标合成自然任务，并只保留经真实执行验证的状态形成实例。
2. **Historical Checkpoint Recovery Fuzzer**：围绕已观测的持久 OS 状态变化，选择 historical cut，并在固定 OS retention policy 下测试 frontier 前、frontier 后与 logical head。

当前 LangGraph executor 使用产品的 `fork` 路径实现 retain-state historical recovery；这是**当前 adapter mechanism**，不是 discovery 搜索维度。以后接入 replay / rewind 时，先验证它们是否实际构造相同的 `<A_C,O_H>`；不相同则把其 OS retention / re-execution 语义作为独立受控条件，而不是把 API 名字并入 fuzz 笛卡尔积。

## 2. 不再采用的设计

以下机制不再属于发现 Query 的 Mutator：

- 将 PATH、环境变量、shell function、FD、Unix socket 等手写场景互相替换；
- 只改变 topology、prompt profile 或 process mode 而把它记为新的 Query；
- 用 `trusted-action` 将已观测 residue 接到 SyncFuzz 预写的后果；
- 以 `cross-seed crossover` 拼接已知 plant 与 activation；
- 用 `parent_query_id` 描述上述操作形成的“谱系”。

它们分别是独立状态样例、实验控制、prompt presentation 或后发现影响验证；都不能说明系统产生了新的恢复状态。历史任务可以保留为 regression fixture，但不计入新路线的 StateSeed corpus 或 coverage claim。

## 3. 术语与不可变关系

| 概念 | 含义 | 是否由人工提供 |
| --- | --- | --- |
| `StateObjective` | 希望形成的 OS 状态关系：effect atom、lifetime、resource relation 与持久性要求 | 是，提供有限 grammar 与合法组合 |
| `SynthesisCandidate` | 为一个 objective 生成的一次自然语言任务尝试 | 否 |
| `ProfileRun` | 不执行恢复的完整 Agent 执行；记录逻辑 checkpoint、raw trace 与基础快照 | 否 |
| `NormalizedEffect` | 对 raw eBPF / probe evidence 的有限语义归一化结果 | 否 |
| `StateSummary(C)` | checkpoint `C` 时可确认的持久 OS 资源及其依赖闭包 | 否 |
| `CheckpointFrontier` | 相邻 checkpoint 间出现经确认的持久状态增量 `ΔR` | 否 |
| `MaterializationHead` | initial execution 完成后、relevant OS effect 仍被确认存在的 logical head `H` | 否；由 terminal controller checkpoint 的 probe evidence 冻结进 StateSeed / recovery set |
| `StateSeed` | 满足 objective、跨 frontier 存活，并可用于在 head OS state 上做历史恢复的 `ProfileRun` | 否，自动晋升 |
| `HistoricalRecoverySet` | 同一 seed、head、retention policy、plan、probe 下的 before / after / head controls | 否；已实现，`RecoveryPair` 仅为历史兼容子集 |
| `RecoveryQuery` | `<seed_id, materialization_head, historical_checkpoint, retention_policy, passive_observation, mechanism>` | 否 |

`StateObjective` 不是 prompt；`SynthesisCandidate` 不是 Query；`StateSeed` 不是人工 testcase；`RecoveryPair` 不是 Query genealogy。

对于一个 frontier `(C_i, C_{i+1}]` 和 materialization head `H`，目标实验结构是：

```text
Q_before = <seed, H, C_i,     retain relevant OS state, W, mechanism>
Q_after  = <seed, H, C_{i+1}, retain relevant OS state, W, mechanism>
Q_head   = <seed, H, H,       retain relevant OS state, W, mechanism>
```

三者的 task、recorded execution plan、topology、retention policy、oracle 和 probe schema 必须相同；**historical checkpoint cut 是唯一 discovery 变量**。`Q_before` 是核心发现 query，`Q_after` 是 frontier-local control，`Q_head` 是 no-logical-rollback control。`HistoricalRecoverySet` 已将 head / retention / 三个 query 写成一等 artifact；`RecoveryPair` 仅保留给旧 fixture 兼容。

## 4. State Objective 与状态面

论文声明的状态基底是：

```text
Namespace | Process | Handle/Capability | IPC | Execution Context | Metadata/Security
```

一个 objective 至少包含：

```yaml
objective_id: ipc.unix-rebind.detached-survival
effects:
  - family: process
    operation: detach
  - family: namespace
    operation: rebind
  - family: ipc
    operation: listen
lifetime: survive-tool-return
relation: fixed-path-served-by-descendant
persistence: across-checkpoint
```

人工维护的是 effect grammar、资源依赖和合法组合，而不是具体路径、daemon 故事或 prompt。初版 atom 覆盖 `Process`、`Namespace`、`Handle/Capability`、`IPC`；`Execution Context` 通过 shell/context probe 加入，`Metadata/Security` 作为第二轮对象。没有一个 family 的实际、可验证 objective 以前，不宣称该 family 已覆盖。

## 5. Hybrid Observation 与 Frontier

### 5.1 观测职责

```text
eBPF / raw collector  -> 哪个区间发生了内核可见 effect
state probe            -> 哪些资源在 checkpoint / recovery 时仍然存在
differential oracle    -> Agent logical state 与 OS state 的关系类别
```

每个 profiling run 使用固定的宽发现面；不能按当前 seed 动态缩窄 hook。第一版 collector 至少记录 process lifecycle、pathname mutation、FD/capability、Unix IPC 事件，并为每项记录 monotonic timestamp、PID/TID、run cgroup（或等价 isolation identity）、process lineage 与 resource identity。

checkpoint 必须以同一 monotonic clock 记录时间戳。Normalizer 将 raw event 归并为有限 effect；state probe 为资源补齐 dependency closure。例如 Unix endpoint 的 closure 至少包括 pathname、kernel socket、holder FD、holder process 与实际 peer identity。

每个 checkpoint 形成：

```text
R(C) = persistent resources observable at C
ΔR_i = R(C_{i+1}) - R(C_i)
```

只有 `ΔR_i` 含有经 probe 确认、可跨边界存活的状态时，区间才是 frontier。仅有 syscall 数量或 LLM 声称的预期 effect 均不构成 frontier evidence。

### 5.2 Container scope 与 host-side collector

真实 profiling 的默认执行环境是每 run 一个低权限 Docker container。container 是 workspace、进程树和 cgroup 的归因边界；collector 运行在宿主机，以该 container 的 host PID 解析 cgroup identity 后过滤所有 descendant event。collector 不进入 container，container 也不获得 `privileged`、`CAP_BPF` 或宿主机 `/proc` 访问。

同一 historical recovery query 的 initial materialization 与 fresh recovery process 留在同一个 container，以保留要判断的 `O_H`；同一 recovery set 的 before / after / head query 必须使用彼此独立的新 container。这样每条 query 测试的是自己的 `<A_C^(q), O_H^(q)>`，而不是跨容器传递一个物理 OS instance；实验必须另行检查这些 freshly materialized head 在声明的状态关系上可比。`local` 仅用于单元测试、fixture 和离线分析，不作为正式 profiling/coverage 结果的默认环境。初始 sandbox 保持非 root、无网络、`cap-drop=ALL`、`no-new-privileges`、CPU/内存/PID 限制；外部服务测试需要显式、单独的网络策略。

### 5.2 Frontier 选择与覆盖

frontier 分数仅用于调度，依据 persistence、capability creation、namespace mutation、lifetime escape 和 novelty。选择采用按 state family 分层的预算，而不是全局 Top-K，避免 Unix socket 等高频对象吞没其他状态面。

每个有效执行写入覆盖元组：

```text
<family, operation, lifetime, resource_relation,
 recovery_mechanism, retention_policy, checkpoint_relation, relation_signature>
```

并周期性执行 full-vs-pruned probe 对照。pruned probe 的 verdict、resource identity、attribution 或 reconstruction 分类与 full probe 不一致时，不能将其作为可靠优化。

## 6. 两个搜索循环

### State Fuzzer

1. coverage scheduler 选择未覆盖或低覆盖的 `StateObjective`；
2. 通过通用 generator interface 合成一个正常的软件工程任务；LLM 只是可替换实现，不是 oracle；
3. Agent 执行候选，collector 与 probes 验证实际 effect；
4. 若 objective 未满足，记录缺失 atom 并 repair/regenerate；
5. 只有满足 effect、persistence、attribution，以及在 materialization head 仍可确认 relevant OS state 的候选才晋升为 `StateSeed`。

当前手写 LangGraph 任务仅用于校准 collector、adapter 和 oracle；它们不作为“靠手写 seed 覆盖状态面”的证据。自然性以人工抽样审查，不能由模型自述代替。

### Historical Checkpoint Recovery Fuzzer

1. 从已晋升的 seed 提取并分层选择 frontier；
2. 将 frontier 前、frontier 后和 logical head 组成一个 historical recovery set；
3. 对每条 query 固定 retention policy，先 materialize `H`，再用 adapter mechanism 恢复 selected `C`，并使用固定 passive observation；
4. 用 deterministic evidence 分类 `consistent`、`residual`、`missing`、`duplicate`、`reconstruction` 或 `inconclusive`；
5. 将 before/after 的 boundary-localization evidence 与 head negative control 一并保留。

`trusted action` 可以在确认 contract violation 后由人工做独立 case study，但它不进入上述 scheduler、coverage 或 Query 生成逻辑。

## 7. 实施里程碑

| 阶段 | 交付物 | 完成标准 |
| --- | --- | --- |
| V2.0 规范与清理 | 本文、v2 data-model audit、旧 mutation 标记为 legacy | 新代码不消费 `TargetScenarioMutation`、mutation-focus prompt variant 或 Query genealogy |
| V2.1a Evidence contract | checkpoint、raw event、normalized effect、checkpoint state summary、frontier JSON schema，raw-trace import，deterministic normalizer 与 fixture tests | **已实现**；不依赖特权 eBPF 也能从记录的 trace 得到稳定 frontier map |
| V2.1b Objective / recovery / coverage IR | `StateObjective`、validated seed、historical recovery set 与 coverage record schema | 这些对象不复用旧 Scenario mutation / Query genealogy；当前 `RecoveryPair` 是 recovery set 的 before/after 兼容子集 |
| V2.2 Profiling collector | checkpoint monotonic timestamp、collector interface、Linux host-side eBPF adapter、per-run container/cgroup scope、core state probes | LangGraph calibration fixture 在独立 container 中生成可归因的 raw trace 与 `R(C)`；不支持 eBPF 或 cgroup v2 的环境明确失败而非静默降级 |
| V2.3 Historical checkpoint recovery | `frontier -> Q_before/Q_after/Q_head` generator、显式 head/retention contract、adapter recovery executor、generic classifier | IR、head evidence gate 与 set executor 已实现；LangGraph 已以 distinct native head 完成 V3 privileged live recovery-set 校准 |
| V2.4 Execution-validated synthesis | objective grammar、coverage scheduler、generator command contract、candidate repair/retention | 新 StateSeed 只能由实际 trace 验证后进入 corpus；手写 fixture 不计覆盖 |
| V2.5 Breadth and fidelity | 分层 frontier selection、coverage ledger、full-vs-pruned calibration | 可报告各 family 的 objective、effect、frontier 与 boundary coverage，且明确支持范围 |
| V2.6 扩展 | 其他 recovery mechanism、第二批 family、contract-profile automation | 仅当 adapter 证明同一 retention/re-execution 语义时，才与 historical-cut baseline 合并比较 |

V2.1 先使用离线 trace fixture，是为了把 Normalizer、effect map 和 pairing 语义与 eBPF 部署权限隔离；它不是用手写 seed 替代自动生成。`syncfuzz profile analyze` 消费 checkpoint catalog、raw-event JSONL 与 checkpoint state summaries，写出 `normalized-effects.json` 和 `checkpoint-effect-map.json`。V2.2 通过后，真实 collector 是所有 coverage claim 的必要条件。

V2.2 的 process-lifecycle slice 已完成 container smoke validation：`profile container-scope` 从运行中 Docker container 的 host PID 解析 cgroup-v2 identity；它同时解析 mountinfo，因此支持 unified 与 hybrid hierarchy。`profile process-monitor` 在宿主机以该 identity 做内核侧过滤，采集 `fork`、`exec` 与 `exit` 并写出 raw-event JSONL。`target run --env container --profile-processes` 会自动写出 process artifacts，并以与 `bpf_ktime_get_ns` 同域的 `CLOCK_MONOTONIC` 写出 controller observation checkpoint catalog、workspace/live-process/open-FD state summaries、normalized effects 和 checkpoint-effect map。

V2.2 的 resource syscall slice 已完成同一 calibration path 的 live validation：`target run --profile-resources` 在同一 cgroup 上记录成功的 `openat`、`close`、`dup*`、namespace mutation、socket/IPC、cwd 和 metadata syscalls，并写出 `ebpf-resource-scope.json` 与 `ebpf-resource-events.jsonl`。`touch frontier-marker` calibration 已得到与 probe 结果同路径的 `openat("frontier-marker")` evidence；随后 deleted-open-FD calibration 也已在真实 privileged run 中得到 `dup(fd=9, device=2049, inode=51668070)` 与 checkpoint 中同一 deleted handle 的 `exact-device-inode` link。target command 的相对路径会按 `/workspace` 规范化；effect map 只为 effect 与 checkpoint delta 中资源存在 exact canonical-path、exact-path、exact `(device,inode)` 或 exact socket ID 匹配时写出 `evidence_links` 并选择 frontier。对 FD identity，collector 会在读取 ring-buffer record 时 best-effort 地解析仍存活的 host FD，而 container probe 解析 workspace-held FD 的 `(device,inode)`；两侧 identity 齐全才匹配。Unix socket probe 的完整 closure 已在特权运行 `1784805732832067342` 通过校准：cgroup `51176` 内的 eBPF 记录 host PID `2647769` 的 `bind` / `listen`，两者均携带 `socket:177721907`；container checkpoint 同时记录 `/workspace/branch-listener.sock`、endpoint `unix-socket:socket:177721907`、container PID `43` 的 FD `3`，且通过 `bound-at-path`、`references-unix-socket`、`held-by-process` 形成完整依赖闭包。`before-command..after-command` 是 frontier，并将 `bind`、`listen` 分别以 `exact-socket-id` 链接至该 endpoint。因此同一区间的动态链接器、shell 初始化和无关 syscall 不再能单独触发 frontier。第一版为 Linux/amd64。controller checkpoint 只是当前 command adapter 的可审计观测边界，不替代未来 Agent-native durable checkpoint。它要求 `CAP_BPF`、`CAP_PERFMON`（或 root）以及 tracepoint access，尚未产生 state-surface coverage claim。

首轮多 family audit 已被固化为 `profile calibration-audit`：它重新读取完成的 container run、两份 cgroup scope、checkpoint catalog/state summaries 和 effect map，分别检查 canonical path、deleted FD `(device,inode)`、Unix socket closure 的 known-answer link。当前三次 run `1784802253362129838`、`1784802974016599838`、`1784805732832067342` 输出 4/4 expected link、0 unexpected、fixture-scoped precision/recall 都为 1.00；报告明确标注为 **fixture-scoped**，不把这三个已知答案当作全局 detector precision/recall 或 state-surface coverage claim。Unix socket 还要求 closure 延续到紧接的 observation checkpoint。

## 8. 现有代码的迁移边界

保留并复用：真实 target adapter、checkpoint/fork 执行能力、workspace/process snapshots、artifact writer、corpus replay、minimizer 与 deterministic oracle 的基础设施。

重构或停用：`TargetScenarioMutationKind`、`Mutations` / `MutationFocus`、派生 prompt variant、target matrix 中的 generated scenario candidate、以及所有依赖它们的 mutation coverage 指标。它们必须从 v2 调度路径删除，而不是套一层新的名字。

现有 `target task` 改为两类：

- `CalibrationFixture`：可重复的 adapter/collector/oracle 回归输入；
- `SynthesisScaffold`：向生成器提供的正常项目环境与任务类别。

新建模块按职责分为 `objective`、`profiling`、`observation/effect`、`frontier`、`recovery`、`coverage` 与 `synthesis`；避免把它们重新塞进 target matrix 或 Scenario mutation 文件。

V2.1b 的独立 IR 已落在 `internal/syncfuzz/objective`、`recovery` 和 `coverage`：`StateObjective` 只接受 bounded effect atom、lifetime、resource relation 与 persistence；`StateSeed` 只能由每个 atom 均有 evidence link 的 persistent frontier 自动晋升，且 linked resources 必须在 terminal materialization-head checkpoint 仍被 probe 确认；`RecoveryPair` 固定为同一 seed、同一 recorded plan artifact、同一 passive observation 的 fork before/after 兼容子集。新的 `HistoricalRecoverySet` 将显式 `<H,C,ρ,μ,W>` 记录为 `materialization_head`、`retain-relevant-os-state` 和 `Q_before/Q_after/Q_head`；所有 query 只能改变 checkpoint coordinate。coverage 的目标维度是 `<family, operation, lifetime, resource_relation, boundary, checkpoint_relation, relation_signature>`，绝不读取 legacy Scenario mutation、prompt variant 或 Query genealogy。当前 ledger 仍兼容旧 aggregate `outcome`；relation-novelty scheduler 尚未接线，不能把 schema 目标误写成已有 coverage 指标。每个 `ProfileRun` 必须显式标为 `synthesis-candidate` 或 `calibration-fixture`：只有前者可晋升为 `StateSeed`，后者即使是成功的真实 eBPF run 也只能校准 collector，绝不计入 coverage。`profile promote-seed` 可离线读取带 provenance 的 ProfileRun，也可导入一次完成的 target profiling artifact；导入时必须声明 provenance，`synthesis-candidate` 由 V2.4 scheduler 产生，不能为手写 smoke 标记。`profile recovery-pair` 与 `profile recovery-set` 均只能复用 seed 锁定的 recorded plan artifact。

V2.3 的 executor core 已落在 `internal/syncfuzz/recovery`：`ForkExecutorRegistry` 只为真正暴露 durable Agent checkpoint 的 adapter 注册 executor；每个 `RecoveryObservation` 必须绑定原始 query、recorded plan、passive observation、materialization head 与 retention policy，并报告独立 `runtime_instance_id`。`ExecuteForkRecoverySet` 强制 before/after/head 使用三个不同 runtime instance，且只向 executor 传递不同的 checkpoint coordinate；其 legacy deterministic classifier 输出 `consistent`、`residual`、`missing`、`duplicate`、`reconstruction` 或 `inconclusive`，并在 `Q_head` 不是 `consistent` 时拒绝把 before/after 的现象归因给 rollback。新的 `RecoveryRelationReport` 将 evidence completeness、logical effect phase、resource origin/multiplicity、relation class 与 `ContractEvaluation` 拆开：relation fuzzer 只消费完整 relation signature，contract 默认 `not-evaluated`，不把任何 relation 升格为漏洞。其 `seed_resource_ids` 只是 StateSeed 的 frontier scope，不是已完成的 effect/resource graph 归因。LangGraph 的 native manifest、frontier binding 和 fork plan 现保留 checkpoint-owned `durable_tool_lifecycle`（tool-call ID/name 与 tool-result ID）；缺失表示 legacy artifact，显式空值表示该 checkpoint 没有完整 durable tool identity。新 lifecycle event 以 `CLOCK_MONOTONIC` 标记 command span；仅当唯一 shell span 完整包围 linked eBPF effect window 且 after checkpoint durable 地记录同一 call/result 时，binding/plan 才附加 `tool_effect_provenance`。缺时间戳、歧义 span 或缺 after result 一律为 unknown。`recovery execute --out-relation` 与 `recovery classify-relation` 现将 immutable plan 中的结果复制为 `causal_effect_evidence`（`proven` 或 `unknown`），供后续 relation-novelty scheduler 消费；它不是 Oracle，也不改变当前 relation signature、classifier 或 contract status。因此当前 phase 仍只安全导出 `effect-not-committed` / `effect-committed`，不得使用 `PRE_CALL` / `CALL_DURABLE` / `RESULT_DURABLE`。resource graph 与 relation-novelty scheduler 仍是后续工作。`ExecuteForkPair` 仍保留给旧 fixture。`command` adapter 没有 Agent-native durable checkpoint，因此 registry 明确拒绝它；不能以 controller observation checkpoint 代替恢复 execution。

`RelationNoveltyLedger` 现已作为 offline coverage artifact 接收 complete recovery relation report；它以 canonical effect scope、adapter、causal evidence status、proven tool name 与 before/after/head normalized signatures 去重。tool-call ID、shell session、command hash、task 与 contract status 都只是审计信息，不能制造 novelty。相同 tuple 的独立 profile 增加 confidence record 而不增加 tuple coverage。`synthesis schedule --relation-novelty-ledger` 现在只按 canonical effect scope 的 proven tuple 数加入次级 exploration priority；unknown causal tuple 仅作审计计数，不能降低该 priority。ledger 仍未传给 generator，因此 relation-novelty 不会被翻译为手写 Oracle 或目标 task 文本。

文档状态更新：上文 V2.1b/V2.3 段落中的“relation-novelty scheduler 尚未接线”是历史阶段描述；当前 `synthesis schedule` 已支持通过 `--relation-novelty-ledger` 读取该 coverage artifact。该接线只改变 objective priority，不改变 StateObjective、RecoveryRelation、ContractEvaluation 或 generator 输入；resource graph 与 generator feedback 仍未实现。

第一个接入点是 `maf-workflow` recovery adapter，而不是 legacy `target run` 的 generic command adapter。它调用 MAF Workflow 的 `FileCheckpointStorage`，在准备阶段形成两个真实、文件持久化的 native checkpoint（effect 之前和之后）；每个 query 都复制准备好的 initial workspace、重建新的 `Workflow` 对象，并用一个精确 native checkpoint ID 进行 restore。adapter plan 显式保存 V2 checkpoint coordinate 到 native MAF ID 的 binding，因而 recovery executor 不能把 controller checkpoint ID 直接交给 MAF。`make maf-workflow-native-fork-smoke` 是该 integration 的 live calibration：它产生 `prepared`、`before`、`after` 三个独立 workspace 与各自的 restore observation。`runs/maf-v2.3-fork-smoke-2` 已成功验证该路径：`v2-start` queue 的 native checkpoint `31f70e81-…` 在 fresh runtime 中 re-execute Plant，得到 `agent=absent, os=present, origin=reconstructed`；`v2-plant` queue 的 checkpoint `e58a22b6-…` 不重放 Plant，得到 `agent=present, os=present, origin=residual`。两者 runtime identity 不同，且均有 MAF restore callback 与重建 Workflow object 的 evidence。该 calibration 的 paired classifier 语义为 `before=reconstruction`、`after=consistent`、总体 `reconstruction`，因此它是预期的 clean calibration，不是 violation。此校准尚是 local fixture，不是 StateSeed、coverage 或 container profiling claim；把 synthesis-generated ProfileRun 映射到 native MAF coordinate 并纳入每-query container isolation 是 V2.4 的工作。

V2.4a 的 synthesis contract 已落在 `internal/syncfuzz/synthesis`。coverage scheduler 只按 objective atom 在 V2 coverage ledger 中的 `<family,operation>` 稀缺性排序；generator command 接收 `SYNCFUZZ_SYNTHESIS_REQUEST` 所指向的 bounded JSON request，并只能在 stdout 返回一个自然任务 JSON。scheduler 为 task 计算 canonical `SynthesisCandidateID`；generator 无法指定 target、adapter、candidate ID、mutation、prompt variant 或 parent query。`ProfileRun(kind=synthesis-candidate)` 与 `StateSeed` 现在都必须携带该 candidate ID，且 `profile promote-seed` 必须同时收到相匹配的 scheduler candidate artifact；因此历史 target run 不能只靠填写 `--profile-kind` 进入 seed corpus。`synthesis evaluate` 仅接受 linked persistent frontier 作为实际 effect，输出 missing-atom feedback 供下一次 attempt 使用；`synthesis promote` 只有在 candidate/profile identity 一致且所有 objective atom 都已验证时才会保留 seed。`synthesis bind-maf-frontier` 已将一个 profile frontier 显式映射到 MAF native checkpoint ID；它要求 profile 的 `native_checkpoint_run_id` 与 manifest 的 initial runtime identity 一致，并验证 MAF 持久化的 `v2-start` / `v2-plant` queue coordinate，拒绝把无关 profile 绑到便利的 checkpoint fixture。当前没有内建 LLM 或默认 generator。

LangGraph 已成为第一条真实 candidate execution 路径：`synthesis execute-langgraph` 只接收 scheduler-issued candidate，以候选的 `task` 作为真实 Agent prompt，在专用镜像的独立 container 中同时开启 process/resource eBPF profiling，并写出 candidate-bound `ProfileRun`。wrapper 强制使用 disk checkpointer，额外记录 `langgraph-native-checkpoints.json`：其中的 `native_checkpoint_run_id` 和精确 LangGraph checkpoint ID 与 controller 的 profiling checkpoint 分开保存。每次 durable `put` 还记录同一 `CLOCK_MONOTONIC` 域的 `persisted_monotonic_ns`。`synthesis bind-langgraph-frontier` 只在同一 native runtime、同一 validated frontier 中，以每个 linked objective effect 的时间窗选择严格前后的 native checkpoint；历史索引、checkpoint 文件顺序或 controller checkpoint 名均不能替代此证据。binding 同时写出每个 native ID 的 structural coordinate（history index、message count、`next`）供完整性审计。`synthesis prepare-langgraph-fork` 现冻结 profile workspace 与 disk checkpoint store 的 SHA-256、源 thread ID、精确 native checkpoint ID，以及 materialization-head socket 的 device/inode/mode。每个 recovery query 先验证 source snapshot，再将常规 workspace 与 durable store 克隆到独立 workspace，并把原 socket 节点只读 bind-mount 到该 clone；fresh Python process 直接以 source checkpoint ID restore，绝不重新执行 candidate task 或以 live-model history shape 替代 checkpoint。`lstat` probe 仍只读且不连接 endpoint。credentials 不会进入 plan。V3 privileged live recovery-set calibration 已在 profile `1784896157047894121` 完成：`Q_before` 为 `residual`，`Q_after` 与 `Q_head` 为 `consistent`，并以 source PID/network namespace 中同一 listener 的 socket ID、holder PID/FD 形成 exact evidence。该手工 baseline 不计入 generator discovery 或 coverage；旧 coordinate-resolution pair 仅是历史 `inconclusive` 基线。完整的输入、evidence chain、语义和限制见 [LANGGRAPH_END_TO_END_CLOSURE.md](LANGGRAPH_END_TO_END_CLOSURE.md)。

## 9. 实验与论文证据

在宣称新方法有效前，需要分别完成：

1. **Synthesis validity**：目标 effect 的生成成功率、持久性、可重放性与自动 coverage 增量；
2. **Frontier guidance**：eBPF-selected historical recovery set 相比随机或非-frontier checkpoint cut 的有效 A/O relation / localization 产出；
3. **Breadth**：按已声明 family 与 effect grammar 报告 coverage，禁止以 testcase 数量替代；
4. **Probe fidelity**：full 与 pruned probe 的分类一致率、漏资源率和开销；
5. **Recovery semantics**：先固定 `retain relevant OS state` 的 historical-cut contract；之后才评估各产品的 fork / replay / rewind API 是否满足该 contract，或应作为不同 retention / re-execution condition 单列。

所有 verdict 依赖可审计的 trace、probe 和 deterministic oracle。Recovery contract 自动生成仍是独立问题：它可为 oracle 提供期望语义，但不替代 effect validation 或 frontier selection。

## 10. 与既定五项任务的对应

| 任务 | v2 处理方式 |
| --- | --- |
| 根因分析 | 已完成；作为 calibration fixture / case study，不再扩展为 mutation 主线 |
| Mutator | 改为 objective-driven task synthesis 与 historical checkpoint cut；不再变异 recovery API 名称 |
| Oracle / Contract 自动化 | 保留为后续 contract-profile 工作；当前先以 deterministic A/O 分类保证证据闭环 |
| Violation / Seed 分类 | 由 effect grammar、validated StateSeed 与 coverage ledger 给出，而非手工 testcase 标签 |
| eBPF 引入 | 作为 profiling 与 frontier mining 的核心发现信号；state probe 负责持久性确认 |

## 11. 紧接着要做的工作

第一层 **FD→`(device,inode)` identity probe** 已实现并完成 privileged live calibration；deleted-open-FD 的 collector effect 与 checkpoint probe 已形成 `exact-device-inode` link。Unix socket 的 namespace/FD identity 与 dependency closure 也已由 `1784805732832067342` 完成 privileged live calibration：`bind` / `listen` 经 `exact-socket-id` 关联到完整 endpoint closure。canonical-path、FD identity 与 Unix socket 的首轮 known-answer audit 均已完成，并由可重跑的 `profile calibration-audit` 输出 fixture-scoped precision/recall。V2.1b Objective / pair / coverage IR 与 provenance gate 已完成；V2.3 的 MAF-native durable-checkpoint recovery adapter 已完成 live fixture calibration；V2.4a 已实现 objective scheduler、generator contract、candidate provenance/retention gate，以及 logical-frontier 到 native MAF checkpoint 的 identity binding。LangGraph 现已接入真实 candidate 的 isolated, eBPF-profiled execution，并保留 initial durable runtime 的精确 checkpoint catalog；这一步不会把 controller checkpoint 冒充为 Agent checkpoint。LangGraph native-frontier mapper 已由 `1784813806441091527` 完成 live calibration：它要求同一 `CLOCK_MONOTONIC` 域的 native durable-save 时间戳，并只接受严格包围 linked objective-effect window 的 native checkpoint 对。LangGraph 的当前 fork executor 已完成 before / after live execution：query 内的 initial 与 fresh resume process 保留同一 workspace 以观察 `O_H^(q)`，before / after 则在独立 container 中执行。该实现是 historical recovery set 的 before/after 子集；它尚未显式记录 materialization head、head-time persistence 或 `Q_head` no-rollback control，且当前 Unix-socket metadata probe 的 multiplicity evidence 仍未知，故最新 pair 为 `inconclusive`。下一步首先是实现 explicit head/retention contract 与 head control，再增强 multiplicity probe；不能把该基线写成漏洞。`command` adapter 仍被明确排除，不能把 controller observation checkpoint 当作恢复点。V2.5 再以 full-vs-pruned 与新增 family 扩展 fidelity/breadth 实验。collector 与 controller checkpoint 只能产生可审计 evidence：它们不单独决定漏洞 verdict 或 StateSeed 晋升。当前没有内建 LLM generator，不新增 trusted-action，也不把任何手写 smoke input 晋升为 StateSeed。LangGraph reference vertical slice 的完整审计说明见 [LANGGRAPH_END_TO_END_CLOSURE.md](LANGGRAPH_END_TO_END_CLOSURE.md)。

2026-07-24 的 recovery-model 更新已完成上述第一步的 IR 与 executor：目标 `StateSeed` 现在必须携带 terminal checkpoint 的 materialization-head persistence evidence；`HistoricalRecoverySet` 与 `recovery execute --set` 冻结 `retain-relevant-os-state` 并执行 before/after/head 三个独立控制；LangGraph recorded plan 还要求一个不同于 frontier after coordinate 的 native head coordinate。随后 executor 改为验证并克隆 profile 的 durable checkpoint snapshot，再以 exact source ID 在 fresh process restore；它不会重跑候选任务。V3 还要求 profile 显式保留 source container lease，并将 recovery container 加入该 source 的 PID/network namespace；只读 `/proc/net/unix` 与 holder-FD probe 把 profile 的 `exact-socket-id` bind/listen evidence 重新验证为唯一存活 listener。profile `1784896157047894121` 已完成这一 privileged live calibration：`Q_before` 为 `residual`，而 after/head controls 都为 `consistent`；源容器随后按 lease 释放。旧 V2 calibration 只验证 socket node metadata，因没有 runtime lease 仍是 `inconclusive`，不能作为 V3 evidence。full-vs-pruned 现在以同源配对和 batch report 实现：报告验证共享 recorded plan、source lease、snapshot、socket identity 与 native coordinates，并分别汇总 classification、exact layer/origin agreement、multiplicity policy 和 post-query probe cost。下一步是运行足量重复 trial、增加 objective family；该手工 baseline 不计入 generator discovery 或 coverage。MAF adapter 也仍只有 before/after binding，不能假装已经支持 set execution。

2026-07-24 的下一步实现为 StateFuzz 的单次 generated-candidate attempt：外置 generator 接收受限 objective/scaffold request，真实 profile 的 `CandidateEvaluation` 只以 bounded atom feedback 进入 repair attempt；候选只有经 eBPF/probe frontier 验证、native binding、StateSeed promotion 与 head control 后才可计入后续 coverage。当前尚不执行随机 non-frontier cut 对照，因为 LangGraph target 只有 before/after/head 三个可解释的 controller state labels；在增加独立且可标注的 non-frontier logical checkpoints 前，把 native checkpoint 随机化会混入未知 Agent state，不能作为 frontier-guidance 对照。

2026-07-25 的增量实现完成了第二个 LangGraph retained-resource family 的代码闭环：`handle.workspace-file.survival` 通过 exact `handle/open` evidence 绑定 materialization-head 的 regular workspace file，冻结 source file 的 device/inode/mode，并在 fresh recovery container 中只用 `lstat` identity probe 和 read-only bind mount 判断 residual。该 family 目前只支持 full passive mode。真实 generated-candidate experiment 已在 `runs/langgraph-workspace-file-contract` 完成：三个独立 candidate 全部经 profile、binding、StateSeed promotion 与完整 before/after/head recovery set 接受，且在不读文件内容的前提下得到相同的 `uncommitted-original-residual -> aligned -> aligned` relation vector。三次 causal evidence 均为 `unknown`，contract status 均为 `not-evaluated`，故它是可审计 relation evidence，不是漏洞或 contract conclusion。详细记录见 [LANGGRAPH_WORKSPACE_FILE_ATTEMPT_000.md](LANGGRAPH_WORKSPACE_FILE_ATTEMPT_000.md)。下一步是增加 family/repetition，并补足可适用的 causal provenance，而不是为该文件任务引入内容 Oracle。
