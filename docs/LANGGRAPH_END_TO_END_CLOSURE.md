# LangGraph 端到端闭环：从状态目标到 Historical Checkpoint Recovery

> 汇报快照：2026-07-24  
> 对应代码：`056e30d`（`master`）  
> 对应主运行：`runs/langgraph-v2.4/manual-baseline/native-timing/1784813806441091527`

本文以目前唯一一条已完整跑通的 LangGraph 路径为主线，解释 SyncFuzz v2 到底在做什么、每一个 artifact 代表什么、已经证明了什么，以及还没有证明什么。它是明天汇报当前开发进度的技术底稿，而不是论文实验结果表。

## 1. 一句话结论

我们已经让一个真实的 shell-enabled LangGraph Agent 完成下面这条链：

```text
StateObjective
  -> 带来源标识的 SynthesisCandidate
  -> 隔离容器中的真实 Agent 执行
  -> host-side eBPF + checkpoint state probe
  -> linked persistent frontier
  -> StateSeed
  -> frontier 两侧与 logical head 的原生 LangGraph durable checkpoint
  -> 冻结的 recorded historical-recovery plan（当前 adapter: fork）
  -> 两个独立容器中的 fresh-runtime recovery query
  -> 固定 passive observation
  -> deterministic paired classification
```

这证明了 v2 的**证据与恢复闭环可以端到端运行**：不是由 SyncFuzz 预先写好“漏洞利用后果”，也不是把 controller 的观察点冒充为 Agent checkpoint，而是先在真实执行中确认一个持久 OS effect，再围绕该 effect 前后的**框架原生 durable checkpoint**做恢复对照。

本轮最终分类是 `inconclusive`。这是一个正确且有意的保守结论：当前 Unix socket passive probe 只观察 metadata，不能确定 effect multiplicity（single / duplicate），因此系统不会把“逻辑状态 absent、OS socket present”直接宣传成漏洞。

## 2. 要回答的问题与最小实验单位

SyncFuzz v2 研究的问题是：

> 一次 Agent 执行形成了可跨恢复边界存活的 OS 状态后，从该状态形成前后不同的 logical checkpoint 恢复，Agent logical state 与 OS state 的关系是否不同？

对 SyncFuzz 而言，核心不是产品把该操作叫 `fork`、`rewind` 还是 `replay`，而是从 head `H` 对一个历史 cut `C` 恢复 logical state，同时保留 relevant OS head state：

```text
initial materialization: <A_H, O_H>
historical recovery:    <A_C, O_H>, where C < H
```

若 `C..H` 之间没有持久 OS effect，就没有 rollback-induced A/O desynchronization 的对象；若产品 API 重放 effect、销毁 OS runtime 或 clone OS namespace，则它实现的是不同的 retention / re-execution semantics，不能与 retain-state recovery 混合。

目标实验对一个 frontier `(C_i,C_{i+1}]` 产生三个 controls：

```text
Q_before = <seed, H, C_i,     retain relevant OS state, W, mechanism>
Q_after  = <seed, H, C_{i+1}, retain relevant OS state, W, mechanism>
Q_head   = <seed, H, H,       retain relevant OS state, W, mechanism>
```

其中 historical checkpoint cut 是唯一 discovery 变量。任务、模型、容器镜像、目标 adapter、retention policy、被动观察方式和 recorded plan 必须保持一致。当前 LangGraph vertical slice 已实现 `Q_before/Q_after`，且使用 LangGraph fork 作为 mechanism；`Q_head` 与显式 head/retention artifact 仍待实现，故当前 artifact 名称仍为 `RecoveryPair`。

这与旧路线有根本区别：旧 mutation 路线会改 prompt profile、场景 primitive、activation 或 trusted follow-up；v2 不把这些变化当作新的 recovery query。它们可能仍是历史 regression fixture 或后续 case study，但不构成 StateSeed 发现或 coverage claim。

## 3. 本次闭环的具体输入

### 3.1 StateObjective：要验证的状态关系，而不是 prompt

本次 objective 文件是 [`examples/objectives/unix-listener-survival.example.json`](../examples/objectives/unix-listener-survival.example.json)，语义为：

| 字段 | 值 | 含义 |
|---|---|---|
| `objective_id` | `ipc.unix-listener.survival` | 该类状态目标的稳定 ID。 |
| effect atom 1 | `ipc/bind` | Agent 必须实际绑定 Unix-domain socket。 |
| effect atom 2 | `ipc/listen` | Agent 必须实际开始监听，而不只是创建普通路径。 |
| `lifetime` | `survive-tool-return` | Shell tool 返回后，服务仍应存活。 |
| `resource_relation` | `fixed-path-served-by-descendant` | 固定路径的 endpoint 由后代进程持有并提供服务。 |
| `persistence` | `across-checkpoint` | 状态必须横跨至少一个 profiling checkpoint。 |

这里的 `StateObjective` 只规定有限的 effect grammar 与资源关系，不规定“写哪个 Python 文件”、具体 socket 名称或如何攻击它。这一点很重要：目标空间应是 OS 状态面，而不是手写漏洞故事的枚举。

### 3.2 SynthesisCandidate：一次任务尝试

本次 candidate 为：

| 字段 | 值 |
|---|---|
| `candidate_id` | `synthesis-candidate:3438f7305e45b379` |
| `target_id` / `adapter_id` | `langgraph-shell-react` / `langgraph` |
| `generator_id` | `manual-langgraph-baseline-v1` |
| `scaffold_artifact` | `examples/synthesis/langgraph-shell-react-scaffold.example.json` |
| task | 在 workspace 创建 `agent.sock` 上的 Unix-domain socket echo service；后台运行；另行连接验证；把回复写入 `unix-listener-result.txt`；结束时保留服务。 |

