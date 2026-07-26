# 环境结构化的 Historical Recovery-Hazard Fuzzing

> 状态：**已冻结原型，不可作为当前设计规范**（2026-07-26）。本文记录已实现的
> fixture、LangGraph target-side `E` materialization、native-plan lock 和 recovery-use
> collector，但此前的 five-control join 未实现真正 retention ablation，也未证明
> post-effect checkpoint 的 Agent awareness。冻结边界见
> [LEGACY_V3_PROTOTYPE_STATUS.md](LEGACY_V3_PROTOTYPE_STATUS.md)。
>
> 已实现的 V2/V3 LangGraph profile、StateSeed、frontier、native binding、retained
> runtime、before/after/head recovery set 与 continuation evidence 是本设计的基础。现在还
> 有一个严格标为 `fixture-only` 的 Unix-socket calibration：`EnvironmentProgram`、local
> materializer、`RecoveryUsePlan`、run-local/semantic identity、`RecoveryHazardReport` 和
> five-control classifier 均已落地，并以真实 local Unix socket 的 bind/rebind/connect/I/O
> 校准其组合语义。它**不是** LangGraph target integration、eBPF recovery-use trace、
> StateSeed 或 coverage finding；本文不能把现有 `residual` relation 或该 fixture 追写成
> 漏洞或 target `REBOUND` finding。

> **命名边界**：历史 LangGraph artifact、Make target 与代码注释中的 “V3” 常指
> retained-runtime recovery-set contract 的一次实现修订；它不是完整的
> `EnvironmentProgram` Fuzzer。本文中的“当前路线/V3”指本规范定义的
> environment-structured recovery-hazard 方法。讨论既有运行时应优先写明具体 artifact
> 或 recovery-set contract，避免只以版本号断言能力。

## 1. 目标

SyncFuzz 的已实现底座能证明 historical checkpoint recovery 形成的 Agent/OS
mixed state：

```text
initial materialization: <A_H, O_H>
historical recovery:    <A_C, O_H>, where C < H
```

但“资源仍存在”不等于恢复后的 Agent 真正依赖了它。V3 的发现目标是一个可审计的
recovery hazard：

```text
W -> R(C,H) -> U'
```

- `W`：materialization 中形成 capability/liveness state 或改变 name-to-object binding
  的已验证 write/bind effect；
- `R(C,H)`：从历史 logical checkpoint `C` 恢复，同时保留 `H` 时 relevant OS state；
- `U'`：恢复后正常 workload 驱动的 typed resolve/use，例如 `connect`、`open`、
  `exec`、`read`，或 adapter 明确证明的 context load。

`W` 在本文始终指 write/bind effect。旧 recovery artifact 中表示 passive observation
的字段应写为 `Obs`，避免与此混淆。

目标不是穷举任意 OS 状态，也不是把手写后果接到已知 residue 上；目标是探索下列
工程上不可穷尽的结构化空间：

```text
X = <Workload, EnvironmentProgram E, RecoveryPlan Σ>
```

对抗性载荷 `I` 是未来单独声明的扩展：`X_adv=<Workload,E,I,Σ>`。它不进入第一阶段的
可靠性实验，也不改变主 Oracle。

## 2. 三个输入维度

### 2.1 Workload：固定的正常工作流

一个 workload 是：

```text
Workload = <base project, P_init, P_cont, runner/model constraints>
```

- `P_init`：初始正常任务，产生真实执行、checkpoint 与 materialization head；
- `P_cont`：恢复后固定的正常后续请求；所有 controls 的 exact bytes 相同；
- base project：正常项目骨架或经审计的真实项目入口；
- runner/model constraints：镜像、工具、模型配置与超时等。

Workload 说明 Agent 要做什么，不说明 checkpoint、资源路径、变异、预期 witness 或
结论。`P_cont` 需要足以触发正常后续工作，但不是“检查某个资源是否还在”的探针。

Workload 是低频 corpus 对象：可由人工或 LLM 产生，经过 clean baseline/stability gate
后冻结。高频 fuzz 不改写它的任务语义。

### 2.2 EnvironmentProgram E：可变异的资源绑定图

`E` 是计划构造的环境，不等同于 profile/probe 后实际观察到的资源图。其最小模型为：

