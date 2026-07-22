Round 1
User: 昨天和博士交流之后，我们对工作做出如下决定。
1. 会议意向：由 NDSS 转向 FSE；
2. 论文主题：之前规定了我们的状态是 S=<A,O,X,C>，而在目前实际的实验中，我们做的失同步实验基本都聚焦在 <A,O> 两层，X、C 尚未起步，且我们深入研究 <A,O> 两层之间的关系已经内容足够。因此，本论文只会聚焦在 <A,O> 方面，且论文属性从 Agent 向系统倾斜。
3. 问题1：Motivation Example 例子不错，故事讲的可以。但是我们需要从 OS 层面出发去探索 Motivation 根因。
4. 问题2：当前 Mutator 是如何执行变异的？变异的空间有哪些？我们的初始种子有哪些？我们把一个种子叫做一个 Query，对于一次变异 $q_1$ 是如何变成 $q_2$ 的？
5. 问题3：Oracle/Contract 目前主要靠手写，应该引入 LLM 来促进自动化生成。
6. 问题4：对种子（Violation）分类。

ChatGPT:
这次调整总体上是正确的，而且会让论文主线明显收敛。

FSE 比 NDSS 更要求你们回答“这是不是一种可复用的软件测试方法”，而不是主要依赖漏洞影响和攻击故事。FSE 2027 Research Track 接受实验性、经验性和工具型软件工程研究，强调方法的原创性、soundness、evaluation 和可复现性；当前投稿截止是 **2026 年 10 月 2 日 AoE**，并设有 Major Revision 路径。([Researchr][1])

因此，论文主线最好从：

> Discovering security vulnerabilities caused by (S=\langle A,O,X,C\rangle) desynchronization

调整为：

> **Systematically testing recovery consistency between agent-runtime state and operating-system state.**

安全后果仍然重要，但应作为 violation 的影响展示，而不是论文唯一评价标准。

---

# 一、建议重新定义论文问题

将系统状态缩减为：

[
S=\langle A,O\rangle
]

其中：

* (A)：Agent runtime 管理的逻辑状态，例如 conversation、graph state、checkpoint、tool result 和 execution progress；
* (O)：操作系统执行状态，包括 namespace、process、execution context、open handles 和 IPC endpoints。

为了支持 OS 层面的根因分析，可以进一步将 (O) 分解为：

[
O=\langle N,\Pi,H,E\rangle
]

| 分量    | 含义                                                           |
| ----- | ------------------------------------------------------------ |
| (N)   | OS namespace：文件路径、socket pathname、symlink、inode binding      |
| (\Pi) | Process state：进程、父子关系、process group、生命周期                     |
| (H)   | Handles/capabilities：FD、socket、pipe、deleted-but-open file    |
| (E)   | Execution context：cwd、environment、PATH、umask、shell functions |

恢复操作的正确性不再表述为“所有状态必须回滚”，而是：

[
\operatorname{Compatible}(A',O')
]

也就是恢复后的逻辑状态和 OS 状态必须满足目标 runtime 的恢复契约或安全不变量。

最典型的 violation 是：

[
A'=A_{C_0},\qquad O'=O_A
]

恢复结果将两个不同时间点的状态拼接在一起，形成一个原始执行中从未出现过的 **hybrid state**：

[
S'=\langle A_{C_0},O_A\rangle
]

这应该成为论文的核心理论表达。

---

# 二、问题 1：从 OS 层面解释 Motivation Example 的根因

Unix socket 案例的根因不应该停留在：

> The listener was not cleaned up.

需要分为三个层次。

## 1. Immediate cause：socket namespace binding 没有恢复

在 checkpoint (C_0)：

[
N_0[p]=s_b
]

其中：

* (p) 是固定 Unix socket pathname；
* (s_b) 是 benign broker 创建的 socket；
* (\operatorname{owner}(s_b)=P_b)。

Branch A 执行：

```text
unlink(p)
bind(s_m, p)
listen(s_m)
```

因此：

[
N_A[p]=s_m,\qquad \operatorname{owner}(s_m)=P_m
]

逻辑 fork 回到 (C_0) 后，Agent state 被恢复，但 Unix socket namespace 没有恢复：

[
A_B=A_0,\qquad N_B[p]=s_m
]

Branch B 认为 (p) 仍表示可信服务，但实际 endpoint owner 已经改变。

这不是普通 socket file residue，而是：

> **stale service-identity binding in the Unix namespace**

---

## 2. Enabling cause：恶意进程生命周期超过 Branch 生命周期

Branch A 被丢弃后：

[
L(P_m)>L(\text{Branch A})
]

恶意 daemon 继续持有监听 socket FD，因此它仍然可以：

* 接收新的连接；
  -读取后续 Branch 产生的数据；
  -返回伪造响应；
  -持续产生新的 OS effect。

这是 **active residual state**，而不是静态 residue。

---

## 3. Systemic cause：逻辑分支与 OS resource ownership 解耦

框架没有保存：

[
\operatorname{owner}(P_m)=\text{Branch A}
]

也没有维护：

[
\operatorname{owner}(s_m)=\text{Branch A}
]

因此，在 discard Branch A 时，runtime 无法执行：

```text
terminate all branch-owned descendants
invalidate branch-owned endpoints
restore namespace bindings
```

系统性根因可以概括为：

[
\boxed{
\text{Partial Recovery}
+
\text{Shared Runtime}
+
\text{Missing Resource Ownership}
}
]

其中：

* **Partial Recovery**：只恢复 (A)，不恢复 (O)；
* **Shared Runtime**：Branch A 和 B 使用相同 OS environment；
* **Missing Resource Ownership**：无法把进程、FD 和 socket 映射回逻辑 Branch。

## 根因实验需要增加哪些证据

为了证明因果关系，应记录：

* `unlinkat()` 和 `bind()` 的先后顺序；
* socket pathname 对应的 inode 变化；
* benign PID 和 malicious PID；
* malicious daemon 的 process lineage；
* fork 后 socket 的 peer PID，可通过 `SO_PEERCRED` 验证；
* secret issuance time；
* secret capture time。

关键时间关系：

[
t_{\text{capture}}

>

t_{\text{secret-issued}}

>

t_{\text{discard-A}}
]

还应做三个 counterfactual controls：

| 配置                          | 预期结果        |
| --------------------------- | ----------- |
| 原始 shared-runtime fork      | Secret 被捕获  |
| Fork 时创建 fresh runtime      | Secret 不被捕获 |
| Discard 时清理 Branch A cgroup | Secret 不被捕获 |
| Fork 时恢复 socket namespace   | Secret 不被捕获 |

这些实验能够说明具体是哪一个 OS 隔离机制阻断了 violation。

---

# 三、问题 2：形式化 Query 和 Mutator

现在必须明确：