候选的完整自然语言任务是：

> Within the current workspace, create a small Unix-domain socket echo service at `agent.sock` and run it in the background. Use a separate local shell command to connect to it and verify that it responds. Write the response to `unix-listener-result.txt`, and leave the service running when you finish.

**需要如实说明的限制：**该 candidate 的 `generator_id` 是 `manual-langgraph-baseline-v1`。它拥有正式 candidate ID，并走了同一 provenance / validation / retention 数据通路，但它是为了打通系统而人工提供的 baseline candidate。因此本闭环证明的是执行、证据和恢复机制；它**不**证明外部 LLM generator 的合成成功率、自然性或 coverage 增益。

### 3.3 真实 Agent 执行条件

主 profile run 的 ID 是 `1784813806441091527`。它的 target task artifact 记录：

| 项 | 值 |
|---|---|
| 环境 | Docker container |
| target image | `syncfuzz-langgraph:dev` |
| Agent | LangGraph Shell ReAct target |
| 模型 | `openai:deepseek-v4-flash` |
| checkpoint backend | disk-backed durable checkpointer |
| process mode | `single`（profile 阶段） |
| tool-use requirement | `--require-tool-use` |
| target timeout | 120 秒 |
| 网络 | 显式允许，仅用于模型 provider |
| workspace | 容器内 `/workspace` |

目标容器负责运行 Agent 和 shell；eBPF collector 不进入目标容器。collector 在宿主机上以该运行容器的 cgroup identity 过滤事件，因此 host 上的无关进程事件不应进入该 run 的 raw trace。正式 profiling 需要 Linux cgroup v2、tracepoint access 以及 `CAP_BPF` / `CAP_PERFMON` 或 root 权限；缺少这些条件时应显式失败，而不是无声降级。

## 4. 参与者、层次与数据流

```text
                    ┌─────────────────────────────────────────┐
                    │ SyncFuzz controller (Go)                │
                    │ candidate / profile / seed / pair        │
                    └───────────────┬─────────────────────────┘
                                    │ starts isolated target
 ┌──────────────────────────────────▼──────────────────────────────────┐
 │ Docker target container                                               │
 │  LangGraph Shell ReAct Agent                                          │
 │    ├─ persistent shell tool                                           │
 │    ├─ disk-backed LangGraph checkpointer                              │
 │    └─ descendant echo-server process -> /workspace/agent.sock        │
 └───────────────┬───────────────────────────────┬──────────────────────┘
                 │ OS syscalls / process events  │ workspace snapshots
 ┌───────────────▼───────────────────────────────▼──────────────────────┐
 │ host-side observers                                                   │
 │  eBPF process/resource collector + controller checkpoint state probe │
 └───────────────┬──────────────────────────────────────────────────────┘
                 │ normalized effects, resource identities, R(C)
                 ▼
       checkpoint-effect map -> StateSeed -> native checkpoint binding
                 │
                 ▼
   two fresh recovery containers, one for Q_before and one for Q_after
                 │
                 ▼
      passive Unix-socket metadata observation -> paired classifier
```

需要区分三个时间/状态层：

1. **Agent logical layer**：LangGraph 的 durable checkpoint 保存的 message/state graph 语义。
2. **OS layer**：进程、FD、Unix socket、路径名等。它们不由 LangGraph checkpointer 自动回滚。
3. **SyncFuzz observation layer**：controller checkpoint、eBPF event 和 probe summary。这一层提供证据和 frontier，但它自身不是 Agent 的可恢复 checkpoint。

## 5. 术语表与概念关系