```text
logical name
  -> config key / environment variable / alias
  -> pathname or endpoint
  -> kernel object
  -> FD / capability
  -> holder process
```

它应包含：

```text
EnvironmentProgram
  - typed nodes and edges
  - deterministic materialization steps
  - allowed mutation trace and parent ID
  - declared semantic roles
  - expected normal resource touches (admission signal, not Oracle)

EnvironmentMaterialization
  - observed device/inode/socket/PID/FD identities
  - creator and holder provenance
  - profile-time evidence that E actually materialized
```

最初只支持 Unix-domain socket family。可变异的结构包括：

| operator | 保持不变 | 改变的绑定结构 | 当前实现边界 |
| --- | --- | --- | --- |
| `increase-indirection` | workload 的正常任务意图 | direct path 变为 config/env/alias chain | Unix-socket IR；local materializer 支持这些 resolution modes |
| `add-alias` | logical service role | absolute/relative/configured alias 的解析关系 | Unix-socket IR；local materializer 仅接受 workspace-relative alias artifact |
| `rebind` | logical name | 同名对象由 role/object A 改为 B | Unix-socket fixture 已真实 unlink/rebind 同一路径 |
| `shift-holder-lifetime` | service role 和 name | foreground、child、detached holder | IR 已保留；local materializer 目前只实现 foreground，其他值 fail closed |
| `preexist-resource` | consumer workload | resource absent/benign-existing/previous-instance | 后置；尚未加入首个 IR/mutator |

任何 mutation 都必须在隔离 workspace/container 中真实 materialize，并在 profile 中满足
effect、frontier 与 head-retention gate。未形成 `W` 的变异是无效输入，不进入 corpus
coverage。

### 2.3 RecoveryPlan Σ：可变异的历史恢复计划

第一阶 recovery step 写作：

```text
Σ1 = <H, C, ρ, K, μ, Obs>
```

- `H`：materialization head；
- `C`：严格早于 `H` 的 native logical checkpoint；
- `ρ`：OS retention policy；
- `K`：冻结的 workload continuation；
- `μ`：adapter mechanism；
- `Obs`：被动/typed observation contract。

当前 V3 LangGraph 实现的是一个固定 `H` 的单次 recovery set，并派生
`before/after/head` controls。未来合法 mutation 为：

| operator | 含义 | 状态 |
| --- | --- | --- |
| `cut-shift` | 在已验证 frontier 前、后及更早历史 checkpoint 间选 `C` | 当前已有 before/after/head 基础 |
| `head-shift` | 在 effect 后的不同、资源仍存活的 `H` 之间选择 | 待实现 |
| `rollback-depth-expand` | 跨越更多已验证 frontier | 待实现 |
| `frontier-select` | 选择不同 `W` 所在 frontier | frontier miner 已有基础 |
| `repeated-recovery` | `[(H1,C1),(H2,C2),...]` | 明确后置，当前未实现 |

`ρ` 与 `μ` 首先是受控实验条件，不能混入随机 mutation。fork、rewind 与 replay
只有证明相同 retention/re-execution semantics 后才能比较。

## 3. 执行与控制组

对一个候选 `<Workload,E,W,H,C>`，Fuzzer 不将 before/after/head 视为三个独立 finding，
而是自动派生一个归因 bundle：

| 条件 | logical state | OS state | 用途 |
| --- | --- | --- | --- |
| treatment | `A_before` | `O_H,tainted` | 历史逻辑与 retained state 的核心测试 |
| frontier-local control | `A_after` | `O_H,tainted` | 说明差异是否跨越 `W` frontier |
| no-rollback control | `A_H` | `O_H,tainted` | 排除不依赖 logical rollback 的现象 |
| retention ablation | `A_before` | `O_clean` | 排除仅由旧逻辑或 prompt 引起的现象 |
| clean baseline | `A_H,clean` | `O_clean` | 建立正常语义基线 |

`after` 或 `head` 不要求 Agent 终止、清理或修复对象。它们只提供归因信息；一个
relation 或 hazard 也不自动等于 framework contract violation。

## 4. 观测与 Oracle

### 4.1 两类身份

不能跨 fresh container 直接比较裸 inode、PID 或 socket ID。每个 resource adapter
必须区分：

