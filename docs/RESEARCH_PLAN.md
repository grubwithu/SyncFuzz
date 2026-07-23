# SyncFuzz v2 研究计划：状态目标驱动的 Checkpoint Frontier Fuzzing

状态：**当前路线**（2026-07-23）。本文取代旧路线中以 `primitive substitution`、`activation substitution`、`phase shift`、`cross-seed crossover` 为核心的变异计划。此前的设计与实验记录保留在历史分支和归档文档中，但不再作为新的实现或论文主张的基础。

本计划由 [ChatGPT-0723.md](ChatGPT-0723.md) 收束而来；后者是讨论记录，本文是可执行的规范。

## 1. 核心问题与新方法

SyncFuzz 要回答的问题是：

> 当 shell-enabled Agent 在一次执行中形成了可跨恢复边界存活的 OS 状态时，从该状态形成前后不同的 logical checkpoint 恢复，是否会得到不同的 Agent/OS 关系？

新的闭环为：

```text
State Objective
  -> task synthesis
  -> profiling execution
  -> eBPF + state-probe validation
  -> executable StateSeed
  -> checkpoint-effect frontier mining
  -> paired recovery queries (before / after)
  -> differential A/O classification
```

这包含两个相互独立、按顺序运行的搜索器：

1. **State Fuzzer**：为未覆盖的 OS 状态目标合成自然任务，并只保留经真实执行验证的状态形成实例。
2. **Checkpoint Frontier Fuzzer**：围绕已观测的持久 OS 状态变化，成对测试 frontier 前后的恢复点。

恢复测试的第一版只支持 `fork`。`replay` 与 `rewind` 是之后对同一 frontier 增加的 recovery-boundary 维度，而不是第一版的笛卡尔积。

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
| `StateSeed` | 可重放、满足 objective、且至少跨一个 checkpoint 存活的 `ProfileRun` | 否，自动晋升 |
| `RecoveryPair` | 同一 seed、同一 frontier、相同执行条件下的 before/after 两个 recovery query | 否 |
| `RecoveryQuery` | `<seed_id, boundary, checkpoint_id, passive_observation>` | 否 |

`StateObjective` 不是 prompt；`SynthesisCandidate` 不是 Query；`StateSeed` 不是人工 testcase；`RecoveryPair` 不是 Query genealogy。

对于一个 frontier `(C_i, C_{i+1}]`，第一版只产生：

```text
Q_before = <seed, fork, C_i, same passive observation>
Q_after  = <seed, fork, C_{i+1}, same passive observation>
```

两者的 task、recorded execution plan、topology、activation、oracle 和 probe schema 必须相同；**checkpoint 是唯一允许变化的字段**。两条结果用 `comparison_pair_id` 与 `frontier_id` 关联，不使用 parent/child 谱系。

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

同一 recovery query 的 initial branch 与 fork follow-up 留在同一个 container，以保留要判断的 OS residue；同一 `RecoveryPair` 的 before/after query 必须使用彼此独立的新 container。`local` 仅用于单元测试、fixture 和离线分析，不作为正式 profiling/coverage 结果的默认环境。初始 sandbox 保持非 root、无网络、`cap-drop=ALL`、`no-new-privileges`、CPU/内存/PID 限制；外部服务测试需要显式、单独的网络策略。

### 5.2 Frontier 选择与覆盖

frontier 分数仅用于调度，依据 persistence、capability creation、namespace mutation、lifetime escape 和 novelty。选择采用按 state family 分层的预算，而不是全局 Top-K，避免 Unix socket 等高频对象吞没其他状态面。

每个有效执行写入覆盖元组：

```text
<family, operation, lifetime, resource_relation,
 boundary, checkpoint_relation, outcome>
```

并周期性执行 full-vs-pruned probe 对照。pruned probe 的 verdict、resource identity、attribution 或 reconstruction 分类与 full probe 不一致时，不能将其作为可靠优化。

## 6. 两个搜索循环

### State Fuzzer

1. coverage scheduler 选择未覆盖或低覆盖的 `StateObjective`；
2. 通过通用 generator interface 合成一个正常的软件工程任务；LLM 只是可替换实现，不是 oracle；
3. Agent 执行候选，collector 与 probes 验证实际 effect；
4. 若 objective 未满足，记录缺失 atom 并 repair/regenerate；
5. 只有满足 effect、persistence、attribution、replayability 的候选才晋升为 `StateSeed`。