| 概念 | 精确定义 | 在本例中的实例 | 不能误解为 |
|---|---|---|---|
| `StateObjective` | 人工维护的、有限 effect atom 与持久性关系的声明。 | `ipc.unix-listener.survival`。 | 一个具体 prompt。 |
| effect atom | objective 中一个 `(family, operation)` 要求。 | `ipc/bind`、`ipc/listen`。 | LLM 的文字自述。 |
| `SynthesisCandidate` | 对一个 objective 的一次自然语言任务尝试；由 ID 绑定来源。 | `synthesis-candidate:3438…`。 | recovery query 或 seed。 |
| `ProfileRun` | **不做 recovery**的完整真实 Agent 执行及其 profiling evidence。 | `target-profile:1784813806441091527`。 | 一条纯离线 fixture。 |
| raw event | eBPF 收到的 kernel-visible 原始记录。 | `bind`、`listen`、`dup`、`openat`、process lifecycle。 | 已经证明持久性的事实。 |
| `NormalizedEffect` | 将 raw event 归一到有限语义族后的 effect。 | `ipc/bind`、`ipc/listen`。 | 只按 syscall 数量筛出的噪声。 |
| controller checkpoint | SyncFuzz 为 snapshot / timeline 设置的观察点。 | `before-command`、`after-command`、`after-observation`。 | LangGraph native checkpoint。 |
| `StateSummary(C)` | checkpoint `C` 时 probe 确认存在的持久资源及依赖闭包。 | socket、路径、FD、holder process。 | 单个文件是否存在。 |
| persistent delta `ΔR` | `R(C_after) - R(C_before)`。 | 新增 Unix endpoint、FD、holder process 等。 | 任意发生在区间中的 syscall。 |
| `evidence_link` | 一个 effect 与 `ΔR` 中具体资源之间的精确身份连接。 | `exact-socket-id`、`exact-device-inode`。 | 字符串模糊匹配。 |
| `CheckpointFrontier` | 有 linked persistent delta 的相邻 controller checkpoint 区间。 | `before-command..after-command`。 | 任意 checkpoint pair。 |
| `MaterializationHead` | initial branch 已执行完成、目标 effect 仍在 OS state 中的 logical head `H`。 | 本例中是每条 recovery query 自己 initial branch 的结束状态。 | profile run 原容器的物理 OS instance。 |
| `StateSeed` | candidate profile 中，满足 objective 且至少跨一个 checkpoint 存活的 validated frontier。目标模型还要求它绑定 effect 在 head 仍存在的证据。 | `state-seed:target-profile:…`。 | 人手写 testcase。 |
| native checkpoint | LangGraph 自己持久化的恢复状态。 | `1f1869b9-…`。 | controller 的 `before-command`。 |
| native binding | 将一个已验证 frontier 以 monotonic timestamp 关联到原生 checkpoint 对。 | `langgraph-native-frontier-binding.json`。 | 按 history 顺序猜测 checkpoint。 |
| native coordinate | fresh runtime 中查找等价 native state 的结构描述。 | `message_count` + `next`；旧 ID/历史 index 仅保留 provenance。 | 将旧 runtime ID 直接传给新 runtime。 |
| retention policy `ρ` | 恢复时 relevant OS state 如何处理的显式条件。 | 目标条件为 `retain relevant OS state`。 | 产品 API 名称。 |
| recovery mechanism `μ` | adapter 用来实现 recovery 的产品 API / protocol。 | 当前为 LangGraph fork。 | discovery 搜索轴。 |
| recorded plan | recovery 时必须冻结的 task、model、image、probe、head/retention 条件和 checkpoint coordinate。 | 当前 `langgraph-fork-plan.json` 尚只冻结两条 coordinate。 | 含 credential 的执行脚本。 |
| `RecoveryQuery` | 目标模型为 `<seed, H, C, ρ, W, μ>`。 | before、after、head 三种 control。 | candidate 的 parent/child 谱系。 |
| `HistoricalRecoverySet` | 同一个 seed/head/retention/plan/probe 下的 before/after/head controls。 | 目标 artifact；当前以 `recovery-pair.json` 实现 before/after 子集。 | 两个随意独立运行。 |
| passive observation | 不改变被观测状态的固定观察。 | 对 `agent.sock` 做 `lstat`。 | 主动连接 echo server。 |
| runtime instance | 一次 query 独享的恢复容器与 workspace。 | `…3094571268`、`…1228889475`。 | 初始 profile 的容器。 |
| paired classification | 基于确定性 A/O evidence 的结果分类。 | 此处 `inconclusive`。 | LLM 对“是否有漏洞”的判断。 |

概念依赖顺序为：`Objective -> Candidate -> ProfileRun -> Frontier -> StateSeed + MaterializationHead -> Binding -> RecordedPlan + HistoricalRecoverySet -> RecoveryExecution`。后一阶段不能跳过前一阶段的 ID、provenance 和 validation。当前代码仍以 `RecoveryPair` / `ForkPairExecution` 表示该结构的 before/after 兼容子集。

## 6. 主运行的逐步拆解

### 6.1 第一步：初始真实 Agent 执行

`synthesis execute-langgraph` 接收 objective 与 candidate，生成一次候选绑定的 target run。运行时：

1. controller 在新的 Docker workspace 中写入 prompt / task contract；
2. LangGraph target 使用 disk-backed checkpointer 创建 durable state；
3. Agent 必须真实调用 shell tool，不能只在自然语言中声称完成任务；
4. Agent 创建 `echo_server.py`，启动后代 echo server，绑定并监听 `/workspace/agent.sock`；
5. Agent 再用独立 shell command 连接 socket，把响应写到 `unix-listener-result.txt`；
6. shell tool 返回后，服务与 endpoint 应仍存在；
7. collector、controller checkpoint、workspace/process/FD probe 同时记录证据。

`target-result.json` 已确认 target command 成功完成，且立即的 expected-file 检查通过。这个完成状态只是执行成功的必要条件；是否形成 StateSeed 仍要看后续 effect 与 persistence evidence。

### 6.2 第二步：profiling 的两个观察面

**eBPF raw trace 回答“区间内发生了什么”。**resource collector 在 cgroup 范围内记录成功的 resource syscall，process collector 记录 fork / exec / exit。其输出包括 `ebpf-resource-events.jsonl`、`ebpf-process-events.jsonl` 和相应 scope artifact。

**state probe 回答“checkpoint 时什么仍存在”。**controller 在 checkpoint 采集 workspace metadata、live process、open FD、Unix endpoint 等，形成 `checkpoint-state-summaries.json`。对 Unix socket，probe 不是只记一个 pathname，而是记录依赖闭包：

```text
workspace:agent.sock
  <- bound-at-path - unix-socket:socket:178185910
  <- references-unix-socket - container-fd:60:3:device:8:inode:178185910
  <- held-by-process - container-process:60
```

含义是：`agent.sock` 这个名字、内核 Unix socket、持有它的 FD、持有 FD 的进程被分开建模，再由 dependency relation 连接。这样可避免把“路径残留”误判为“仍有可服务的监听 socket”。

### 6.3 第三步：从 raw event 到已验证 frontier

主 run 的 controller interval 为：

| interval | controller monotonic 范围 | 是否 frontier | 说明 |
|---|---:|---|---|
| `before-command..after-command` | `4416194811343231..4416225346295993` | 是 | 出现 linked persistent delta。 |
| `after-command..after-observation` | `4416225346295993..4416226093041987` | 否 | 没有 persistent delta。 |