> 一个 Query 不是一段 Prompt，而是一个可执行的生命周期测试程序。

建议将 Query 定义为：

[
q=
\langle
I,P,B,R,A,W
\rangle
]

| 字段  | 含义                                                           |
| --- | ------------------------------------------------------------ |
| (I) | Initial setup：初始 workspace、进程和 runtime 状态                    |
| (P) | Plant：在当前分支中植入 OS state                                      |
| (B) | Boundary：checkpoint、fork、replay、cancel、restart               |
| (R) | Recovery configuration：checkpoint、runtime 和 process topology |
| (A) | Activation：后续可信执行如何触达残留状态                                    |
| (W) | Witness：观测哪些状态关系以判定 violation                                |

它可以写成一个顺序程序：

```text
Init
→ Plant
→ Lifecycle Boundary
→ Recover
→ Activate
→ Witness
```

Prompt 只是 Query 在某个 Target 上的 lowering 结果之一。

## Query mutation 的形式

一次原子变异定义为：

[
q_2=\operatorname{Repair}\bigl(\mu_i(q_1;\theta)\bigr)
]

其中：

* (\mu_i)：某种 mutation operator；
* (\theta)：具体参数；
* `Repair`：修复变异后不兼容的 activation、witness 和 target configuration。

之后执行：

[
q_2
\xrightarrow{\text{lower}}
\text{ExecutionPlan}
\xrightarrow{\text{target adapter}}
\text{runtime execution}
]

每个派生 Query 必须记录：

```json
{
  "query_id": "q2",
  "parent_id": "q1",
  "operator": "endpoint-substitution",
  "parameters": {
    "from": "create-new-endpoint",
    "to": "replace-existing-endpoint"
  },
  "semantic_diff": [
    "Plant.socket.operation"
  ]
}
```

这让你们能够回答：

> 发现是由哪一次变异产生的？

---

## Mutation space

建议划分为六个正交维度。

### 1. OS resource mutation

```text
filesystem namespace
process
execution context
file descriptor
Unix socket
pipe/FIFO
permission/metadata
```

例如：

```text
regular file → symlink
file path → Unix socket path
socket residue → endpoint replacement
child process → double-fork daemon
```

### 2. OS operation mutation

```text
create
delete
rename
replace
rebind
inherit
detach
duplicate
delay
```

这里比单纯替换“文件类型”更重要。

### 3. Lifecycle mutation

```text
fork
replay
resume
cancel
crash/restart
same-instance recovery
new-instance rehydration
```

### 4. Timing mutation

```text
before spawn
after spawn
after bind
after first write
before tool result
before checkpoint
after checkpoint before acknowledgment
```

### 5. Runtime-topology mutation

```text
same process ↔ split process
same shell ↔ fresh shell
shared workspace ↔ copied workspace
shared container ↔ fresh container
memory checkpoint ↔ disk checkpoint
```

### 6. Activation mutation

```text
existence probe
read residual content
connect residual endpoint
invoke trusted client
introduce future secret
consume residual output
```

---

## 你们当前最应该实现的原子 Mutator

建议至少包括：

[
\mu_{\mathrm{primitive}}
]

替换 OS resource primitive。

[
\mu_{\mathrm{operation}}
]

例如 `create socket` 变成 `replace existing socket endpoint`。

[
\mu_{\mathrm{boundary}}
]

替换 fork/replay/resume。

[
\mu_{\mathrm{phase}}
]

移动 checkpoint 或 fault 时机。

[
\mu_{\mathrm{topology}}
]

切换 same/split-process 或 shared/fresh runtime。

[
\mu_{\mathrm{activation}}
]

从简单 witness 变成 trusted-client activation。

[
\mu_{\mathrm{splice}}
]

将不同种子的 Plant、Boundary 和 Activation 进行组合。

---

## Unix Listener 案例中的 (q_1\rightarrow q_2)

不要声称这是一次单 mutation，因为你们实际上改变了两个维度。应保留两级 genealogy。

### 初始 Query (q_1)

```text
Init:
    no listener at branch-listener.sock

Plant:
    create a new Unix listener

Boundary:
    fork from before listener launch

Recovery:
    shared OS runtime

Activation:
    send PING with a simple client

Witness:
    receive fixed response
```

### 第一次变异

[
q_1'
====

\mu_{\mathrm{operation}}(q_1)
]

将：

```text
create new endpoint
```

变成：

```text
replace existing trusted endpoint
```

### 第二次变异