当前手写 LangGraph 任务仅用于校准 collector、adapter 和 oracle；它们不作为“靠手写 seed 覆盖状态面”的证据。自然性以人工抽样审查，不能由模型自述代替。

### Checkpoint Frontier Fuzzer

1. 从已晋升的 seed 提取并分层选择 frontier；
2. 建立 before/after `RecoveryPair`；
3. 使用固定的 passive observation 和相同 execution plan 重跑；
4. 用 deterministic evidence 分类 `consistent`、`residual`、`missing`、`duplicate`、`reconstruction` 或 `inconclusive`；
5. 将 negative paired result 也保留为 boundary-localization evidence。

`trusted action` 可以在确认 contract violation 后由人工做独立 case study，但它不进入上述 scheduler、coverage 或 Query 生成逻辑。

## 7. 实施里程碑

| 阶段 | 交付物 | 完成标准 |
| --- | --- | --- |
| V2.0 规范与清理 | 本文、v2 data-model audit、旧 mutation 标记为 legacy | 新代码不消费 `TargetScenarioMutation`、mutation-focus prompt variant 或 Query genealogy |
| V2.1a Evidence contract | checkpoint、raw event、normalized effect、checkpoint state summary、frontier JSON schema，raw-trace import，deterministic normalizer 与 fixture tests | **已实现**；不依赖特权 eBPF 也能从记录的 trace 得到稳定 frontier map |
| V2.1b Objective / pair / coverage IR | `StateObjective`、validated seed、recovery pair 与 coverage record schema | 这些对象不复用旧 Scenario mutation / Query genealogy |
| V2.2 Profiling collector | checkpoint monotonic timestamp、collector interface、Linux host-side eBPF adapter、per-run container/cgroup scope、core state probes | LangGraph calibration fixture 在独立 container 中生成可归因的 raw trace 与 `R(C)`；不支持 eBPF 或 cgroup v2 的环境明确失败而非静默降级 |
| V2.3 Fork frontier pairs | `frontier -> Q_before/Q_after` generator、recorded-plan recovery executor、generic paired classifier | Unix-listener calibration 证明 only-checkpoint-changes invariant，并能区分 residue / reconstruction / clean negative |
| V2.4 Execution-validated synthesis | objective grammar、coverage scheduler、generator command contract、candidate repair/retention | 新 StateSeed 只能由实际 trace 验证后进入 corpus；手写 fixture 不计覆盖 |
| V2.5 Breadth and fidelity | 分层 frontier selection、coverage ledger、full-vs-pruned calibration | 可报告各 family 的 objective、effect、frontier 与 boundary coverage，且明确支持范围 |
| V2.6 扩展 | replay/rewind、第二批 family、contract-profile automation | 每次只增加一个独立维度，并与 fork baseline 做受控比较 |

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

V2.1b 的独立 IR 已落在 `internal/syncfuzz/objective`、`recovery` 和 `coverage`：`StateObjective` 只接受 bounded effect atom、lifetime、resource relation 与 persistence；`StateSeed` 只能由每个 atom 均有 evidence link 的 persistent frontier 自动晋升；`RecoveryPair` 固定为同一 seed、同一 recorded plan artifact、同一 passive observation 的 fork before/after 对，checkpoint 是唯一可变字段；coverage 使用 `<family, operation, lifetime, resource_relation, boundary, checkpoint_relation, outcome>` 去重，绝不读取 legacy Scenario mutation、prompt variant 或 Query genealogy。每个 `ProfileRun` 必须显式标为 `synthesis-candidate` 或 `calibration-fixture`：只有前者可晋升为 `StateSeed`，后者即使是成功的真实 eBPF run 也只能校准 collector，绝不计入 coverage。`profile promote-seed` 可离线读取带 provenance 的 ProfileRun，也可导入一次完成的 target profiling artifact；导入时必须声明 provenance，`synthesis-candidate` 由 V2.4 scheduler 产生，不能为手写 smoke 标记。`profile recovery-pair` 只能复用 seed 锁定的 recorded plan artifact。实际 recorded-plan executor 属于 V2.3。

V2.3 的 executor core 已落在 `internal/syncfuzz/recovery`：`ForkExecutorRegistry` 只为真正暴露 durable Agent checkpoint 的 adapter 注册 executor；每个 `RecoveryObservation` 必须绑定原始 query、recorded plan 与 passive observation，并报告独立 `runtime_instance_id`。`ExecuteForkPair` 强制 before/after 使用不同 runtime instance，且只向 executor 传递不同的 checkpoint coordinate；其 deterministic classifier 输出 `consistent`、`residual`、`missing`、`duplicate`、`reconstruction` 或 `inconclusive`。`command` adapter 没有 Agent-native durable checkpoint，因此 registry 明确拒绝它；不能以 controller observation checkpoint 代替 fork execution。