目标 effect 的关键 kernel-visible 记录为：

| monotonic ns | process | normalized effect | 身份 |
|---:|---|---|---|
| `4416205992104239` | host PID `2821885` (`python3`) | `ipc/bind` | FD 3、`socket:178185910`、`(device=8,inode=178185910)` |
| `4416205992110483` | host PID `2821885` (`python3`) | `ipc/listen` | 同一 `socket:178185910` |

前面可能还有其他 `bind`、`openat` 或运行时初始化 syscall；它们不能单独使区间成为 StateSeed evidence。只有同时满足下列条件的 effect 才被 objective 使用：

1. effect 的 family/operation 与 objective atom 相同；
2. effect 落在该 frontier 的时间范围内；
3. effect 与 `ΔR` 中资源形成精确 link；
4. 资源在 probe 中被确认跨 checkpoint 存在。

本例的关键 evidence links 为：

| effect | resource | link relation | 为什么可靠 |
|---|---|---|---|
| `bind` | `unix-socket:socket:178185910` | `exact-socket-id` | 内核 socket ID 精确一致。 |
| `listen` | `unix-socket:socket:178185910` | `exact-socket-id` | 同上。 |
| socket-related FD effect | `container-fd:60:3:device:8:inode:178185910` | `exact-device-inode` | 不依赖可变 FD number 或路径字符串。 |

于是 `before-command..after-command` 的 `ΔR` 中不仅有 `workspace:agent.sock`，还包含 Unix endpoint、FD、holder process 及其闭包关系。该 interval 才成为有意义的 checkpoint-effect frontier。

### 6.4 第四步：ProfileRun、评估与 StateSeed 晋升

profile 形成：

```text
ProfileRun ID:          target-profile:1784813806441091527
kind:                   synthesis-candidate
candidate ID:           synthesis-candidate:3438f7305e45b379
native runtime ID:      langgraph-native-runtime:1784813806441091527
frontier:               before-command..after-command
validated atoms:        ipc/bind, ipc/listen
```

`synthesis evaluate` 的判据是“每个 objective atom 是否在 linked persistent frontier 上被观察到”，而不是“模型说自己创建了服务”。本 candidate 的历史 evaluation 记录了：

```text
validated_frontiers: [before-command..after-command]
observed_effects:    [ipc/bind, ipc/listen]
missing_effects:     []
eligible_for_retention: true
```

`StateSeed` 将 candidate、profile、objective、frontier、controller checkpoints、resource IDs、recorded plan ID 绑定为一个不可随意拼接的恢复输入：

```text
seed_id = state-seed:target-profile:1784813806441091527:
          before-command..after-command
```

它不是“这个 socket 漏洞的 testcase”，而是“该 candidate 真实形成了一个能满足 objective 的、可用于恢复比较的持久状态实例”。

### 6.5 第五步：为什么需要 native checkpoint binding

controller checkpoints 仅仅告诉我们何时做了 SyncFuzz snapshot；LangGraph 恢复时却只能使用 LangGraph 自己的 durable state。因此不能把 `before-command` 直接交给 framework restore。

wrapper 对每次 durable checkpointer `put` 记录：

```text
<exact native checkpoint ID,
 persisted_monotonic_ns,
 history index,
 message count,
 next nodes>
```

这些 `persisted_monotonic_ns` 与 eBPF / controller evidence 都在 `CLOCK_MONOTONIC` 域。绑定器只选择严格包围**已链接目标 effect window**的 native checkpoint：

| 项 | before native checkpoint | objective effect window | after native checkpoint |
|---|---:|---:|---:|
| persisted monotonic ns | `4416205970252825` | `4416205992104239..4416205992110483` | `4416206332623238` |
| source native ID | `1f1869b9-40c7-69e3-8008-652ee06faf48` | — | `1f1869b9-4441-6ca4-8009-5d7b62e46f73` |
| message count | 8 | — | 9 |
| `next` | `[tools]` | — | `[model]` |

因此 binding 证明的是：before native checkpoint 已在 `bind/listen` 之前耐久化，after native checkpoint 只在两个 effect 都发生之后才耐久化。它不是按 checkpoint JSON 文件顺序、history index 或 controller checkpoint 名字猜出来的。

### 6.6 第六步：fresh runtime 不能复用旧 checkpoint ID

LangGraph checkpoint ID 是一次 runtime 的本地分配值。把 profile run 的 ID 传入新的 container 没有意义，也会错误地把“同一字符串”误认为“同一 logical state”。

因此 `LangGraphNativeCheckpointCoordinate` 保存：

```json
{
  "source_checkpoint_id": "旧 runtime 的 ID（仅 provenance）",
  "history_index": "旧 runtime 的观察索引（仅 provenance）",
  "message_count": 8 或 9,
  "next": ["tools"] 或 ["model"]
}
```

fresh runtime 的 resolver 不使用 `source_checkpoint_id`，也不要求 history index 完全相同；模型可能因无害 setup turn 产生不同的 history 排列。它在 fresh durable history 中寻找 `message_count` 与 `next` 同时匹配的 checkpoint，并要求**恰好一个**匹配，否则失败退出。这里的“等价”严格说是当前 adapter 定义的**结构坐标等价**，不是对完整 Agent state 做 cryptographic equality assertion。

本轮实际解析到了新的 ID：

| query | profile 中旧 ID | fresh runtime 中新 ID | 恢复 shape |
|---|---|---|---|
| before | `1f1869b9-40c7-…` | `1f186a3e-bc3c-6a1e-8008-2a1b0945006b` | 8 messages, `[tools]` |
| after | `1f1869b9-4441-…` | `1f186a3f-8c79-6979-8009-02f0ee2cb4fc` | 9 messages, `[model]` |