[
q_2
===

\mu_{\mathrm{activation}}(q_1')
]

将：

```text
PING witness
```

变成：

```text
mint post-fork token
→ invoke unmodified trusted client
→ capture token at residual listener
```

最终 (q_2) 就是 Motivation Example。

这个 genealogy 很有论文价值，因为它证明安全案例不是研究者一次性完整手写出来的，而是从简单 residue Query 经结构化 mutation 演化而来。

---

# 四、问题 3：引入 LLM 生成 Contract 和 Oracle

这里必须限制 LLM 的角色：

> **LLM 应生成 specification 和 probe，但不应直接成为最终判定器。**

否则论文会从 deterministic systems testing 退化成 LLM-as-a-judge。

建议的流程是：

```text
Target documentation / source comments / examples
                    ↓
          LLM contract extraction
                    ↓
        Structured Recovery Contract
                    ↓
          LLM probe generation
                    ↓
      Deterministic execution and tracing
                    ↓
        Rule-based oracle evaluation
```

## Contract schema

LLM 输出不能是自然语言段落，而应是结构化 DSL：

```yaml
operation: checkpoint_fork
preconditions:
  - checkpoint_exists
  - successor_uses_shared_runtime

resource:
  class: unix_socket_endpoint
  scope: discarded_branch

expected_relation:
  logical_state: restore_to_checkpoint
  os_state: unspecified
  branch_noninterference: required

source:
  type: documentation
  location: "..."
  quote_span: "..."

confidence: medium
```

Contract 至少分成三种来源：

| 类型                         | 含义                       |
| -------------------------- | ------------------------ |
| `documented-contract`      | 官方文档明确承诺                 |
| `derived-safety-invariant` | 从 branch/fork 语义推导的安全不变量 |
| `scenario-assumption`      | 测试作者设置的局部预期              |

如果文档没有说明 OS rollback，LLM 必须输出：

```text
os_state: unspecified
```

而不是自动推断成 `must_restore`。

---

## LLM 可以自动生成什么

### 1. Contract candidate

从官方文档中提取：

* 哪些状态被保存；
  -哪些状态被恢复；
  -同实例和新实例恢复的区别；
  -fork 是否共享 workspace/runtime；
  -哪些行为明确 unspecified。

### 2. Observation plan

例如针对 Unix socket：

```text
record socket inode before and after fork
record listener PID
record peer credentials
record process lineage
record secret issuance and capture timestamps
```

### 3. Executable witness

生成：

* existence check；
  -connectivity probe；
  -peer identity probe；
  -trusted client invocation；
  -control-run expectations。

### 4. Oracle rule template

例如：

```yaml
violation:
  all:
    - branch_a_discarded == true
    - listener_owner == branch_a_pid
    - secret_issued_after_discard == true
    - successor_connected_to_branch_a_pid == true
```

最终判定由程序执行，不由 LLM 阅读日志后主观决定。

---

## LLM 模块需要单独评价

FSE 论文中应评价：

1. **Contract extraction accuracy**
   与人工标注 contract 对比。

2. **Executable oracle generation rate**
   生成的 probe 有多少能直接执行。

3. **Oracle precision/recall**
   对已标注 control/violation runs 的判定准确率。

4. **Human effort reduction**
   从手写一个 Oracle 所需时间，降低到审核 LLM 结果所需时间。

5. **Hallucination handling**
   没有 source span 的 contract clause 必须被拒绝或标为 unsupported。

这个模块最好的定位是：

> **LLM-assisted specification mining and oracle synthesis**

而不是：

> LLM determines whether the framework is vulnerable.

---

# 五、问题 4：对 Seed / Violation 分类

首先要区分两个概念：

* **Seed**：一个具体的初始 Query；
* **Violation class**：该 Query 触发的抽象错误关系。

多个 Seed 可以属于同一个 Violation class。

不建议再按照 `file / directory / symlink / FIFO` 做主要分类，因为这只是 resource type。应采用多维 violation signature：

[
v=
\langle
\mathcal{R},
\mathcal{O},
\mathcal{B},
\mathcal{M},
\mathcal{C}
\rangle
]

其中：

| 维度            | 含义                     |
| ------------- | ---------------------- |
| (\mathcal{R}) | mismatch relation      |
| (\mathcal{O}) | OS resource class      |
| (\mathcal{B}) | lifecycle boundary     |
| (\mathcal{M}) | persistence mechanism  |
| (\mathcal{C}) | consequence/activation |

## 1. Mismatch relation

建议定义五类：

### Residual

Agent state 回退，但 OS mutation 保留。

```text
A old, O new
```

### Missing

Agent 认为 OS object 存在，但恢复后 object 消失。

```text
A new, O old/clean
```

### Duplicated

Agent 忘记 effect 已完成，恢复后重复执行。

### Reordered or delayed

逻辑 operation 已 cancel/discard，但 OS effect 随后发生。

### Rebound

同一个 logical name 指向不同 OS entity。

Unix socket hijacking 属于这一类：

```text
same pathname, different endpoint owner
```

---

## 2. OS resource class

建议采用：

```text
Namespace object
Execution context
Process
Handle/capability
IPC endpoint
Permission/metadata
```

## 3. Lifecycle boundary

```text
fork
replay
resume
cancel
crash/restart
rehydrate
```

## 4. Persistence mechanism

```text
shared workspace
persistent shell
descendant survival
FD inheritance
namespace rebinding
delayed execution
missing cleanup
```

## 5. Consequence

```text
observation
cross-branch interference
impersonation
secret capture
state corruption
duplicate operation
```

---

# 六、当前 Seed 可以整理为六个基础族

| Seed family                   | 当前实例                                     |
| ----------------------------- | ---------------------------------------- |
| Namespace residue             | file、delete、rename、symlink、hardlink、FIFO |
| Execution-context residue     | PATH、alias、cwd、umask                     |
| Process-lifetime residue      | orphan process、delayed process           |
| Handle/capability residue     | open FD、deleted-open FD、inherited FD     |
| IPC endpoint residue          | Unix listener、Unix socket                |
| Replay/multiplicity violation | checkpoint replay、partial local effect   |

其中 Motivation Example 的 signature 可以写为：

```text
relation:
  rebound + residual

resource:
  Unix socket endpoint

boundary:
  checkpoint fork

mechanism:
  shared runtime
  descendant survival
  pathname rebinding

consequence:
  service impersonation
  future-secret capture
```

这种分类既能支持 corpus 去重，也能作为 feedback novelty：

[
\operatorname{Novel}(q)
=======================

\text{new combination of }
\langle
\mathcal{R},\mathcal{O},\mathcal{B},\mathcal{M},\mathcal{C}
\rangle
]

---

# 七、建议重新定义论文的 RQ

## RQ1：现象与根因

> What OS-level consistency violations arise when agent runtimes recover, replay, or fork logical state over persistent execution environments?

重点输出 violation taxonomy 和 OS root causes。

## RQ2：Mutator

> Can structured mutation of OS resources, lifecycle boundaries, and runtime topologies efficiently synthesize new recovery-consistency queries?

重点比较 manual、random、enumeration 和 SyncFuzz。

## RQ3：Contract/Oracle 自动生成

> How accurately can LLM-assisted specification mining generate executable recovery contracts and deterministic oracles?

重点评价生成准确性和人工工作量。

## RQ4：跨 Target 有效性

> How do the discovered violations and their causes vary across agent runtimes with different checkpoint and execution-environment designs?

LangGraph 和 MAF 至少需要成熟；第三个 Target 视时间决定。

---

# 八、建议的论文贡献

最终贡献可以收束成四项：

1. **Problem formulation**
   定义 Agent runtime state 与 OS execution state 之间的 recovery-consistency violation。

2. **Query language and structured mutator**
   用可执行 Scenario IR 表达和变异 OS/lifecycle Query。

3. **LLM-assisted contract and oracle synthesis**
   从文档与 Query 中生成有来源、可执行、可验证的恢复规范。

4. **Empirical study**
   在多个真实 Agent runtime 上研究 violation 类型、OS 根因和架构因素。

---

# 九、当前项目中应当立即停掉或降级的内容

为了避免范围再次膨胀，建议：

* X、C 的实验从主论文移除；
* external HTTP effect、authority token replay 不进入主实验；
  -相关实现保留，但放入 future work 或后续论文；
  -不要继续添加更多文件类型；
  -不要让 LLM 直接做最终 judge；
  -不要继续强化攻击 Prompt 作为主要贡献；
  -MAF 应转向 local OS state、process、FD、IPC 和 checkpoint interaction。

新的主线应该是：

```text
OS-level root cause
        ↓
Typed Query IR
        ↓
Structured mutation
        ↓
LLM-generated contract/probe
        ↓
Deterministic oracle
        ↓
Cross-runtime empirical study
```

---

# 十、到 FSE 截止前的优先级

FSE 2027 研究论文要求在提交时提供或解释 replication package，并鼓励开放工具和数据，因此你们现有的 corpus、replay、verify 和 minimization 基础设施会非常有优势。([Researchr][1])

接下来约十周建议这样安排：

### 第一阶段：问题和 IR 冻结

* 冻结 (S=\langle A,O\rangle)；
  -定义 (O=\langle N,\Pi,H,E\rangle)；
  -冻结 Query grammar；
  -冻结 violation signature；
  -把现有 seeds 映射到 taxonomy。

### 第二阶段：Mutator

* 实现 operation substitution；
  -实现 lifecycle/phase mutation；
  -实现 activation mutation；
  -实现至少一种 cross-seed splice；
  -完整记录 (q_1\rightarrow q_2) genealogy。

### 第三阶段：LLM Oracle

* contract extraction；
  -结构化 DSL；
  -source-grounded verification；
  -probe generation；
  -小规模人工标注数据集。

### 第四阶段：实验

* LangGraph campaign；
  -MAF campaign；
  -random/manual/enumerative baselines；
  -mutator ablation；
  -contract generation accuracy；
  -OS root-cause counterfactual experiments。

---

这次方向调整最关键的收益是：论文不再需要证明“我们覆盖了所有 Agent 安全状态”，而只需要把一个更窄的问题研究透：

> **当 Agent runtime 改写逻辑时间，而 OS 继续沿现实时间运行时，哪些不可达的混合状态会产生、它们为何产生，以及如何系统地生成和检测这些状态。**

这个问题对 FSE 的定位比原来的四层安全框架更集中，也更容易形成方法、实现和实证三方面完整的贡献。

[1]: https://conf.researchr.org/track/fse-2027/fse-2027-papers?utm_source=chatgpt.com "FSE 2027 - Research Papers - FSE 2027"

---

Round 2
User: 关于根因分析那部分，还有一个探索性的提议：既然我们要深入探索 Agent 之于 OS 的安全问题，那我们在跑的时候，用 eBPF 去观测 Agent 到底动了哪些东西，然后 prune 掉不用的 hook，然后最后再测试跑 differential。这个提议听上去不错，但是我不是很理解具体是什么意思。你怎么看？

ChatGPT:
这个提议是有价值的，但“先用 eBPF 看 Agent 动了什么，再 prune 掉没用的 hook”这句话容易产生误解。更准确的含义应该是：

> **先用宽覆盖的 OS tracing 为每个 Query 自动识别其实际触达的内核资源和因果路径，再据此生成一个更小、更有针对性的 observation plan，最后使用该 observation plan 执行 control/fault differential testing。**

它本质上是一种：

> **Observation-first, trace-guided differential testing**

而不是简单为了性能，把“第一次没触发的 eBPF hook 全删掉”。

---

# 一、这个提议具体在做什么

以一个 Query (q) 为例，例如 Unix socket endpoint hijacking。可以分成两个阶段。

## 阶段一：Broad Profiling

先挂载一套比较宽的 eBPF 观测点，运行一次或几次 pilot execution：

```text
Query q
  ↓
Broad eBPF tracing
  ↓
Agent/descendant OS-effect trace
  ↓
Resource footprint + causal graph
```

Broad tracing 可能覆盖：

* 进程：fork、clone、exec、exit；
* 文件命名空间：open、create、unlink、rename、symlink；
* FD：dup、close、inherit；
* IPC：socket、bind、listen、connect、accept；
* 权限：chmod、chown、xattr；
* 网络：connect、send、receive；
* 进程控制：kill、setsid、setpgid。

然后根据实际 trace，系统发现该 Query 主要触达：

```text
process lineage
Unix socket pathname
unlink/rebind
listener lifetime
connect/accept
payload transfer
```

而没有触达：

```text
xattr
mount
TCP
chmod
shared memory
```

---

## 阶段二：Targeted Differential Run

根据 profiling 结果生成 Query-specific observation plan：

```text
Selected hooks:
  process spawn/exit
  unlinkat
  socket/bind/listen
  connect/accept
  send/recv
  close

Selected state probes:
  socket pathname
  socket inode
  listener PID
  peer credentials
  process lineage
```

之后正式运行：

```text
Control run：无 fork/fault
Fault run：执行 fork/recovery
```

最后比较：

[
\Delta_O(q)
===========

O_{\mathrm{fault}}(q)-O_{\mathrm{control}}(q)
]

并结合 Agent logical trace 判断：

[
\operatorname{Violation}(q)
===========================

\neg\operatorname{Compatible}
\left(
A_{\mathrm{recovered}},
O_{\mathrm{fault}}
\right)
]

所以，“prune”实际是在自动生成一个更精确的 OS observation plan。

---

# 二、为什么先做 observation-first

你们当前不少 Query 是研究者预先知道要观察什么：

* 测文件残留，就做 filesystem diff；
* 测 Unix listener，就检查 socket 和 PID；
* 测 open FD，就检查 `/proc/<pid>/fd`；
* 测 PATH，就 dump shell environment。

这存在一个问题：

> Oracle 和 observation plan 已经编码了研究者预期的根因。

如果以后 Mutator 自动合成一个新 Query，系统未必提前知道它需要看：

* socket；
  -进程组；
  -FD；
  -rename；
  -namespace binding；
  -还是 delayed child。

Observation-first 的作用是：

> 让真实执行轨迹告诉 SyncFuzz：这个 Query 到底修改了哪些 OS 对象，产生了哪些仍然存活的能力，以及后续差分应该关注什么。

这会降低 Query、Oracle 和具体 OS resource 之间的手工绑定。

---

# 三、放到 Unix Listener 案例里是什么样的

对你当前的 Motivation Example，第一阶段的广泛 tracing 可能观察到下面的事件链。

## Branch A

```text
shell PID
  └─ fork/exec → malicious broker PID
                   ├─ unlinkat(trusted.sock)
                   ├─ socket(AF_UNIX)
                   ├─ bind(trusted.sock)
                   └─ listen()
```

## Branch A 被丢弃

eBPF 观察到：

```text
no exit(malicious broker PID)
no kill(malicious broker PID)
listening FD remains open
```

## Branch B

```text
trusted client PID
  ├─ socket(AF_UNIX)
  ├─ connect(trusted.sock)
  └─ send(secret)
```

同时：

```text
malicious broker PID
  ├─ accept()
  └─ recv(secret)
```

由此可以构造因果链：

```text
Branch A
  → spawned PID Pm
  → Pm rebound pathname p
  → Branch A discarded
  → Pm remained alive
  → Branch B connected to p
  → Pm accepted the connection
  → Pm received post-fork secret
```

eBPF 在这里不只是证明“socket 存在”，而是帮助回答：

1. 谁创建了 listener？
2. listener 属于哪条进程 lineage？
3. endpoint 是新建还是替换？
4. Branch discard 时进程是否退出？
5. Branch B 最终连接到了哪个进程？
6. secret 是否真的进入了 Branch A 的残留进程？

这才是 OS-level root-cause analysis。

---

# 四、所谓 prune 可以有三种不同含义

这点需要和博士进一步确认，因为三种做法的工程意义不同。

## 1. Hook pruning

直接卸载不需要的 eBPF program，只保留：

```text
process + Unix socket + FD
```

优点是运行时开销更低。

缺点是容易漏事件，而且动态 attach/detach 比较复杂。

---

## 2. Event filtering

所有基础 hook 仍然挂载，但通过 BPF map、cgroup ID、PID lineage、syscall mask 控制哪些事件上报。

例如：

```text
当前 Query 只上报：
  cgroup_id == target_run
  syscall ∈ {clone, execve, exit, unlinkat, bind, listen, connect, accept}
```

这是我更推荐的方案：

* hook 集稳定；
  -动态修改过滤策略；
  -避免频繁加载和卸载程序；
  -仍可保留关键的低成本事件；
  -不同 Query 只改变 observation policy。

严格来说，这不是 prune hook，而是 prune emitted events。

---

## 3. State-probe pruning

eBPF 只提供资源线索，随后减少用户态 snapshot 范围。

例如 broad trace 发现 Query 只接触：

```text
/workspace/.runtime/credential-broker.sock
PID 1661689
FD 7
```

那么 differential 阶段不必 hash 整个 workspace、枚举所有进程，而只检查：

* 这个 pathname；
  -这个 PID lineage；
  -这个 socket FD；
  -相关 peer identity。

这通常会带来最明显的性能收益，也最容易实现。

---

# 五、不能简单删除“没触发过”的 Hook

这是这个提议最大的风险。

假设 broad profiling 的 control run 没有出现：

```text
kill
exit
rename
```

不能因此认为这些事件不重要。

原因有三个。

## 1. Fault run 可能触发 control run 没有的事件

例如只有在 cancel/recovery 时才出现：

* kill；
  -process restart；
  -FD close；
  -workspace replacement。

所以 profiling 至少应覆盖：

```text
control pilot
fault pilot
recovery pilot
```

不能只跑正常路径。

---

## 2. 未出现的事件本身可能是关键证据

在 Unix listener 案例中：

> Branch discard 之后没有 `exit(Pm)`，也没有 `kill(Pm)`。

“没有发生退出或清理”正是进程残留的重要依据。

如果把 process-exit hook prune 掉，就无法区分：

* 进程一直存活；
  -旧进程退出后被另一个进程重建；
  -PID 被复用。

因此一些 hook 是为了提供 **negative evidence**，即使 pilot 中没有产生事件也应该保留。

---

## 3. 单次运行不能代表完整路径

LLM Agent 的执行具有变异性。某次使用 `renameat2`，另一次可能使用：

```text
unlinkat + openat
```

某次通过 Python socket，另一次通过 helper binary。

如果根据一次 run 做激进 pruning，观测计划会过拟合具体执行。

---

# 六、建议使用三层 Hook 集

不要把所有 hooks 简单分成“保留/删除”。建议设计三个层级。

## Core Hooks：始终保留

用于构建基本因果关系和 negative evidence：

* process fork/clone；
  -exec；
  -exit；
  -cgroup/process lineage；
  -FD close/dup；
  -关键 lifecycle 时间戳。

这是最低可信观测面。

## Resource-Family Hooks：按 Query 激活

根据 pilot trace 选择：

* filesystem namespace；
  -Unix socket；
  -TCP；
  -permission；
  -FD inheritance；
  -process group；
  -mount/namespace。

例如 Unix listener Query 启用：

```text
Unix socket + process + FD + namespace binding
```

## Exploratory Hooks：只在 profiling 阶段启用

用于发现未知资源：

* broader syscall coverage；
  -非常规 IPC；
  -xattr；
  -shared memory；
  -message queue；
  -scheduling；
  -mount operations。

这样得到：

```text
H(q)
=
H_{\mathrm{core}}
\cup
H_{\mathrm{selected}}(q)
```

而不是完全由一次 trace 决定。

---

# 七、可以形式化成一个 Observation Plan 生成算法

设完整 hook 集为 (H_0)。

先运行若干 pilot：

[
T_{\mathrm{pilot}}
==================

\operatorname{Trace}
\left(
q,H_0
\right)
]

从 trace 中提取资源和操作：

[
R_q
===

\operatorname{Footprint}
\left(
T_{\mathrm{pilot}}
\right)
]

例如：

[
R_q=
{
\text{process},
\text{Unix socket},
\text{pathname binding},
\text{FD}
}
]

然后根据资源依赖闭包选 hook：

[
H_q
===

H_{\mathrm{core}}
\cup
\operatorname{HooksFor}
\left(
\operatorname{Closure}(R_q)
\right)
]

这里必须使用 `Closure`。例如观察 Unix socket，不能只保留 `bind/connect`，还需要：

* process lineage；
  -FD lifecycle；
  -unlink；
  -exit；
  -可能的 rename。

最后使用 (H_q) 运行 differential：

[
D_q
===

\operatorname{Diff}
\left(
\operatorname{Trace}(q_{\mathrm{control}},H_q),
\operatorname{Trace}(q_{\mathrm{fault}},H_q)
\right)
]

这就是一个完整的方法组件。

---

# 八、这对根因分析有什么帮助

你们的根因分析最终可以从“人工解释”升级为“trace-guided root-cause localization”。

系统可以自动生成这样的摘要：

```yaml
logical_boundary:
  operation: checkpoint-fork
  restored_to: C0

persistent_os_resources:
  - type: process
    pid: 1661689
    origin_branch: A
    survived_boundary: true

  - type: unix_socket_endpoint
    path: credential-broker.sock
    operation: rebound
    current_owner_pid: 1661689

activation:
  branch: B
  operation: connect
  peer_pid: 1661689

consequence:
  operation: recv
  payload_class: post-fork-secret
```

由此定位根因：

```text
logical state restored to C0
+
Branch-A process survived
+
socket pathname remained rebound
+
Branch-B trusted client connected to surviving PID
```

这比静态地报告：

```text
socket exists after fork
```

强很多。

---

# 九、eBPF 能做什么，不能做什么

## eBPF 能证明

* Agent 及其 descendants 执行了哪些 OS 操作；
  -哪个 PID 创建或修改了资源；
  -进程、FD 和 socket 的 lineage；
  -资源是否跨 lifecycle boundary 存活；
  -Branch B 是否真正触达 Branch A 的资源；
  -数据流是否跨进程发生。

## eBPF 不能单独证明

* 哪个 LangGraph checkpoint 被恢复；
  -某个 PID 在语义上属于哪个 Branch；
  -框架是否承诺清理该资源；
  -该行为是否违反 Contract；
  -用户是否认为某个 client 是可信的。

这些仍然需要：

```text
Framework lifecycle events
+
Query IR
+
checkpoint metadata
+
contract
```

所以完整分析应当是：

```text
Framework trace：逻辑发生了什么
eBPF trace：OS 实际发生了什么
Query IR：预期激活链是什么
Contract：哪些关系被要求
```

eBPF 不是替代现有 Oracle，而是为 Oracle 提供 OS-level evidence 和自动 observation discovery。

---

# 十、对 FSE 论文的价值

这个想法比单纯“我们用 eBPF 收集系统调用”更有研究性。可以把它包装成：

> **Trace-guided observation-plan synthesis**

即：

1. Broad profiling 自动发现 Query 的 OS footprint；
2. Footprint 映射成 resource-aware observation plan；
3. Observation plan 驱动低开销 differential execution；
4. Cross-layer trace 用于 root-cause localization。

它能回答一个清晰的软件测试问题：

> 面对未知的 Agent-generated OS effects，如何自动决定应该观测哪些系统资源，才能构造有效的 differential oracle？

这与当前“引入 LLM 自动生成 Contract/Oracle”也可以结合：

```text
LLM：
  从 Query 和文档生成候选 contract

eBPF profiling：
  从真实执行生成 OS observation plan

Deterministic oracle：
  对 contract 和 trace 进行机器判定
```

分工很合理：

* LLM 负责语义 specification；
* eBPF 负责事实 discovery；
  -规则系统负责最终判定。

---

# 十一、我建议的最小实现

先不要做复杂的自动 hook 编译器。第一版可以这样：

## Step 1：定义固定 Broad Profile

覆盖：

```text
process
filesystem namespace
FD
Unix socket
TCP
permission
```

## Step 2：将事件映射为 Resource Footprint

输出：

```json
{
  "resource_classes": [
    "process",
    "unix_socket",
    "fd"
  ],
  "operations": [
    "spawn",
    "unlink",
    "bind",
    "listen",
    "connect",
    "accept"
  ],
  "paths": [
    "credential-broker.sock"
  ]
}
```

## Step 3：Observation Policy

从静态映射表选择：

```text
unix_socket
→ bind/connect/accept/unlink + process + fd + exit
```

## Step 4：通过 BPF Map 过滤

按以下字段过滤上报：

* cgroup ID；
  -run ID；
  -PID lineage；
  -syscall/resource mask；
  -相关路径 hash。

## Step 5：运行 Differential

比较：

* control；
  -fault；
  -fresh-runtime control；
  -cleanup control。

## Step 6：生成 Root-cause Report

自动输出：

```text
surviving resource
originating branch
boundary crossed
successor activation
causal OS operations
```

---

# 最终判断

这个提议是好的，而且与当前转向 FSE、聚焦 (\langle A,O\rangle) 的方向非常一致。但应该把它理解为：

> **利用宽覆盖 eBPF profiling 自动推导 Query-specific OS observation plan，再使用该计划进行低噪声 differential testing 和根因定位。**

而不是：

> 跑一次，哪个 hook 没响就删哪个。

后者很容易漏掉 fault-only events、negative evidence 和执行变体；前者则可以成为 SyncFuzz 一个独立且有说服力的方法贡献。

最值得推进的核心抽象是：

[
\boxed{
\text{Query}
\rightarrow
\text{Broad OS Trace}
\rightarrow
\text{Resource Footprint}
\rightarrow
\text{Observation Plan}
\rightarrow
\text{Differential Oracle}
}
]

这条链一旦落地，你们就不再只是“提前知道要看 socket，所以去查 socket”，而是能够声称：

> **SyncFuzz automatically discovers which OS state is relevant to a generated recovery query and synthesizes the observation plan needed to test it.**

---

Round 3
User: 我们要以最快的速度去做实现，所以我想选择 State-probe pruning。

ChatGPT:
我赞成，而且从“最快做出可用结果”的角度看，**State-probe pruning 是三种方案里最合适的**。

你们暂时不要动态卸载 eBPF hook，也不要根据每个 Query 重编译 tracing program。最小可行架构应该是：

> **eBPF tracer 保持固定；先用 trace 提取 Query 的 OS footprint，再只对相关对象执行精细的用户态 state probe。**

这样既能保留完整的 OS 事实，又能快速减少快照范围和 Oracle 手工配置。

---

# 一、建议冻结成这条流水线

```text
Existing/Pilot Run
        ↓
Fixed eBPF Trace
        ↓
Resource Footprint Extraction
        ↓
Observation Plan Compilation
        ↓
Control + Fault Run
        ↓
Targeted State Probes
        ↓
Differential Oracle
        ↓
Root-Cause Report
```

其中真正新增的只有三个模块：

1. `FootprintExtractor`
2. `ObservationPlanCompiler`
3. `TargetedProbeRunner`

你们已经有 eBPF trace、process snapshot、filesystem metadata 和 differential oracle，所以不用重做基础设施。

---

# 二、最快的实现策略：先做对象级 pruning

State-probe pruning 其实还可以分两层：

## Probe-family pruning

决定启用哪些探针：

```text
filesystem
process
fd
unix_socket
shell_context
```

## Object-level pruning

决定具体探测哪些对象：

```text
/workspace/.runtime/credential-broker.sock
PID lineage rooted at 1661689
socket inode 12345
FDs owned by selected descendants
```

我建议第一版两层都做，但重点放在 **object-level pruning**。因为你们现在最重的操作很可能不是判断启用哪个模块，而是：

* hash 整个 workspace；
  -递归扫描所有文件；
  -枚举所有进程；
  -读取所有 `/proc/<pid>/fd`；
  -检查所有 Unix socket。

缩小到“本 Query 碰过的路径和进程”，性能收益最大，代码也最简单。

---

# 三、固定保留 eBPF，不做 Hook pruning

第一版 eBPF 继续固定观测：

```text
process:
  fork / clone / exec / exit

filesystem namespace:
  open/create / unlink / rename / symlink
  chmod / xattr（已有就保留）

fd:
  dup / close

unix socket:
  socket / bind / listen / connect / accept
```

然后按 `run_id` 或 cgroup 过滤。

原因是：

* 已经实现的 tracer 不需要改；
* eBPF trace 用于发现相关资源；
* fault run 可能产生 pilot run 没出现的新事件；
* process exit 等“没有发生的事件”也是根因证据；
  -论文里更容易解释：tracing coverage 固定，pruning 只改变 snapshot policy。

因此方法最好命名为：

> **Trace-guided state-probe pruning**

而不是：

> eBPF hook pruning。

---

# 四、Resource Footprint 应该长什么样

从 `trace.jsonl`、process lineage 和已有 artifacts 中提取一个结构化 footprint：

```json
{
  "query_id": "unix-listener-residue-fork",
  "resource_classes": [
    "process",
    "filesystem_namespace",
    "unix_socket",
    "file_descriptor"
  ],
  "paths": [
    {
      "path": "/workspace/.runtime/credential-broker.sock",
      "operations": ["unlink", "bind", "connect"],
      "origin_phase": "plant"
    },
    {
      "path": "/workspace/captured-secrets.jsonl",
      "operations": ["create", "write"],
      "origin_phase": "activation"
    }
  ],
  "processes": [
    {
      "origin": "branch-a",
      "role": "listener",
      "executable": "python3",
      "lineage_key": "shell-child-listener"
    }
  ],
  "operations": [
    "spawn",
    "unlink",
    "bind",
    "listen",
    "connect",
    "accept",
    "recv",
    "write"
  ]
}
```

这里不要依赖固定 PID，因为 replay 后 PID 会变化。

更稳定的进程标识是：

```text
origin lifecycle phase
+ parent lineage
+ executable
+ argv fingerprint
+ first observed effect
```

例如：

```text
Branch A
→ child of persistent shell
→ python3
→ bind credential-broker.sock
```

---

# 五、Observation Plan 的最小 Schema

Footprint 再编译成真正执行的 state-probe plan：

```yaml
query_id: unix-listener-residue-fork

checkpoints:
  - before_plant
  - after_plant
  - after_recovery
  - after_activation

probes:
  process:
    enabled: true
    selection:
      - descendants_matching:
          executable: python3
          effect_path: credential-broker.sock
    fields:
      - alive
      - ppid
      - start_time
      - process_group
      - command_line

  filesystem:
    enabled: true
    paths:
      - .runtime/credential-broker.sock
      - captured-secrets.jsonl
    fields:
      - exists
      - type
      - inode
      - mode
      - content_hash

  unix_socket:
    enabled: true
    paths:
      - .runtime/credential-broker.sock
    fields:
      - socket_type
      - listening
      - owner_pid
      - peer_pid
      - connectivity

  fd:
    enabled: true
    process_selectors:
      - role: listener
    fields:
      - fd_number
      - target
      - socket_inode
      - open_state

  shell_context:
    enabled: false
```

这个 plan 同时具备三个作用：

1. 控制实际 probe；
   2.记录本次 Oracle 依赖哪些事实；
   3.形成可复现 artifact。

---

# 六、第一版只需要一个静态映射表

不需要做复杂规则引擎。直接写：

```go
var ProbeRules = map[ResourceClass]ProbeRule{
    ResourceProcess: {
        ProbeFamilies: []ProbeFamily{
            ProbeProcess,
        },
    },
    ResourceFilesystemNamespace: {
        ProbeFamilies: []ProbeFamily{
            ProbeFilesystem,
        },
    },
    ResourceUnixSocket: {
        ProbeFamilies: []ProbeFamily{
            ProbeUnixSocket,
            ProbeProcess,
            ProbeFD,
            ProbeFilesystem,
        },
    },
    ResourceOpenFD: {
        ProbeFamilies: []ProbeFamily{
            ProbeFD,
            ProbeProcess,
        },
    },
    ResourceShellContext: {
        ProbeFamilies: []ProbeFamily{
            ProbeShellContext,
        },
    },
}
```

重点是要做**依赖闭包**。

例如发现 `unix_socket`，不能只启用 Unix socket probe，还必须带上：

```text
unix_socket
→ filesystem pathname
→ listener process
→ listening FD
```

所以：

```text
UnixSocket
⇒ {UnixSocket, Filesystem, Process, FD}
```

这是第一版最重要的规则。

---

# 七、推荐的 Go 数据结构

可以新增一个包：

```text
internal/syncfuzz/observation/
```

最小结构如下：

```go
package observation

type ResourceClass string

const (
	ResourceProcess             ResourceClass = "process"
	ResourceFilesystemNamespace ResourceClass = "filesystem_namespace"
	ResourceUnixSocket          ResourceClass = "unix_socket"
	ResourceFileDescriptor      ResourceClass = "file_descriptor"
	ResourceShellContext        ResourceClass = "shell_context"
)

type PathFootprint struct {
	Path        string   `json:"path"`
	Operations  []string `json:"operations"`
	OriginPhase string   `json:"origin_phase,omitempty"`
}

type ProcessFootprint struct {
	Executable string `json:"executable,omitempty"`
	ArgvHash   string `json:"argv_hash,omitempty"`
	EffectPath string `json:"effect_path,omitempty"`
	Origin     string `json:"origin,omitempty"`
	Role       string `json:"role,omitempty"`
}

type ResourceFootprint struct {
	QueryID         string             `json:"query_id"`
	ResourceClasses []ResourceClass    `json:"resource_classes"`
	Paths           []PathFootprint    `json:"paths,omitempty"`
	Processes       []ProcessFootprint `json:"processes,omitempty"`
	Operations      []string           `json:"operations,omitempty"`
}

type ObservationPlan struct {
	QueryID     string        `json:"query_id"`
	Checkpoints []string      `json:"checkpoints"`
	Filesystem  FilesystemPlan `json:"filesystem"`
	Process     ProcessPlan    `json:"process"`
	FD          FDPlan         `json:"fd"`
	UnixSocket  UnixSocketPlan `json:"unix_socket"`
	Shell       ShellPlan      `json:"shell"`
}
```

第一版不用做过度抽象。各 Probe Plan 直接放：

* `Enabled`
* `Paths`
* `Selectors`
* `Fields`

就够了。

---

# 八、FootprintExtractor 从哪里拿信息

为了最快实现，优先复用已有 artifacts，不要马上解析所有原始 syscall。

## 第一优先级：已有规范化 trace

从你们现在的：

```text
trace.jsonl
process-lineage.json
filesystem-metadata.json
state-trace.json
```

读取。

## 第二优先级：Scenario IR 提供的显式对象

Query 本身可能已经声明：

```text
primitive.path
activation.path
witness.path
```

这些对象直接纳入 footprint。

## 第三优先级：eBPF trace 补充发现对象

把 Agent 实际触达但 IR 未声明的路径、进程和 socket 加进去。

因此 footprint 应取并集：

[
F(q)=
F_{\mathrm{IR}}(q)
\cup
F_{\mathrm{trace}}(q)
]

这比只依赖 eBPF 更稳，因为某些状态对象可能在 pilot 中没有恰好被触发，但已经由 Query 明确声明。

---

# 九、不要只保留“被写”的对象

State probe 需要覆盖三个集合：

[
P(q)=W(q)\cup R(q)\cup D(q)
]

其中：

* (W(q))：Agent 写入或修改的对象；
* (R(q))：Agent 后续读取、连接或消费的对象；
* (D(q))：这些对象的依赖对象。

以 Unix socket 为例：

```text
W:
  credential-broker.sock 被 unlink/bind

R:
  Branch B 连接 credential-broker.sock

D:
  listener process
  listening FD
  socket inode
```

如果只看写集合，就可能记录 socket pathname，却漏掉真正的活跃 daemon。

---

# 十、Probe 执行时机

第一版固定四个 observation points，不要让每个 Query 自定义复杂时序：

```text
T0: before plant
T1: after plant
T2: after lifecycle recovery
T3: after activation
```

对应 Unix socket 案例：

| 时刻 | 需要确认的事实                        |
| -- | ------------------------------ |
| T0 | 路径由 benign broker 服务           |
| T1 | 路径已经被 malicious daemon 接管      |
| T2 | fork 后恶意 daemon 和 endpoint 仍存在 |
| T3 | Branch B 连接并产生 secret capture  |

状态表示：

[
O_0,O_1,O_2,O_3
]

Oracle 可以检测：

[
\operatorname{owner}*{O_0}(p)
\neq
\operatorname{owner}*{O_2}(p)
]

并且：

[
\operatorname{origin}
\left(
\operatorname{owner}_{O_2}(p)
\right)
=======

\text{Branch A}
]

再结合：

[
\operatorname{connect}*{B}(p)
\rightarrow
\operatorname{owner}*{O_2}(p)
]

---

# 十一、Differential 应比较两种差异

## 1. 时间差分

同一个 run 内：

```text
before plant
vs.
after plant
vs.
after recovery
vs.
after activation
```

这用于定位状态从什么时候发生变化、是否跨 boundary 存活。

## 2. Control/Fault 差分

```text
control run
vs.
fork/replay/fault run
```

这用于排除正常任务本身就会产生的效果。

最后报告：

```yaml
persistent_changes:
  - resource: unix_socket
    path: credential-broker.sock
    introduced_at: after_plant
    survived_until: after_recovery
    absent_from_logical_successor: true

activations:
  - actor: branch_b_trusted_client
    operation: connect
    target_owner: branch_a_listener
    consequence: future_secret_capture
```

---

# 十二、最快上线方式：先离线生成 Plan

你们已有大量归档结果，因此第一步甚至不需要修改运行主链路。

先实现一个命令：

```bash
syncfuzz target observation-plan \
  --run <existing-run-dir>
```

它读取已有 artifacts，输出：

```text
resource-footprint.json
observation-plan.json
```

这样可以立刻在：

* Unix listener；
  -open FD；
  -orphan process；
  -PATH poisoning；
  -file residue

上验证规则是否合理。

等离线结果稳定后，再接到 campaign：

```text
pilot run
→ observation-plan
→ control/fault targeted run
```

这能显著降低重构风险。

---

# 十三、建议的命令接口

第一版可以只实现三个命令：

```bash
# 从已有 run 提取 footprint
syncfuzz target footprint \
  --run <run-dir>

# 从 footprint 编译 observation plan
syncfuzz target plan-probes \
  --footprint resource-footprint.json

# 使用 plan 重跑
syncfuzz target run \
  --observation-plan observation-plan.json
```

之后再合成：

```bash
syncfuzz target differential \
  --query q.json \
  --auto-probe
```

---

# 十四、失败时必须有保守回退

自动 pruning 可能漏资源，因此需要一个简单 fallback：

```text
如果 footprint 为空
→ 使用 full probe

如果出现未知 resource class
→ full probe + 标记 needs-rule

如果 targeted run 出现未规划的新 eBPF resource
→ 扩展 plan 并重跑一次
```

可以表示成：

```text
pilot
→ plan v1
→ targeted run
→ detect unplanned resources?
     ├─ no  → evaluate
     └─ yes → plan v2 → rerun
```

这其实是一个非常简单的固定点迭代：

[
P_{i+1}
=======

P_i
\cup
\operatorname{NewResources}(T_i)
]

通常限制最多一次扩展重跑就够了。

---

# 十五、如何验证 pruning 没有破坏 Oracle

你们需要做一个小规模的 **probe fidelity experiment**：

在已有 confirmed corpus 上，同时运行：

1. Full probe；
2. Pruned probe。

比较：

* Oracle status；
  -attribution；
  -mismatch signature；
  -contract interpretation；
  -root-cause chain。

定义：

[
\operatorname{Fidelity}
=======================

\frac{
#\text{runs with equivalent verdict}
}{
#\text{runs}
}
]

同时测：

* probe 时间；
  -artifact 大小；
  -扫描路径数量；
  -枚举进程数量。

目标不是一定达到 100% 性能提升，而是证明：

> 在保持 verdict fidelity 的情况下，显著缩小 observation surface。

第一版用 20—30 个代表性 runs 就能得到可信结果。

---

# 十六、建议的两周实现顺序

## 第 1—2 天：Footprint schema

* 定义 ResourceFootprint；
  -从 Scenario IR 提取声明对象；
  -从现有 trace 提取 touched paths 和 process lineage。

## 第 3—4 天：Plan compiler

先支持五类：

* filesystem；
  -process；
  -FD；
  -Unix socket；
  -shell context。

实现静态依赖闭包。

## 第 5—6 天：Targeted probe runner

让已有 probes 接受：

```text
path allowlist
process selector
probe enabled flags
```

## 第 7 天：离线回放验证

对归档 runs 生成 observation plan，人工检查。

## 第 8—10 天：接入 differential runner

固定 T0—T3 observation points，并输出 plan artifact。

## 第 11—12 天：Fidelity comparison

Full vs pruned。

## 第 13—14 天：Root-cause summary

自动输出：

* surviving object；
  -origin phase；
  -boundary crossed；
  -successor activation；
  -关键状态转移。

---

# 十七、当前不应该做的事

为了最快实现，先不要做：

* 动态 attach/detach eBPF；
  -每个 Query 自动生成新的 BPF code；
  -复杂 syscall dependency analysis；
  -完全依赖 LLM 生成 probe；
  -全自动污点追踪；
  -跨主机资源追踪；
  -路径级 BPF filter；
  -实时自适应 probe policy。

这些都可以留给后续增强。

第一版最小目标就是：

> **从已有 trace 中提取 touched resource set，并让现有 state probes 只扫描这些资源及其依赖闭包。**

---

# 最终建议

你们可以把这部分方法暂时定义为：

> **SyncFuzz uses broad OS tracing to identify the resource footprint of each query, and then synthesizes a query-specific state-probe plan for differential execution.**

实现上则保持非常朴素：

[
\boxed{
\text{IR objects}
\cup
\text{eBPF touched objects}
\rightarrow
\text{static dependency rules}
\rightarrow
\text{probe allowlist}
}
]

这是当前最值得做的折中：

* 足够快；
* 不修改 eBPF 核心；
* 可以复用已有 artifacts；
* 能减少 Oracle 与手工 testcase 的绑定；
* 能支持 OS-level root-cause analysis；
* 也能作为 FSE 论文中的独立方法组件。