第一个接入点是 `maf-workflow` recovery adapter，而不是 legacy `target run` 的 generic command adapter。它调用 MAF Workflow 的 `FileCheckpointStorage`，在准备阶段形成两个真实、文件持久化的 native checkpoint（effect 之前和之后）；每个 query 都复制准备好的 initial workspace、重建新的 `Workflow` 对象，并用一个精确 native checkpoint ID 进行 restore。adapter plan 显式保存 V2 checkpoint coordinate 到 native MAF ID 的 binding，因而 recovery executor 不能把 controller checkpoint ID 直接交给 MAF。`make maf-workflow-native-fork-smoke` 是该 integration 的 live calibration：它产生 `prepared`、`before`、`after` 三个独立 workspace 与各自的 restore observation。`runs/maf-v2.3-fork-smoke-2` 已成功验证该路径：`v2-start` queue 的 native checkpoint `31f70e81-…` 在 fresh runtime 中 re-execute Plant，得到 `agent=absent, os=present, origin=reconstructed`；`v2-plant` queue 的 checkpoint `e58a22b6-…` 不重放 Plant，得到 `agent=present, os=present, origin=residual`。两者 runtime identity 不同，且均有 MAF restore callback 与重建 Workflow object 的 evidence。该 calibration 的 paired classifier 语义为 `before=reconstruction`、`after=consistent`、总体 `reconstruction`，因此它是预期的 clean calibration，不是 violation。此校准尚是 local fixture，不是 StateSeed、coverage 或 container profiling claim；把 synthesis-generated ProfileRun 映射到 native MAF coordinate 并纳入每-query container isolation 是 V2.4 的工作。

V2.4a 的 synthesis contract 已落在 `internal/syncfuzz/synthesis`。coverage scheduler 只按 objective atom 在 V2 coverage ledger 中的 `<family,operation>` 稀缺性排序；generator command 接收 `SYNCFUZZ_SYNTHESIS_REQUEST` 所指向的 bounded JSON request，并只能在 stdout 返回一个自然任务 JSON。scheduler 为 task 计算 canonical `SynthesisCandidateID`；generator 无法指定 target、adapter、candidate ID、mutation、prompt variant 或 parent query。`ProfileRun(kind=synthesis-candidate)` 与 `StateSeed` 现在都必须携带该 candidate ID，且 `profile promote-seed` 必须同时收到相匹配的 scheduler candidate artifact；因此历史 target run 不能只靠填写 `--profile-kind` 进入 seed corpus。`synthesis evaluate` 仅接受 linked persistent frontier 作为实际 effect，输出 missing-atom feedback 供下一次 attempt 使用；`synthesis promote` 只有在 candidate/profile identity 一致且所有 objective atom 都已验证时才会保留 seed。`synthesis bind-maf-frontier` 已将一个 profile frontier 显式映射到 MAF native checkpoint ID；它要求 profile 的 `native_checkpoint_run_id` 与 manifest 的 initial runtime identity 一致，并验证 MAF 持久化的 `v2-start` / `v2-plant` queue coordinate，拒绝把无关 profile 绑到便利的 checkpoint fixture。当前没有内建 LLM 或默认 generator。

LangGraph 已成为第一条真实 candidate execution 路径：`synthesis execute-langgraph` 只接收 scheduler-issued candidate，以候选的 `task` 作为真实 Agent prompt，在专用镜像的独立 container 中同时开启 process/resource eBPF profiling，并写出 candidate-bound `ProfileRun`。wrapper 强制使用 disk checkpointer，额外记录 `langgraph-native-checkpoints.json`：其中的 `native_checkpoint_run_id` 和精确 LangGraph checkpoint ID 与 controller 的 profiling checkpoint 分开保存。每次 durable `put` 还记录同一 `CLOCK_MONOTONIC` 域的 `persisted_monotonic_ns`。`synthesis bind-langgraph-frontier` 只在同一 native runtime、同一 validated frontier 中，以每个 linked objective effect 的时间窗选择严格前后的 native checkpoint；历史索引、checkpoint 文件顺序或 controller checkpoint 名均不能替代此证据。该 mapping 已由 live profile `1784813806441091527` 校准：`before-command..after-command` 被映射到精确的 native before/after ID。binding 现同时写出每个 native ID 的 structural coordinate（history index、message count、`next`）；fresh runtime 绝不复用 profile ID，而是必须在新 initial runtime 中将该完整 coordinate 解析为唯一的新 ID。wrapper 已支持该失败即停的 resolution 和无 Agent follow-up 的 passive recovery observation，并对一个 workspace-contained Unix endpoint 做固定、只读的 `lstat` identity observation。`synthesis prepare-langgraph-fork` 现把 candidate task、model/image、runtime root、固定 passive observation 与两个 structural coordinate 冻结为 LangGraph recorded plan，并将 bound ProfileRun 指向该 plan；credentials 不会进入 plan。下一步是执行该 plan：每个 query 建立独立 container，在容器内运行 initial+fresh-resume 两个 Python process，再把 observation 接回 paired classifier。