“exactly one match or fail” 是必要的 fail-closed 规则：如果一个 shape 在 fresh history 中对应多个 durable checkpoint，SyncFuzz 不会任选一个来制造结论。

### 6.7 第七步：冻结 recorded historical-recovery plan 与形成当前 RecoveryPair

`synthesis prepare-langgraph-fork` 把当前 LangGraph fork adapter 所需的不可变输入写为 `langgraph-fork-plan.json`。它是目标 historical-recovery plan 的当前 before/after 子集：

| 冻结字段 | 本例值 / 作用 |
|---|---|
| candidate task | 原始 Unix echo service 任务。 |
| model | `openai:deepseek-v4-flash`。 |
| image | `syncfuzz-langgraph:dev`。 |
| runtime root | recovery workspace 的宿主目录。 |
| passive observation | `unix-socket-metadata:agent.sock`。 |
| two coordinates | before: 8 / `[tools]`；after: 9 / `[model]`。 |
| agent logical projection | before `absent`；after `present`。 |
| head / retention contract | **当前尚未显式记录**；query 内实际通过 initial branch 后保持同一 workspace 来实现 retain-state 条件。 |

credential 不写入 plan 或任何 JSON artifact。executor 只从启动自身的环境中选择性转发模型 provider 所需的变量。

接着 `profile recovery-pair` 形成：

```text
Q_before: <same seed, fork, before-command, unix-socket-metadata:agent.sock>
Q_after:  <same seed, fork, after-command,  unix-socket-metadata:agent.sock>
```

两条 query 的 candidate、StateSeed、frontier、plan、passive observation 都相同；`before-command` / `after-command` 是唯一不同的 coordinate key。它们是目标 recovery set 的 `Q_before/Q_after`，但尚没有 `Q_head=<H,H,…>` negative control。

### 6.8 第八步：执行 before / after historical recovery query

`recovery execute` 通过已注册的 LangGraph `ForkExecutor` 依次执行两条 query。这里的 `ForkExecutor` 是当前 adapter 的实现名称；方法层面它执行的是 historical recovery。每条 query 都会：

1. 在 runtime root 下建立全新的 workspace；
2. 以不同 Docker container 启动一次 initial process，完成同一个 task 并写入该 query 自己的 disk checkpoint store；
3. 在**同一 query 的同一 container/workspace**中启动 fresh resume process；这保留该 query materialization head 的 OS residue；
4. fresh resume process 按 coordinate 唯一解析本次 initial process 新产生的 native checkpoint；
5. 不添加 follow-up user message，不调用 fork tool action；
6. 对 `agent.sock` 在 passive observation 前后只做 `lstat`；
7. 写出 `langgraph-recovery-observation.json`；
8. controller 验证 query / plan / runtime ID 绑定关系后，才接受 observation。

before 与 after 必须是不同的 runtime instance。本轮实际 ID 是：

```text
Q_before runtime = langgraph-fork-syncfuzz-langgraph-fork-3094571268
Q_after  runtime = langgraph-fork-syncfuzz-langgraph-fork-1228889475
```

这两个 ID 不同，且每个 observation 均记录 `runtime_recreated: true` 与一个 fresh native checkpoint ID。因此没有把两条 query 偷偷放在同一个 long-lived runtime 里比较。也因此它们不是共享同一个物理 `O_H`：实际比较的是两个 independently materialized `<A_C^(q),O_H^(q)>`。后续 head contract 必须定义并验证哪些状态关系在这些 `O_H^(q)` 之间可比。

## 7. 被动观察到底观察了什么

本例 passive observation 是 `unix-socket-metadata:agent.sock`。它的实现严格为 workspace 内目标路径的 `lstat`，记录：

```text
exists
is_unix_socket
device
inode
mode
CLOCK_MONOTONIC timestamp
```

它**不会**：

- 连接 Unix socket；
- 发送 echo request；
- 创建、unlink 或替换 socket；
- 要求 Agent 再运行一个 shell tool；
- 根据模型文字回复猜测服务是否存在。

这样设计是为了避免 observation 本身改变被观测 OS state。例如连接服务可能消费一条应用请求，或者触发一次新的日志/文件写入；那时 before/after 的唯一区别就不再是 checkpoint。

在每条 query 中，resume 前后看到同一 `(device,inode)` 的 Unix socket：

| query | before observation | after observation | `same_endpoint_identity` |
|---|---|---|---|
| before | socket, `(2049,51668602)` | socket, `(2049,51668602)` | `true` |
| after | socket, `(2049,51668625)` | socket, `(2049,51668625)` | `true` |

两个 query 的 inode 不同是正常的：它们是不同的 Docker container / workspace，绝不应期待跨容器 identity 相同。`same_endpoint_identity` 只说明**单条 query 内** passive observation 没有重新创建或替换 endpoint。

## 8. Agent state、OS state 与本轮结果

### 8.1 三个独立字段

每个 `RecoveryObservation` 都独立记录：

| 字段 | 问题 | 本例的来源 |
|---|---|---|
| `agent_state` | 被恢复的 logical checkpoint 是否包含目标 effect 后的 Agent state？ | timestamp-validated binding 所导出的 before/after logical projection。 |
| `os_state` | passive probe 是否仍看到目标 OS resource？ | `lstat(agent.sock)`。 |
| `os_state_origin` | OS resource 是边界残留还是在观察期间重建？ | 同一 query 内 passive probe 前后 identity。 |
| `effect_multiplicity` | effect 是 single 还是 duplicate？ | 需要专门、确定性的 multiplicity evidence。 |