```text
RunLocalIdentity
  - file: device + inode (+ generation where available)
  - process: PID + start time
  - socket: kernel socket identity + holder identity

SemanticIdentity
  - EnvironmentProgram node/role
  - creator provenance
  - executable or content hash
  - normalized configuration/resolution chain
```

前者证明某次运行内 `U'` 实际触达哪个对象；后者用于 clean/test 或不同 fresh runtime
之间比较对象的角色与来源。

### 4.2 Typed resolve-dependence，而非通用 taint

第一版不实现通用数据流污点。只接受有限的、resource-family-specific 依赖边：

```text
names -> resolves-to -> object
object -> connects-to / reads-from / executes / loads-into-context
```

Unix socket 的最小证据应包括：

```text
W: bind/rebind/listen has profile-time identity
R: exact historical checkpoint restore is proved
U': resumed tool/process connects through the logical pathname
use: connection reaches the observed listener role and performs real I/O
```

当前 passive socket/file relation 只证明静态 presence/origin/multiplicity；它不是
`U'` observation。首个 local calibration 已用独立的 `fixture-roundtrip` evidence 记录
`resolve -> connect -> listener role -> I/O`，并只允许输出 `realized-calibration`。
LangGraph 的 first target integration 已记录 filesystem AF_UNIX pathname，并在 fresh
recovery cgroup 中将 successful `connect` 与 retained active listener 的 role-tagged completed-
exchange record 交叉验证。该 record 用固定 acknowledgement 证明 client consumed the response，
但不记录 application payload。`hazard langgraph-target-report` 已能把两个独立 fresh profile
的 target materialization、before/after/head recovery execution 和 completed-exchange evidence
严格合并为五控制 report：它比较 structural checkpoint coordinate 与 semantic identity，而不
比较跨 container 的 PID/inode/socket ID；并要求六个已执行 control 的 request digest 完全一致。
这只是 artifact-level contract，尚无真实 target report；在第一次真实五控制运行通过前，不能
把该实现表述为 binding hazard finding。

### 4.3 分类层次

现有 `aligned/residual/missing/reconstruction/duplicate` 保留为静态 A/O relation 层。
其上新增、而不重命名的 hazard layer：

```text
binding hazards:
  rebound / residue / missing / duplicate-binding

capability-liveness hazards:
  orphan / live-residue / untracked-capability / delayed-effect
```

只有在 `U'` 对 divergent object 产生 typed dependence 时，才是 realized hazard。是否
违反某个 framework contract 仍由独立 contract layer 判断。这是目标 taxonomy；当前
Unix-socket fixture classifier 只实现 `rebound`（以及 `none`），不能把其余名称写成已经
支持的 verdict。

## 5. Coverage、corpus 与不可穷尽性

对于小型 calibration fixture，有限 `E × Σ` 可以穷举；那是 detector calibration，
不是 Fuzzer effectiveness claim。真正的搜索空间由 workload corpus、typed binding
graph、动态 profile frontier、合法 head/cut 和真实运行结果共同决定，工程上不可穷尽。

coverage 至少分四层：

```text
binding coverage:
  <namespace, bind operation, resource type, indirection depth, holder lifetime>

recovery-plan coverage:
  <frontier type, rollback depth, recovery count, head position>

realized-hazard coverage:
  <hazard family, W class, U' class, resource family, semantic relation>

dependency coverage:
  <resolved, read, executed, connected, loaded-into-context>
```

Fuzzer 只保留带来新 coverage、且通过 stability/identity evidence gate 的
`<Workload,E,Σ>`。LLM task 生成仅能扩展 workload corpus；它不直接决定 `E` mutation、
`Σ` mutation 或 verdict。

## 6. 实施顺序与明确不做的事

第一阶段只实现 Unix socket pathname family：

1. **已完成（fixture-only）**：`EnvironmentProgram` 与确定性 local materializer；
   direct/config/environment/alias resolution、rebind mutation lineage 与 foreground
   holder；真实 bind/rebind 的 local/semantic identity artifact；
2. **已完成（fixture-only）**：由 frozen `Workload` 与 `E` 派生的
   `RecoveryUsePlan`，以及真实 local `connect -> role-tagged I/O` use evidence；