## 9. 实验与论文证据

在宣称新方法有效前，需要分别完成：

1. **Synthesis validity**：目标 effect 的生成成功率、持久性、可重放性与自动 coverage 增量；
2. **Frontier guidance**：eBPF-selected frontier pair 相比随机或非 frontier checkpoint pair 的有效 A/O relation / localization 产出；
3. **Breadth**：按已声明 family 与 effect grammar 报告 coverage，禁止以 testcase 数量替代；
4. **Probe fidelity**：full 与 pruned probe 的分类一致率、漏资源率和开销；
5. **Recovery semantics**：fork first，随后才在相同 seed/frontier 上评估 replay 与 rewind。

所有 verdict 依赖可审计的 trace、probe 和 deterministic oracle。Recovery contract 自动生成仍是独立问题：它可为 oracle 提供期望语义，但不替代 effect validation 或 frontier selection。

## 10. 与既定五项任务的对应

| 任务 | v2 处理方式 |
| --- | --- |
| 根因分析 | 已完成；作为 calibration fixture / case study，不再扩展为 mutation 主线 |
| Mutator | 改为 objective-driven task synthesis 与唯一合法的 checkpoint-coordinate recovery pair |
| Oracle / Contract 自动化 | 保留为后续 contract-profile 工作；当前先以 deterministic A/O 分类保证证据闭环 |
| Violation / Seed 分类 | 由 effect grammar、validated StateSeed 与 coverage ledger 给出，而非手工 testcase 标签 |
| eBPF 引入 | 作为 profiling 与 frontier mining 的核心发现信号；state probe 负责持久性确认 |

## 11. 紧接着要做的工作

第一层 **FD→`(device,inode)` identity probe** 已实现并完成 privileged live calibration；deleted-open-FD 的 collector effect 与 checkpoint probe 已形成 `exact-device-inode` link。Unix socket 的 namespace/FD identity 与 dependency closure 也已由 `1784805732832067342` 完成 privileged live calibration：`bind` / `listen` 经 `exact-socket-id` 关联到完整 endpoint closure。canonical-path、FD identity 与 Unix socket 的首轮 known-answer audit 均已完成，并由可重跑的 `profile calibration-audit` 输出 fixture-scoped precision/recall。V2.1b Objective / pair / coverage IR 与 provenance gate 已完成；V2.3 的 MAF-native durable-checkpoint recovery adapter 已完成 live fixture calibration；V2.4a 已实现 objective scheduler、generator contract、candidate provenance/retention gate，以及 logical-frontier 到 native MAF checkpoint 的 identity binding。LangGraph 现已接入真实 candidate 的 isolated, eBPF-profiled execution，并保留 initial durable runtime 的精确 checkpoint catalog；这一步不会把 controller checkpoint 冒充为 Agent checkpoint。LangGraph native-frontier mapper 已由 `1784813806441091527` 完成 live calibration：它要求同一 `CLOCK_MONOTONIC` 域的 native durable-save 时间戳，并只接受严格包围 linked objective-effect window 的 native checkpoint 对。wrapper 已补齐 fresh-process 的 exact-native-ID selection 与固定 Unix-socket metadata observation；接下来要将这些输入冻结为 LangGraph recorded plan，并在每个独立 container 中执行 before/after recovery pair。`command` adapter 仍被明确排除，不能把 controller observation checkpoint 当作恢复点。V2.5 再以 full-vs-pruned 与新增 family 扩展 fidelity/breadth 实验。collector 与 controller checkpoint 只能产生可审计 evidence：它们不单独决定漏洞 verdict 或 StateSeed 晋升。当前没有内建 LLM generator，不新增 trusted-action，也不把任何手写 smoke input 晋升为 StateSeed。