注意 `agent_state` 在本例是一个受 timestamp binding 约束的 logical projection：before durable checkpoint 在 `bind/listen` 前保存，所以标为 `absent`；after checkpoint 在 effect 后保存，所以标为 `present`。它不是用 OS socket 反推出来的，也不是一次新的 LLM 语义判断。

### 8.2 实际 observation 与分类

| query | restored shape | `agent_state` | `os_state` | origin | multiplicity | point outcome |
|---|---|---|---|---|---|---|
| `Q_before` | 8 messages, `[tools]` | `absent` | `present` | `residual` | `unknown` | `inconclusive` |
| `Q_after` | 9 messages, `[model]` | `present` | `present` | `residual` | `unknown` | `inconclusive` |

pair artifact 的总体结论也是：

```json
{
  "before_outcome": "inconclusive",
  "after_outcome": "inconclusive",
  "outcome": "inconclusive"
}
```

### 8.3 为什么 before 看起来像 desynchronization，却仍是 `inconclusive`

若系统已确定 `effect_multiplicity = single`，则 before 行的组合：

```text
agent_state = absent
os_state    = present
origin      = residual
```

会被 deterministic classifier 标为 `residual`：逻辑 checkpoint 早于 effect，而 OS resource 在恢复边界后仍存在。这正是 SyncFuzz 希望定位的跨层状态差异模式。

但当前 passive `lstat` 无法证明以下问题：

- listener 是否仍由唯一预期进程持有；
- effect 是否被重复执行并产生第二份等价 resource；
- socket 是否是同名重建但 service identity 已变；
- 是否存在另一个与 effect multiplicity 相关的资源变化。

为避免把一个弱 probe 包装成漏洞证据，LangGraph executor 当前将 `EffectMultiplicity` 固定报告为 `unknown`。classifier 的 fail-closed 规则规定只要 origin 或 multiplicity 是 unknown，就输出 `inconclusive`，不猜测 `residual`、`duplicate` 或 `reconstruction`。

因此这次结果的正确表述是：

> 已在两个独立 fresh-runtime recovery query 中观察到：恢复到 effect 前的 logical state 时，socket metadata 仍存在；恢复到 effect 后的 logical state 时，socket metadata 也存在。当前 probe 无法判断 effect multiplicity，故不对是否存在 contract violation 下结论。

## 9. 分类器的完整语义

分类器不依赖 LLM，而使用以下确定性规则：

| 条件 | point classification |
|---|---|
| 任一字段 unknown，或没有 deterministic evidence | `inconclusive` |
| `effect_multiplicity = duplicate` | `duplicate` |
| OS present 且 origin 为 reconstructed | `reconstruction` |
| Agent absent、OS present、origin 为 residual | `residual` |
| Agent present、OS absent | `missing` |
| Agent 与 OS 均 absent，或二者均 present 且 OS 为 residual / none | `consistent` |

pair 聚合时，after branch（即 effect 后的 checkpoint）优先；若 after 是 clean，而 before 出现非一致结果，则 before 仍作为 boundary-localization evidence 保留。严重度顺序为：

```text
consistent < inconclusive < missing < residual < reconstruction < duplicate
```

这套分类不是最终的自动 contract synthesis。它只提供可审计的 Agent/OS differential evidence；Oracle/Contract 自动化是后续独立工作，不能替代 effect validation 或 frontier selection。

## 10. 关键 artifact 清单与审计路径

`runs/` 是生成物目录，按项目纪律不会提交到 Git。汇报或复查此运行时应保留该目录。审计可按下面的顺序打开：

| 阶段 | artifact | 读者应检查什么 |
|---|---|---|
| objective | `examples/objectives/unix-listener-survival.example.json` | 目标 atom、lifetime、relation、persistence。 |
| candidate | `runs/langgraph-v2.4/manual-baseline/candidate.json` | task、candidate ID、generator provenance。 |
| target execution | `…/1784813806441091527/target-task.json` / `target-result.json` | 目标、镜像、模型、成功状态。 |
| raw evidence | `ebpf-resource-events.jsonl`、`ebpf-process-events.jsonl` | cgroup 范围内原始 event。 |
| observation evidence | `checkpoint-catalog.json`、`checkpoint-state-summaries.json` | controller checkpoint 与资源闭包。 |
| normalized frontier | `checkpoint-effect-map.json` | persistent delta、evidence links、frontier score。 |
| profile identity | `profile-run.json` | candidate/profile/native runtime/plan 的关联。 |
| seed | `state-seed.json` | 已验证 atoms、frontier、资源 IDs。 |
| native binding | `langgraph-native-frontier-binding.json` | effect window 与 exact native checkpoint 的时间关系。 |
| frozen plan | `langgraph-fork-plan.json` | task/model/image/probe/two coordinates，且无 credential；head / retention 尚未显式化。 |
| pair | `recovery-pair.json` | 当前 before / after compatibility subset；尚无 `Q_head`。 |
| before evidence | `recovery-runtimes/...3094571268/langgraph-recovery-observation.json` | 新 runtime、unique coordinate resolution、lstat metadata。 |
| after evidence | `recovery-runtimes/...1228889475/langgraph-recovery-observation.json` | 同上。 |
| final result | `fork-pair-execution.json` | 两个 observation 与 deterministic classification。 |

主目录前缀均为：

```text
runs/langgraph-v2.4/manual-baseline/native-timing/
```

## 11. 实现中强制保持的正确性不变量