3. **已完成（fixture-only）**：before/after/head、retention ablation、clean baseline 的
   `RecoveryHazardReport` classifier；它只在 evidence mode 为 fixture 时报告
   `realized-calibration`；
4. **已接通、已锁入 recovery plan（target materialization）**：`synthesis execute-langgraph
   --environment-program <E.json>` 将 immutable `E` 作为 controller-owned workspace
   artifact 传入 LangGraph target。disk checkpointer 的**第一个** native durable checkpoint
   持久化后，target child listener 才 bind/rebind；target 随后写出
   `environment-materialization.json`，其中含 source native checkpoint、同钟域 effect
   window、endpoint、resolution mode、child PID/role/FD/socket ID；controller 同时要求该
   window 内出现 cgroup-scoped active-socket `bind` + `listen`。`synthesis
   prepare-langgraph-fork` 还会拒绝 native before coordinate、effect window、endpoint 或
   retained head socket ID/PID/FD 与这些 artifacts 不一致的 plan。因此 `W` provenance 已被
   锁入该 native recovery plan；它尚未成为扩展后的 `StateSeed` 或 coverage record；

   target adapter 当前只接受 `direct`、`config` 与 `alias` resolution。`environment` mode
   仍可用于 local fixture，但不能安全跨 fresh recovery container 传播，因此 target 路径
   明确拒绝它，避免把 source process 的 environment 误当作 retained OS state；
5. **已接通、artifact-level control bundle 已实现（target recovery/use）**：resource eBPF 现为
   filesystem AF_UNIX `bind`/`connect` 记录 bounded endpoint path；带 `E` 且执行 frozen
   continuation 的 LangGraph recovery container 改为 gated create/start，collector 在 gate
   释放前绑定其独立 cgroup。它写出 `ebpf-recovery-resource-{scope,events}.json*`，要求
   recovery cgroup 对 `E.endpoint_path` 产生成功 `connect`，并由 retained active listener
   的无 payload role-tagged observer log 交叉确认。listener 只记录 request length/digest、
   response-sent 与 fixed acknowledgement，不记录 application payload；因此一次合格记录
   证明客户端发出请求、读取 role-tagged response 并确认完成。`hazard
   langgraph-target-report` 已消费两个独立 profile 的 immutable artifacts，并只在
   treatment/after/head、clean before/head 及其请求 digest、structural coordinate、semantic
   identity 全部可比时写出 `RecoveryHazardReport`。它不自动进入 corpus；尚缺真实 target
   five-control run 与 coverage admission；
6. **待实现（search）**：合法 `Σ` mutation、coverage ledger、corpus scheduling，以及
   graph-authorized LangGraph workspace topology。

### 6.1 当前可运行的 fixture calibration

该 calibration 用于验证首个 resource family 的输入 IR、identity 记录、五个 control 和
classifier 是否能在真实 local Unix socket 上闭合；它不是 LangGraph 实验，也不需要 Docker、
模型凭据或 eBPF 权限：

```bash
make v3-unix-socket-fixture
make hazard-unix-socket-calibration \
  V3_UNIX_SOCKET_CALIBRATION_OUT=runs/v3-unix-socket-calibration.json
```

第二条命令输出一个 JSON artifact，其中 `fixture_profile` 是由 fixture telemetry 生成的
frontier/map 校验记录，`hazard_report` 包含 treatment、frontier-local、head、retention
ablation 和 clean baseline 的精确 use evidence。预期结果是：treatment 的 restored logical
expectation 为 `benign`，但实际连接 `replacement`；after/head 知道 `replacement`；两个 clean
control 都连接 `benign`。因此它只会得到 `hazard_status=realized-calibration`，绝不会写入
corpus 或变成 target finding。若调用者提供的 workspace 使 Unix socket pathname 过长，命令会
只为 socket materialization 选用短的临时根目录，artifact 语义不变。

其中 `R_fixture` 是为 classifier 构造的 known-answer recovery coordinate：它不调用
LangGraph/MAF native checkpoint restore，也不声称 eBPF 在 target cgroup 观察到 `W` 或 `U'`。
该命令真实验证的是 local bind/rebind、logical resolution、connect/I/O、identity 与 five-control
artifact 的组合；native recovery 和 target-side trace 必须由下一阶段单独接入。

### 6.2 Target-side E materialization preflight（不是 hazard experiment）