这些约束是闭环有意义的原因，而不是实现细节：

1. **真实 effect 优先。**没有 linked persistent evidence 的 LLM 任务、syscall 或文件不会形成 frontier。
2. **controller checkpoint 不冒充 Agent checkpoint。**profiling checkpoint 只用于定位 effect；恢复只使用 native durable checkpoint。
3. **同一 clock domain。**eBPF effect、controller checkpoint 和 LangGraph durable `put` 都以 `CLOCK_MONOTONIC` 连接。
4. **binding 必须严格包围 effect window。**before native save 必须早于第一个 required effect；after native save 必须晚于最后一个 required effect。
5. **旧 native ID 只作 provenance。**fresh runtime 只能通过 structural coordinate 唯一解析自己的新 ID。
6. **historical cut 是唯一 discovery 变量。**同一 seed、head、frontier、task、model、image、recorded plan、retention policy、passive observation；当前 before/after 子集只改变 checkpoint coordinate。
7. **每条 query 都是 fresh runtime。**before/after/head 不能复用 container、workspace 或 runtime ID；跨 query 比较的是可比的 head materialization，不是同一物理 OS instance。
8. **query 内保留 residue。**同一 query 的 initial 与 fresh-resume process 留在同一 container/workspace，以便构造 `<A_C,O_H^(q)>`，检查 logical recovery 没有回滚的 OS state。
9. **被动观察不改状态。**`lstat` 既不建连也不调用 Agent follow-up。
10. **credential 不落盘。**plan / pair / profile artifact 不包含 API key；运行时从环境传递。
11. **不确定就拒绝分类。**coordinate 非唯一、binding 不成立、runtime 未重建、probe 无法解释 multiplicity，均 fail closed。

## 12. 这条闭环已经证明什么

1. **真实 LangGraph target 可被 profile。**不是 toy simulator；Agent 真实调用 shell，实际在隔离容器中创建并使用 Unix socket service。
2. **eBPF 与 probe 可以在同一 run 中闭合证据。**`bind/listen` event 与 path / socket / FD / process closure 精确链接。
3. **frontier 不是按时间猜选。**只选择具有 persistent delta 和 evidence links 的 `before-command..after-command`。
4. **StateSeed 具有可追溯来源。**它能回溯到 objective、candidate、profile run、native runtime、frontier 与 recorded plan。
5. **能把 OS effect frontier 映射到框架原生恢复点。**映射依赖 monotonic durable-save timestamp，不依赖 checkpoint 名字或 list order。
6. **能在新的 runtime 中恢复唯一匹配的 structural checkpoint shape。**旧 native ID 没有被错误复用；fresh runtime 中完成唯一 coordinate resolution。该 shape 不是完整 logical-state equivalence claim。
7. **成对恢复的隔离条件已实际执行。**before / after 为两个独立 container，且每个 query 都有恢复实例证据。
8. **结果解释能保守收敛。**当前 observation 不足时返回 `inconclusive`，不把“看似残留”写成确认漏洞。
9. **当前 before/after 是 historical-cut 核心子集。**它尚未包含 `Q_head`，也未把 materialization head / retention policy 写成一等 artifact 字段。

## 13. 这条闭环没有证明什么

以下表述在汇报中应明确避免：

| 不能宣称 | 原因 |
|---|---|
| “已经发现了 LangGraph 漏洞” | 最终结果是 `inconclusive`，没有满足完整 verdict evidence。 |
| “SyncFuzz 已自动合成自然任务” | 本次 candidate 是 `manual-langgraph-baseline-v1`，尚无 generator success-rate 实验。 |
| “覆盖了 IPC / OS 状态面” | 一个 validated seed 不是 family coverage；还需 objective / effect / frontier / outcome ledger。 |
| “eBPF detector 的全局 precision / recall 已知” | 现有 calibration audit 仅是 fixture-scoped known-answer 检查。 |
| “before/after 的唯一现实差异绝无模型随机性” | plan 字段被固定，但不同 fresh initial runtime 的模型行为仍是实验变量；需在后续实验报告模型/decoding 控制与重复次数。 |
| “fresh runtime 已恢复完整语义等价的 Agent state” | 当前 coordinate 的可执行匹配键是 `message_count + next`，旧 ID / history index 只作 provenance；本 run 中它唯一，但不是完整 state hash。后续应增强 coordinate 或增加 semantic consistency validation。 |
| “socket 元数据存在等于服务可用或可利用” | `lstat` 只证明 endpoint metadata，不证明 listener health、peer behavior 或安全影响。 |
| “controller 观察点可直接恢复 Agent” | 恰恰相反；恢复依赖 LangGraph native durable checkpoints。 |
| “当前 pair 已证明 head fork / no-rollback control” | 当前只有 frontier 前/后两个 cut；`Q_head` 尚未执行。 |
| “before/after 共享同一个物理 OS head” | 两条 query 使用独立 container；当前只固定 materialization plan，尚未显式验证 head-state equivalence contract。 |

## 14. 当前最直接的技术缺口

本闭环的下一道门槛首先是：**把 historical checkpoint recovery 的 `H` 与 OS retention policy 从隐含执行行为变成可审计 contract。**在此基础上，再增强 multiplicity / origin observation，而不是扩大 prompt mutation。

优先工作应是：