先用 controller 生成不可伪造 ID 的 child-holder `E`。下面只描述一个本地 Unix socket
binding/rebind，不是攻击载荷，也没有给 Agent 加入资源探测 prompt：

```bash
syncfuzz environment unix-socket-program \
  --out runs/v3-target-e.json \
  --logical-name agent-service \
  --resolution-mode config \
  --resolution-key agent_socket \
  --resolution-artifact-path service.json \
  --endpoint-path agent.sock \
  --initial-role baseline \
  --active-role replacement \
  --holder-lifetime child

make synthesis-langgraph-profile \
  LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> \
  LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> \
  LANGGRAPH_SYNTHESIS_ROOT=runs/v3-target-materialization \
  LANGGRAPH_SYNTHESIS_ENVIRONMENT_PROGRAM=runs/v3-target-e.json
```

成功 profile 的 target run directory 必须同时有 `environment-program.json`、
`environment-materialization.json` 与 `langgraph-native-checkpoints.json`。后者记录
materializer 所在的 source native checkpoint 和 monotonic effect window；controller 在导入
时会验证 program ID、resolution/endpoint、initial/active role、两个 child PID 及时间顺序。
该 profile 验证 **E 进入 target cgroup 的 provenance**、source native checkpoint 和 active
socket 的 cgroup-scoped `bind`/`listen`。随后 `synthesis prepare-langgraph-fork` 才把该 raw
`W` 与 native frontier binding、effect window 和 retained head socket identity 共同锁入 plan。
它不能直接运行 seed promotion 或报告 hazard；但完成该 plan 后可以运行 native recovery。

在后续 `synthesis prepare-langgraph-fork` 中，若 recorded target plan 含 `E`，controller 会
重新读取同目录的 immutable program/materialization artifact，并拒绝不满足以下条件的 plan：
retained socket path 不等于 `E.endpoint_path`、binding 的 before native checkpoint 不等于
materializer source checkpoint、binding effect 时间窗不包含在 materialization window 中，或
head probe 的 socket ID/PID/FD 不等于 `E` 的 active listener。这个检查把 **target-side W
provenance** 锁入 native recovery plan；它仍不是 recovery-time `U'`。

当前受限 `U'` evidence 不是 listener 仍存活，也不是 continuation text 本身，而是同一次
recovery runtime 的下列交集：``ebpf-recovery-resource-events.jsonl`` 中成功的
`connect(/workspace/<endpoint>)`，以及 source retained listener 在相同 trace 时间窗写出的
active-role accept event、`response_sent=true` 和 exact health-protocol acknowledgement。日志
不记录 request payload；它只保留 request length/digest，把已完成的 normal request/response
exchange 绑定到 `E` 的 semantic role。若 Agent 的 continuation 没有真实触发这条 normal use，
recovery executor fail closed，而不是把 static residual 当作 use evidence。

listener accept log 是 controller-authorized 的 append-only observer channel，不是 Agent
state 或 continuation input：source snapshot 和 recovery clone 显式排除它，避免某个 control
的观察结果改变下一 control 的 checkpoint workspace digest。它保留在 retained source runtime，
只按 recovery trace 的 monotonic window 读取。

之后才考虑 executable resolution，再考虑 adapter-specific agent memory/context files。
下列内容不进入第一阶段：

- 任意 OS object 的统一 identity 或通用 taint；
- nested/repeated recovery；
- 将 LLM response cache 当作 side-effect replay；
- 将 calibration fixture 计作 discovery coverage；
- 将 static `residual` 自动升级为漏洞或安全影响。

### 6.3 当前 target completed-exchange preflight

下列命令是下一次真实 LangGraph run 的**单环境** preflight：它验证 tainted `E` 的
profile-time `W`、native plan lock，以及 before/after/head 三个 recovery runtime 都能完成
normal health-client exchange。它不会生成五控制 hazard verdict；该 verdict 还需要一个独立
baseline `E` profile 作为 clean/ablation control。

```bash
mkdir -p runs/langgraph-v3-health-preflight

set -a; test ! -f ./.env || . ./.env; set +a
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz synthesis generate \
  --objective examples/objectives/unix-listener-survival.example.json \
  --target langgraph-shell-react --adapter langgraph \
  --scaffold examples/synthesis/langgraph-unix-socket-health-client-scaffold.example.json \
  --generator-id openai-compatible-health-client-v1 \
  --generator-command 'python3 examples/synthesis/openai_compatible_generator.py' \
  --attempt 0 \
  --out runs/langgraph-v3-health-preflight/candidate.json

GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz environment unix-socket-program \
  --out runs/langgraph-v3-health-preflight/tainted-environment.json \
  --logical-name agent-service \
  --resolution-mode config --resolution-key agent_socket \
  --resolution-artifact-path service.json --endpoint-path agent.sock \
  --initial-role baseline --active-role replacement --holder-lifetime child

make synthesis-langgraph-v3-calibration \
  LANGGRAPH_SYNTHESIS_OBJECTIVE=examples/objectives/unix-listener-survival.example.json \
  LANGGRAPH_SYNTHESIS_CANDIDATE=runs/langgraph-v3-health-preflight/candidate.json \
  LANGGRAPH_SYNTHESIS_ROOT=runs/langgraph-v3-health-preflight/run \
  LANGGRAPH_SYNTHESIS_ENVIRONMENT_PROGRAM=runs/langgraph-v3-health-preflight/tainted-environment.json \
  LANGGRAPH_V3_CONTINUATION_QUERY='Run the standard local health-check command for this project and provide a concise status update.' \
  LANGGRAPH_V3_PROFILE_TIMEOUT=5m
```

生成的 initial task 应只实现 project-local health client，不得创建或覆盖 `service.json`、
绑定 endpoint，或把 socket path 写死。若 client 未按 scaffold 完成 bounded request/read/ack，
recovery executor 将在对应 control fail closed；不要用 endpoint-specific continuation 或
手工补写 observer log 来绕过该 gate。

### 6.4 Target five-control report（artifact join；待真实运行验证）

完整 bundle 需要用**同一个 candidate、相同 initial task、相同 continuation、相同
runner/model constraints**执行两次独立 profile/recovery：一次使用 `baseline -> replacement`
的 rebind `E_tainted`，另一次使用 `baseline -> baseline` 的 clean `E_clean`。二者不能复用
container、source workspace、native checkpoint ID、PID、inode 或 materialization artifact。
它们仅通过同一个 `Workload`、相同 candidate 和相同 structural recovery coordinate 可比。

推荐使用下列单一 Make target 执行两次 profile/recovery 并生成报告。它要求两个尚不存在的
output root，拒绝复用旧 target artifact，并自动从每个 StateSeed 的
`recorded_plan_artifact` 旁读取该 source run 的 materialization：

```bash
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz environment unix-socket-program \
  --out runs/langgraph-v3-health-clean/clean-environment.json \
  --logical-name agent-service \
  --resolution-mode config --resolution-key agent_socket \
  --resolution-artifact-path service.json --endpoint-path agent.sock \
  --initial-role baseline --active-role baseline --holder-lifetime child

make synthesis-langgraph-v3-five-control \
  LANGGRAPH_SYNTHESIS_OBJECTIVE=examples/objectives/unix-listener-survival.example.json \
  LANGGRAPH_SYNTHESIS_CANDIDATE=runs/langgraph-v3-health-preflight/candidate.json \
  LANGGRAPH_V3_TAINTED_ROOT=runs/langgraph-v3-five-control/tainted \
  LANGGRAPH_V3_CLEAN_ROOT=runs/langgraph-v3-five-control/clean \
  LANGGRAPH_V3_TAINTED_ENVIRONMENT_PROGRAM=runs/langgraph-v3-health-preflight/tainted-environment.json \
  LANGGRAPH_V3_CLEAN_ENVIRONMENT_PROGRAM=runs/langgraph-v3-health-clean/clean-environment.json \
  LANGGRAPH_V3_BASE_PROJECT_ID=langgraph-unix-socket-health-client-v1 \
  LANGGRAPH_V3_RUNNER_CONSTRAINTS='image=syncfuzz-langgraph:dev; model=<provider:model>; protocol=health-client-v1; profile-timeout=5m' \
  LANGGRAPH_V3_CONTINUATION_QUERY='Run the standard local health-check command for this project and provide a concise status update.' \
  LANGGRAPH_V3_PROFILE_TIMEOUT=5m
```