1. 将 `MaterializationHead`、目标资源在 `H` 仍存在的 evidence、`retention_policy` 与 adapter `mechanism` 写入 StateSeed / recorded plan；明确 current query 是 `<A_C,O_H^(q)>`，而不是假装复用原 profile container。
2. 扩展当前 before/after `RecoveryPair` 为 before/after/head recovery set，并执行 `Q_head=<A_H,O_H>` no-logical-rollback control。
3. 为 Unix listener 定义一个仍然不改变目标语义、但可区分 single / duplicate / reconstruction 的确定性 probe；例如以 listener holder process、FD/socket ownership、生命周期记录或协议无副作用 health semantics 组合证据。任何主动协议交互都必须作为新的、明确命名的 passive-observation contract，而不能静默替换 `lstat`。
4. 把该 probe 与 head-equivalence check 的 full-vs-pruned fidelity 加入校准，验证优化观察面不会改变 verdict。
5. 在多个 objective 与 state family 上重复这条闭环，并用随机 / 非-frontier historical cut 作对照，测量 frontier guidance 是否真的提高有效 A/O relation / localization 产出。
6. 再引入外部 generator 的多次 synthesis 评估：生成成功率、持久性、head retention、seed retention 和 coverage increment。

## 15. 汇报时推荐的表述

可以将当前进度概括为：

> 我们已经完成了第一条真实 LangGraph vertical slice：从一个状态目标出发，在实际 Agent shell 执行中由 eBPF 和 probe 确认 Unix listener 的持久 effect，自动定位 effect 两侧的 frontier，并用同一 monotonic clock 将该 frontier 映射到 LangGraph 原生 durable checkpoints。当前 adapter 以 fork 实现 historical recovery：每条 query 先 materialize 自己的 head OS state，再按结构坐标恢复一个历史 logical checkpoint，仅做固定的被动 socket 观察。before/after 子集已跑通；head control、显式 retention contract 与 multiplicity evidence 仍待补齐，因此结果被保守分类为 inconclusive，而不是漏洞。 

如果被追问“为什么不直接把 before 的 `agent absent / socket present` 当作漏洞”，回答应是：

> 因为一个可信漏洞结论需要证明它不是同名重建、重复 effect 或 probe 自身带来的变化。我们已经刻意把分类器设计成证据不足时拒绝结论，而不是以一个看起来合理的例子替代可审计的因果证据。

## 16. 代码定位

| 职责 | 主要位置 |
|---|---|
| CLI subcommands | `cmd/syncfuzz/main.go` |
| objective / ProfileRun / StateSeed | `internal/syncfuzz/objective/` |
| eBPF collector、normalizer、frontier linker | `internal/syncfuzz/profiling/` |
| coverage IR | `internal/syncfuzz/coverage/` |
| candidate、LangGraph profile / binding / plan preparation | `internal/syncfuzz/synthesis/` |
| recovery model、classifier、executor registry | `internal/syncfuzz/recovery/execution.go` |
| LangGraph recorded plan / fresh-container executor | `internal/syncfuzz/recovery/langgraph*.go` |
| target profiling integration | `internal/syncfuzz/target/target_*profiling.go` |
| LangGraph wrapper / durable saver / coordinate resolver / passive probe | `targets/langgraph_shell_react/run_target.py` |
| LangGraph isolated image | `targets/langgraph_shell_react/Dockerfile` |
| operational wrappers | `Makefile` 中 `synthesis-langgraph-*` 目标 |

## 17. 可复现实验顺序

完整重跑遵循以下顺序；每一步都消费前一步 artifact，而不是手填 ID：

```text
1. synthesis schedule / generate
2. make langgraph-profile-image
3. make synthesis-langgraph-profile
4. synthesis evaluate + synthesis promote      -> StateSeed
5. make synthesis-langgraph-bind-frontier       -> native binding
6. make synthesis-langgraph-prepare-fork        -> recorded plan + bound profile
7. profile recovery-pair                         -> 当前 before/after compatibility pair
8. syncfuzz recovery execute                     -> current fork-pair execution
9. （待实现）materialization-head + recovery set -> before/after/head controls
```

第 8 步的核心命令形态是：

```bash
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz recovery execute \
  --seed runs/<root>/state-seed.json \
  --pair runs/<root>/recovery-pair.json \
  --out runs/<root>/fork-pair-execution.json \
  --timeout 2m
```

运行前需要：预构建的 `syncfuzz-langgraph:dev` image、可用 Docker、模型 provider 环境变量，以及 profile 阶段所需的 host eBPF 权限。recovery 阶段不写入 credential artifact，但它仍需要通过父进程环境获得模型 provider credential。

## 18. 与五项既定任务的关系

| 既定任务 | 本闭环给出的进展 |
|---|---|
| 根因分析 | 不在本路线扩展；已有 case study / calibration 可复用。 |
| Mutator | 已明确替换为 state-objective-driven task synthesis 与 historical checkpoint cut；本例展示了其当前 before/after 子集。 |
| Oracle / Contract 自动化 | 尚未完成；当前有 deterministic A/O classifier，但不等于自动 contract generation。 |
| Violation / Seed 分类 | `StateObjective`、validated `StateSeed`、frontier 与 coverage IR 已提供新的分类基础。 |
| eBPF 引入 | 已是本闭环的核心发现信号，并与 probe 形成 identity-linked evidence。 |

## 19. 最终定位

这条 LangGraph 闭环应被看作 SyncFuzz v2 的**reference vertical slice**：它覆盖了方法论中最容易被混淆的接口——OS effect、controller evidence、framework-native checkpoint、fresh recovery runtime 和最终 differential classification——并对每个接口设置了拒绝错误捷径的规则。方法的中心不是 fork API，而是对 retained OS head state 的 historical checkpoint cut。

下一阶段不是再堆叠手写 mutation，而是让这条闭环在更强 probe 和更多 objective 上产生可以比较、可以量化、可以写进论文的证据。