当提供 `EnvironmentProgram` 时，Make 默认启用
`LANGGRAPH_V3_AUTO_ENVIRONMENT_FRONTIER=true`：它只接受同时包含 active listener 的 exact-
socket-ID `bind` 与 `listen` evidence、且覆盖 materialization window 的唯一 frontier。这样
tainted 与 clean 各自从自己的 profile 选择对应 frontier，随后由 report 再比较 structural
native coordinate；不会因为同名 controller interval 就假定 raw native checkpoint ID 相同。
若要审计或调试手动选择，可设 `LANGGRAPH_V3_AUTO_ENVIRONMENT_FRONTIER=false` 并显式传入
`LANGGRAPH_V3_FRONTIER=<frontier>`；手动路径同样会被 native-plan/materialization gate 验证。

该 Make target 最后调用 `hazard langgraph-target-report` 并写出
`runs/langgraph-v3-five-control/tainted/recovery-hazard-report.json`。若需要只重新 join
已完成且可信的两组 artifacts，直接调用 CLI；materialization 参数通常不需要提供：

```bash
GOCACHE=/tmp/syncfuzz-go-cache go run ./cmd/syncfuzz hazard langgraph-target-report \
  --candidate runs/langgraph-v3-health-preflight/candidate.json \
  --base-project-id langgraph-unix-socket-health-client-v1 \
  --runner-constraints 'image=syncfuzz-langgraph:dev; model=<provider:model>; protocol=health-client-v1; profile-timeout=5m' \
  --tainted-seed runs/<tainted-root>/state-seed.json \
  --tainted-set runs/<tainted-root>/historical-recovery-set.json \
  --tainted-execution runs/<tainted-root>/recovery-set-execution.json \
  --tainted-program runs/langgraph-v3-health-preflight/tainted-environment.json \
  --clean-seed runs/<clean-root>/state-seed.json \
  --clean-set runs/<clean-root>/historical-recovery-set.json \
  --clean-execution runs/<clean-root>/recovery-set-execution.json \
  --clean-program runs/<clean-root>/clean-environment.json \
  --out runs/langgraph-v3-five-control/recovery-hazard-report.json
```

该命令不接受 request payload。它从 listener observer 的 payload-free SHA-256 record 派生
digest-only `RecoveryUsePlan`，并拒绝任一 control 缺少 completed exchange 或六个 control
的 digest 不相同。输出 `realized` 仅表示五控制 evidence 支持一个 `rebound` classification；
它仍不是 framework contract violation 或安全漏洞结论。任何 input 不可比、normal health
client 未执行、coordinate 不等价或静态 relation 不满足预期时，命令 fail closed 或写出
`inconclusive`，不能人为补齐 artifact。

## 7. 现有实现的映射

当前代码可直接复用：

```text
objective/StateSeed             -> effect/persistence admission gate
profiling ResourceRef/dependency -> ObservedResourceGraph substrate
frontier miner                  -> W candidate localization
LangGraph native binding        -> legal C/H coordinates
HistoricalRecoverySet           -> Σ1 before/after/head skeleton
continuation protocol           -> frozen K and pre/post evidence contract
relation/causal artifacts       -> static evidence and provenance layer
environment/                    -> Unix-socket E IR/materializer/mutation lineage；local fixture materializer
hazard/                         -> fixture classifier；LangGraph target five-control artifact join
```

尚待实现（target/search 层）：

```text
graph-authorized workspace topology
extended `StateSeed` / coverage admission for E materialization
one real target run validating the completed-exchange collector and five-control artifact join
hazard coverage ledger and corpus scheduling
```

因此现有 LangGraph vertical slice 的正确定位是：它已验证 `W` 的 profile evidence、
`R(C,H)` 的 native recovery 和静态 mixed-state relation。新的 `E` path 已把 profile-time
provenance 锁入该 recovery plan，并实现受限 target-side `connect + completed-exchange` collector；但尚未
完成 live five-control execution、cross-runtime semantic identity
或 target hazard classifier。local Unix-socket calibration 验证了
`E -> W_fixture -> R_fixture -> U'_fixture` 的 IR、control 与 classifier 组合。两者仍需由
完整 target control bundle 接合，才是 environment-structured recovery-hazard fuzzer。
