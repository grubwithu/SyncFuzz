# MAF: Microsoft Agent Framwork

Microsoft Agent Framework（MAF）里的 **Workflows**，可以理解为微软把 AutoGen 的多 Agent 编排能力和 Semantic Kernel 的企业级状态管理进一步统一后，提供的一套**显式、可持久化、可恢复的图执行引擎**。

它不是“又一种智能 Agent”，而是 Agent 运行在其中的确定性控制平面：

```text
Agent / Function / Tool
          ↓
       Executor
          ↓
   Message + Edge
          ↓
 Superstep Scheduler
          ↓
 State + Checkpoint
```

对 SyncFuzz 来说，它很有价值，因为它和 LangGraph 的架构明显不同：LangGraph 当前主要暴露 node/thread/checkpoint 语义，而 MAF Workflows 以 **Executor、消息和 Superstep** 为核心，checkpoint 发生在同步屏障之后。你的原始报告将第二层 Target 放在 AutoGen；考虑到 MAF 已被微软定位为 AutoGen 和 Semantic Kernel 的直接继任者，可以将它作为新的 L2 Target，AutoGen则保留为历史架构对照。fileciteturn0file0 citeturn270807view3

# 一、MAF 整体由什么组成

Microsoft Agent Framework 目前把能力分为三类：

- **Agents**：LLM 驱动，动态选择工具和执行步骤；
- **Harness**：为长程任务提供 planning、文件、memory、approval、observability 等完整能力；
- **Workflows**：显式定义执行路径，将 Agent 和普通函数编排为图，并提供 checkpoint、HITL 和类型安全消息路由。

因此：

> Agent 决定“做什么”，Workflow 决定“由谁执行、消息流向哪里、何时同步、何时保存状态”。

官方也建议：开放式、对话式任务适合 Agent；步骤明确、需要多个组件协调和显式执行顺序的任务适合 Workflow。citeturn270807view3turn476025view0

---

# 二、两套 Workflow API

MAF Workflows 当前提供两种编写方式。

## 1. Functional Workflow API

Python 中通过：

```python
@workflow
@step
```

使用普通的 `async` 函数、`if/else`、循环和 `asyncio.gather` 编写。

特点是：

- 代码接近普通 Python；
- 每个 `@step` 可以形成缓存和恢复单位；
-适合线性流程和自定义控制流；
-目前官方标为 experimental。

## 2. Graph API

使用：

```python
WorkflowBuilder
Executor
Edge
```

显式构造有向图。

它适合：

- 固定拓扑；
-类型验证；
-fan-out / fan-in；
-条件分支；
-明确的消息路由；
-superstep 并行执行；
-superstep 边界 checkpoint。

对于 SyncFuzz，应该优先选择 **Graph API**。因为 Functional API 会把恢复语义隐藏在普通 Python 控制流里，而 Graph API 的 Executor、Edge、Superstep 和 Checkpoint 都有明确事件与标识，方便 Adapter 插桩。citeturn476025view0

---

# 三、核心概念一：Executor

Executor 是 Workflow 中真正执行工作的节点。

一个 Executor 可以是：

- 普通转换函数；
-数据库或 HTTP API 调用；
-自定义业务逻辑；
-文件或 Shell 操作；
-一个完整的 AI Agent。

它接收类型化消息、执行操作，然后：

- 向下游 Executor 发送新消息；
-向 Workflow 输出结果；
-发出自定义事件；
-或者只产生副作用而不发送任何消息。

例如一个简化的 Executor：

```python
from agent_framework import Executor, WorkflowContext, handler


class UpperCaseExecutor(Executor):
    @handler
    async def handle(
        self,
        text: str,
        ctx: WorkflowContext[str],
    ) -> None:
        await ctx.send_message(text.upper())
```

Executor 可以持有可变内部状态。但如果同一个 Executor 实例被多个 Workflow run 复用，它必须正确 reset，否则状态可能在不同任务间泄漏。官方文档明确警告共享有状态 Executor 会造成 stale state，并提供 reset 生命周期接口。citeturn133992view0turn378245view0

这对 SyncFuzz 很重要，因为它天然产生一类不同于 LangGraph workspace residue 的 Target：

```text
Workflow A logical state
        ↓
shared Executor object
        ↓
Workflow B inherits mutable state
```

但这类结果需要正确分类：官方已经明确警告不要不安全地复用有状态 Executor，因此单纯复现它可能是 documented hazard，而不是未知框架漏洞。

---

# 四、核心概念二：Edge 与消息路由

Executor 之间通过 Edge 连接。

支持的主要形式包括：

- direct：一对一；
- conditional：条件路由；
- switch-case：多分支选择；
- fan-out：一条消息发送到多个 Executor；
- fan-in：多个 Executor 的结果汇总到一个 Executor。

消息具有类型，构建 Workflow 时会检查：

- 相邻 Executor 的类型是否兼容；
-所有 Executor 是否可达；
-Edge 是否有效；
-是否存在重复或错误连接。

这使得 Workflow 的运行时控制流比普通 Agent loop 更明确。citeturn133992view1turn133992view3

---

# 五、最关键的机制：Superstep

MAF Graph Workflow 采用类似 Pregel 的 **Bulk Synchronous Parallel** 模型。

每个 Superstep 依次做：

1. 收集上一 Superstep 产生的待处理消息；
2. 根据 Edge 路由消息；
3. 并发运行所有被激活的 Executor；
4. 等待这些 Executor 全部完成；
5. 将新消息排入下一个 Superstep。

可以表示为：

```text
Superstep N
┌───────────────────┐
│ 收集 pending 消息  │
└─────────┬─────────┘
          ↓
┌───────────────────┐
│ 根据 Edge 路由     │
└─────────┬─────────┘
          ↓
┌───────────────────┐
│ 并发执行 Executors │
└─────────┬─────────┘
          ↓
    synchronization barrier
          ↓
┌───────────────────┐
│ 形成下轮消息与状态 │
└───────────────────┘
```

**Checkpoint 在每个 Superstep 结束、其中所有 Executor 完成后创建。**

这和“每个 tool call 后立刻 checkpoint”不是一回事。citeturn378245view3turn270807view1

---

# 六、为什么 Superstep 对 SyncFuzz 很关键

它会形成一个很有价值的原子性裂缝。

假设同一个 Superstep 中并行执行两个 Executor：

```text
Executor A：向外部服务完成支付
Executor B：执行一个可能失败的检查
```

可能发生：

```text
A 已提交不可逆 Effect
B 抛出异常
Superstep 未正常结束
本轮 Checkpoint 尚未形成
```

如果系统随后从上一个 checkpoint 恢复，A 和 B 都可能再次执行。

即：

\[
X_{\text{external}} = \text{A 已执行}
\]

但：

\[
A_{\text{checkpoint}} = \text{本 Superstep 尚未完成}
\]

恢复后可能出现：

\[
\text{A 再次执行}
\]

这正是 SyncFuzz 所关注的：

> effect first, bookkeeping later。

这是根据官方的 Superstep 屏障和 checkpoint 时机推导出的攻击窗口，而不是微软文档已经确认的具体漏洞。citeturn378245view3turn270807view1

相比 LangGraph 的 persistent-shell residue，MAF 更适合测试：

- 并行 Executor 的部分提交；
- Superstep 失败后的 effect replay；
- checkpoint 之前的外部效果；
- Executor 状态和 Workflow 状态不一致。

---

# 七、MAF Checkpoint 保存什么

Graph Workflow 的 checkpoint 在 Superstep 结束时捕获：

- 所有 Executor 的当前状态；
- 下一 Superstep 的 pending messages；
- pending requests 和 responses；
- shared state。

Checkpoint 不是默认自动出现的：运行时需要配置 `CheckpointStorage` 或对应 checkpoint manager。citeturn270807view1

Python 当前提供三种内置存储：

| 存储 | 持久性 | 适用场景 |
|---|---|---|
| `InMemoryCheckpointStorage` | 仅当前进程 | 测试与短期运行 |
| `FileCheckpointStorage` | 本地磁盘 | 单机、进程重启恢复 |
| `CosmosCheckpointStorage` | Cosmos DB | 分布式、跨进程恢复 |

它们实现同一接口，因此可以在不修改 Workflow 和 Executor 的前提下切换。citeturn270807view1

这对 SyncFuzz 提供了一个很干净的实验变量：

```text
相同 Workflow
+ 相同 Scenario
+ 不同 CheckpointStorage
```

可以比较：

- 同进程恢复；
-进程重启后恢复；
-新实例从远程 checkpoint rehydrate。

---

# 八、Resume 与 Rehydrate 的区别

MAF 有两种恢复模式。

## 1. Resume：在原 Workflow 上继续

Python 中大致是：

```python
async for event in workflow.run(
    checkpoint_id=saved_checkpoint.checkpoint_id,
    stream=True,
):
    ...
```

逻辑状态恢复到指定 checkpoint，然后继续运行。

## 2. Rehydrate：创建新 Workflow 实例

重新构造一个 Workflow，然后从旧 checkpoint 加载：

```python
new_workflow = builder.build()

async for event in new_workflow.run(
    checkpoint_id=saved_checkpoint.checkpoint_id,
    checkpoint_storage=checkpoint_storage,
    stream=True,
):
    ...
```

这里逻辑状态来自旧 checkpoint，但运行对象是新实例。citeturn270807view1

对 SyncFuzz 来说，这两个生命周期必须分开：

```text
same-instance resume
new-instance rehydration
```

它们可能产生完全不同的状态结果：

| 状态 | Same-instance resume | New-instance rehydrate |
|---|---|---|
| Executor Python 对象 | 可能继续存在 | 通常重新创建 |
| Workflow state | 恢复 | 恢复 |
| workspace | 通常仍在 | 取决于外部配置 |
| Shell process | 可能继续存在 | 取决于 Runtime |
| 后台进程 | 可能仍存在 | 旧进程甚至可能与新实例并存 |
| external effect | 不会自动回滚 | 不会自动回滚 |

最后三行是实验上需要测量的外部状态，并非 MAF 官方承诺的 checkpoint 范围。

---

# 九、Executor 状态并不是魔法保存的

MAF 文档这里非常关键。

如果一个 Executor 有内部状态：

```python
self.messages
self.counter
self.current_task
```

它必须显式实现：

```python
async def on_checkpoint_save(self) -> dict:
    return {...}

async def on_checkpoint_restore(self, state: dict) -> None:
    ...
```

否则该内部状态不会自动以开发者期望的方式参与恢复。citeturn270807view1turn133992view0

这意味着 MAF 中至少存在三种状态：

\[
S_{\text{workflow}}
\]

框架管理的消息、shared state 和 pending request；

\[
S_{\text{executor}}
\]

Executor 自己显式保存的内部状态；

\[
S_{\text{external}}
\]

文件、Shell、进程、网络和远程 API。

Checkpoint 的正确性依赖开发者把需要保存的 Executor 状态显式注册进去，而 external state 则根本不属于普通 checkpoint 的自动管理范围。

这非常符合 SyncFuzz 的跨状态域模型。

---

# 十、Workflow State 的可见性

Workflow state 可以供不同 Executor 共享，但其可见性服从 Superstep 规则：

- 写状态的 Executor 在当前 handler 中可以立刻读到更新；
-其他 Executor 从下一个 Superstep 开始才能看到；
-未指定 scope 时，状态位于 Executor 私有默认 scope；
-指定同一个 scope name 时，不同 Executor 可以共享。citeturn378245view1

这会产生另一组很有意思的测试：

```text
Executor A 更新共享状态
Executor B 在同一 Superstep 完成外部 Effect
Superstep 随后失败
```

此时需要判断：

- A 的状态是否进入 checkpoint；
- B 的 external effect 是否已经发生；
-恢复后 B 是否重复执行；
-共享状态与外部状态是否形成矛盾。

---

# 十一、状态隔离本身就是一个重点

官方文档明确表示：

- 不推荐在不同任务间复用同一个 Workflow 实例；
-共享 Executor 实例会共享其内部可变状态；
-应当为每个请求创建新的 Workflow 和 Executor；
-Agent thread 默认会跨同一 Workflow 实例的多个 run 持久化；
-如果同一个 Workflow 实例被用于不同任务，可能出现意外状态共享。citeturn378245view0

这给 SyncFuzz 提供了两种实验类型：

## 合规配置测试

每次创建新 Workflow 和 Executor，但测试 checkpoint 与外部环境的失同步。

这才适合寻找框架或安全组合层问题。

## 故意共享配置测试

复用 Workflow/Executor，验证官方已经提醒的泄漏。

这只能作为：

- known-answer seed；
-安全配置基线；
-文档化危险行为。

不能直接当作新漏洞。

---

# 十二、Observability 对 Target Adapter 很友好

MAF Workflows 原生发出：

- workflow started/completed/error；
-executor invoked/completed/failed；
-superstep started/completed；
-message send；
-request info；
-output/intermediate output。

同时支持 OpenTelemetry spans：

```text
workflow.session
workflow_invoke
executor.process
edge_group.process
message.send
```

并携带：

- workflow ID；
-session ID；
-executor ID 和 type；
-message source/target；
-edge delivery status；
-error 信息。

这意味着 SyncFuzz Adapter 不必修改框架核心代码，就可以获得相当完整的生命周期 trace。citeturn378245view4turn476025view2

建议直接映射成：

```text
workflow.session       → run_id
workflow_invoke        → invocation_id
superstep_started      → lifecycle P0
executor_invoked       → dispatch
executor OS effect     → P3/P4
executor_completed     → result available
superstep_completed    → checkpoint boundary
```

然后再把 eBPF 的 PID/process/file/network effect 接入这条时间线。

---

# 十三、HITL 与 Authority State

MAF 的 Human-in-the-loop 通过 request/response 机制完成。

Executor 可以：

```python
await ctx.request_info(...)
```

然后 Workflow 发出 `RequestInfoEvent`，外部系统处理请求并把响应发回来。框架会把响应路由回原 Executor；pending requests 和 responses 也属于 checkpoint 所保存的状态。citeturn802558view1turn270807view1

这对 SyncFuzz 的 Authority Resurrection 很有价值：

```text
Executor 请求一次性 approval
→ approval 被外部 AuthorityServer 消费
→ 后续 Superstep 故障
→ 从旧 checkpoint 恢复 pending request/response
→ approval 是否被重新使用？
```

可以分别测试：

- approval request 已发出但未响应；
-响应已返回但 checkpoint 尚未完成；
-一次性 token 已消费但 Workflow 未记账；
-rehydrate 后旧响应是否仍绑定正确 request。

这比 synthetic `authority-resurrection` seed 更接近真实框架原生语义。

---

# 十四、MAF 没有什么

最需要认清的是：

> **MAF Workflows 本身没有一个等同于 `ShellToolMiddleware` 的官方 Persistent Shell。**

Executor 可以执行任意代码或外部操作，但 Shell runtime 需要：

- 自定义 ShellExecutor；
-包装 Agent tool；
-或连接外部 sandbox/runtime。

因此 MAF 适合测试 **checkpoint、superstep、executor state 和恢复边界**，但不天然代表一个完整 Terminal Agent 产品。

如果你们自己写了一个巨大而脆弱的 ShellExecutor，再找到漏洞，评审可能会说漏洞来自自定义实现。

所以 Adapter 应保持极薄：

```text
MAF Workflow
    ↓
官方 Executor 接口
    ↓
最小 Shell Runtime Adapter
    ↓
同一个 SyncFuzz sandbox/probe
```

ShellExecutor 只负责：

- 接收结构化 command；
-调用统一 Runtime；
-返回 stdout/stderr 和 command ID。

不要自己实现 retry、checkpoint 或 recovery。

---

# 十五、最适合 SyncFuzz 的第一个 MAF Workflow

建议先不用 LLM，构造一个确定性图：

```text
InputExecutor
      ↓
ShellExecutor
      ↓
EffectRecordExecutor
      ↓
OutputExecutor
```

每个 Executor 位于独立 Superstep。

第一批实验：

## Case 1：External Effect Replay

```text
Superstep 1：Shell/API Executor 提交外部 effect
Superstep 2：Fault Executor 失败
恢复到上一个 checkpoint
观察 effect 是否重复
```

## Case 2：Parallel Partial Commit

```text
               ┌─ EffectExecutor：提交 effect ─┐
StartExecutor ─┤                              ├─ Barrier
               └─ FailureExecutor：延迟后失败 ─┘
```

这是最体现 MAF Superstep 架构的测试。

## Case 3：Executor-State Desynchronization

Executor 内部 counter 已增长，但未正确实现 checkpoint hook；恢复后逻辑状态与外部动作计数不一致。

这属于 documented developer obligation，可作为 known-answer，而非主要漏洞。

## Case 4：Same-instance Resume vs Rehydrate

相同 checkpoint 分别：

- 在原 Workflow 上 resume；
-在新 Workflow 上 rehydrate。

比较：

- Executor identity；
-process identity；
-workspace；
-external ledger；
-pending messages。

## Case 5：HITL Approval Replay

围绕 RequestInfoEvent、一次性 approval 和 checkpoint 边界注入故障。

---

# 十六、它与 LangGraph 形成什么对照

| 维度 | LangGraph + Shell | MAF Workflows |
|---|---|---|
| 执行单位 | node / graph transition | Executor / Superstep |
| 并行模型 | 取决于 Graph | 明确 BSP barrier |
| Checkpoint | thread/graph state | Superstep 全局状态 |
| Shell | 官方 persistent shell middleware | 需薄 ShellExecutor |
| 恢复 | replay/fork | resume/rehydrate |
| Executor 内部状态 | node/state schema | 显式 save/restore hook |
| HITL | interrupt | request/response + pending state |
| 最独特攻击面 | branch/shell residue | parallel partial commit |

因此它不是为了重复 LangGraph 的文件 residue 实验，而是为了回答：

> 当框架声称在 Superstep 边界提供一致 checkpoint 时，同一 Superstep 中已外部化的部分效果会发生什么？

---

# 十七、适合作为第几个 Target

我的判断是：

- **LangGraph**：继续作为第一个成熟 Target；
- **Microsoft Agent Framework Workflows**：非常适合第二个框架级 Target；
- **OpenHands/OpenClaw**：作为第三个完整产品或 Agent runtime Target。

MAF 的优势是：

- checkpoint 机制原生且文档明确；
-有 same-instance 与 new-instance 两种恢复；
-有三种持久化后端；
-有清晰 Superstep 事件；
-有 HITL pending state；
-与 LangGraph 架构差异足够大。

它的缺点是：

- Shell 不是原生核心能力；
-你们必须添加薄 Executor；
-它更像编排框架，而不是完整 Terminal Agent；
-部分“不保存状态”场景是明确的开发者责任，不能误报。

# 最终评价

> **MAF Workflows 最值得 SyncFuzz 测的，不是“恢复后文件还在不在”，而是 Superstep checkpoint 能否与同一轮中已经发生的 Executor、External 和 Authority Effect 保持一致。**

最有辨识度的研究场景是：

```text
同一 Superstep 中
Executor A 已提交不可逆 Effect
Executor B 发生故障
同步屏障未完成
Checkpoint 未形成
恢复后 A 再次执行
```

如果能围绕这个机制自动生成、变异故障时序，并证明重复 effect、权限复活或跨实例残留，MAF 会比简单增加另一个 persistent-workspace Target 更有论文价值。


---

> MAF 有没有现成的、权威官方的 Shell-enabled Agent 项目，并且没那么复杂的那种

有，而且我找到了一个**非常接近你要求的官方对象**：

> **Microsoft Agent Framework 官方仓库中的 `GitHubCopilotAgent` Shell 示例。**

具体是官方仓库：

```text
python/samples/02-agents/providers/github_copilot/
└── github_copilot_with_shell.py
```

官方 README 明确说明，这组示例展示 `GitHubCopilotAgent` 在 MAF 中的用法，其中 `github_copilot_with_shell.py` 会开启 Shell command execution permission，让 Agent 执行列目录、查看系统信息等真实系统命令。它不是第三方 Demo，也不需要你们自己实现 Shell tool。([GitHub][1])

# 这个样例为什么合适

它基本满足三个条件。

## 1. 权威性足够

它位于微软官方 `microsoft/agent-framework` 仓库的 Python samples 目录中，使用的是 MAF 官方提供的 `GitHubCopilotAgent` provider。官方仓库目前将 MAF 定位为生产级 Agent 和 Workflow 框架，并提供 Python/.NET 的官方 Agent、Workflow、checkpoint 和 hosting samples。([GitHub][2])

## 2. Shell 是原生能力，不是你们自己拼的

官方示例通过 GitHub Copilot CLI 的 permission system 开启 Shell 权限。相同目录还提供：

```text
github_copilot_with_file_operations.py
github_copilot_with_session.py
github_copilot_with_multiple_permissions.py
```

也就是说，Shell、文件和 Session 都是该官方 provider 原生支持的能力。官方还专门警告，Shell、read、write、URL 权限会扩大 Agent 对系统的影响，应只启用任务必要的权限。([GitHub][1])

这比自己写：

```python
def shell(command):
    return subprocess.run(...)
```

强得多，因为发现不能简单归因于你们自定义 Shell tool 的实现错误。

## 3. 复杂度较低

它是一个单文件 sample，不是 OpenHands 那样包含前端、服务端、sandbox provider、数据库和消息队列的完整系统。

运行所需的主要条件是：

* 安装并登录 GitHub Copilot CLI；
* 有可用的 GitHub Copilot 订阅；
* 安装 `agent-framework-github-copilot --pre`；
* 配置权限回调。

官方 README 还说明可以配置 Copilot CLI 路径、模型、timeout、日志等级和 session home。([GitHub][1])

---

# 但它有一个关键缺口

这个 Shell sample 本身只是：

```text
GitHubCopilotAgent
        ↓
Copilot CLI
        ↓
Shell / filesystem
```

它**不是一个自带 MAF Workflow checkpoint 的示例**。

所以需要区分：

## `GitHubCopilotAgent` Session

官方 `github_copilot_with_session.py` 展示了：

* 创建 Session；
  -多轮复用 Session；
  -记录 `service_session_id`；
  -在新的 Agent 实例中通过 Session ID 恢复会话。

这些 Session 由 Copilot CLI 在服务侧持久化。([GitHub][3])

## MAF Workflow Checkpoint

MAF 官方另有：

```text
python/samples/03-workflows/checkpoint/
└── workflow_as_agent_checkpoint.py
```

这个示例展示：

* 将 Workflow 包装为 Agent；
  -给 `agent.run()` 传入 `checkpoint_storage`；
  -保存 Workflow execution state；
  -结合 Agent Session 和 Workflow checkpoint；
  -恢复 checkpoint。

官方特别强调：Session 保存对话历史，而 Checkpoint 保存 Workflow execution state，这两者互补但不是同一套状态。([GitHub][4])

所以目前官方仓库里，我没有看到一个单文件 sample 已经把下面三项完整组合好：

```text
GitHubCopilotAgent
+ Shell permission
+ Workflow checkpoint / resume
```

---

# 最适合 SyncFuzz 的做法

不是自己重新写一个 MAF Shell Agent，而是**组合两个官方样例**。

## 基础 Target：未经修改的官方 Shell Agent

先直接接入：

```text
github_copilot_with_shell.py
```

验证以下基础事实：

* Agent 实际执行了哪些命令；
* Shell command 是否跨 Agent run 共享状态；
* workspace 是否持久；
  -子进程生命周期；
  -权限 approval 行为；
  -Copilot CLI session identity。

这个 Target 可以叫：

```text
maf_github_copilot_shell
```

它是最小、最权威的 Shell-enabled MAF Agent。

---

## Session Target：Shell Agent + 官方 Session

然后把官方 Shell 示例和官方 Session 示例做非常小的合并：

```python
agent = GitHubCopilotAgent(
    instructions="You are a terminal assistant.",
    default_options={
        "on_permission_request": permission_handler,
    },
)

async with agent:
    session = agent.create_session()

    await agent.run(
        "Inspect the workspace using shell commands.",
        session=session,
    )
```

后续可以通过：

```python
session.service_session_id
```

在新的 Agent 实例中恢复同一个 Session。官方 Session sample 已经演示了这种跨实例恢复模式。([GitHub][3])

这个 Target 可以测试：

```text
Agent instance A 执行 OS Effect
→ 保存 Copilot session ID
→ 销毁 Agent instance A
→ 创建 Agent instance B
→ 恢复 Session
→ OS Effect 和 Agent 认知是否一致
```

它不是 Workflow checkpoint，但已经是很现实的：

> logical session recovery versus OS state persistence。

---

## 正式 Workflow Target：Shell Agent + MAF Checkpoint

最终再把 `GitHubCopilotAgent` 放入一个最小 Workflow。

MAF 官方 `_start-here/step2_agents_in_a_workflow.py` 已经展示了 Agent 可以直接作为 `WorkflowBuilder` 的节点：

```python
workflow = (
    WorkflowBuilder(start_executor=agent)
    .build()
)
```

官方 checkpoint sample 又展示了可以把 Workflow 包装成 Agent，并向运行调用传入 `checkpoint_storage`。([GitHub][5])

所以你们要做的最小组合大致是：

```python
from agent_framework import InMemoryCheckpointStorage, WorkflowBuilder
from agent_framework.github import GitHubCopilotAgent

shell_agent = GitHubCopilotAgent(
    instructions=(
        "You are a terminal agent. Use shell commands when needed."
    ),
    default_options={
        "on_permission_request": permission_handler,
    },
)

workflow = WorkflowBuilder(
    start_executor=shell_agent,
).build()

workflow_agent = workflow.as_agent()
checkpoint_storage = InMemoryCheckpointStorage()

response = await workflow_agent.run(
    "Inspect and modify the test workspace.",
    checkpoint_storage=checkpoint_storage,
)
```

具体 API 可能要根据你们锁定的 MAF 版本做少量调整，但架构上只组合官方的：

* `GitHubCopilotAgent`；
  -Shell permission；
  -`WorkflowBuilder`；
  -`workflow.as_agent()`；
  -`CheckpointStorage`。

没有自定义 Agent loop，也没有自定义 Shell executor。

---

# 这个组合对 SyncFuzz 特别有价值

它实际上存在三套独立状态：

```text
MAF Workflow Checkpoint
        ↓
Copilot CLI Session
        ↓
Shell / OS Environment
```

可以写成：

[
S =
(
S_{\text{workflow}},
S_{\text{copilot-session}},
S_{\text{OS}}
)
]

这比简单的 MAF + 自定义 ShellExecutor 更有意义。

## MAF Workflow State

保存：

* Workflow execution；
  -Executor progression；
  -pending messages；
  -checkpoint metadata。

## Copilot Session State

保存：

* conversation history；
  -provider session ID；
  -Agent 上下文；
  -可能的工具交互历史。

官方 Session sample 明确说明该状态由 Copilot CLI 服务侧持久化，并可以在新的 Agent 实例中通过 session ID 继续。([GitHub][3])

## OS State

保存或遗留：

* 文件；
  -进程；
  -Shell command effect；
  -credential；
  -socket；
  -外部系统状态。

这三套状态具有不同生命周期，正好适合 SyncFuzz。

---

# 建议首先测试的场景

## 1. Workflow checkpoint 与 Shell Effect

```text
Workflow Agent 执行 Shell 命令创建文件
→ Workflow checkpoint
→ 恢复旧 checkpoint
→ 文件是否仍然存在
→ Agent 是否认为命令尚未执行
```

普通 workspace residue 可能是预期行为，所以还要观察后续是否重复执行或产生安全影响。

## 2. Copilot Session 恢复与 OS Effect

```text
Agent A 启动后台进程
→ 保存 service_session_id
→ 关闭 Agent A
→ Agent B 恢复 Session
→ 后台进程是否仍存活
→ Agent B 是否知道它存在
```

## 3. 权限请求与恢复

GitHub Copilot Agent 使用 permission callback 决定 Shell command 是否执行。官方 sample 还展示了 interactive approval handler。([GitHub][3])

可以测试：

```text
Shell 命令获得一次 approval
→ Effect 已发生
→ Workflow 或 Session 恢复
→ 相同或变形命令是否绕过再次 approval
```

## 4. Workflow checkpoint 与 Copilot Session 不一致

```text
MAF checkpoint 回到旧状态
Copilot session 保持较新状态
```

或者反过来：

```text
Copilot session 被恢复
MAF Workflow 已经进入后续节点
```

这里可能形成一种双重逻辑状态失同步，而不仅是 Agent/OS 状态失同步。

---

# 它的主要缺点

## 依赖 GitHub Copilot CLI

这意味着：

* 需要 Copilot 账号和订阅；
  -CLI 版本会影响实验；
* provider session 部分可能不是完全白盒；
  -离线复现不如普通 OpenAI/Foundry Agent 简单；
  -模型和 CLI 行为可能随版本变化。

因此必须固定：

```text
agent-framework version
agent-framework-github-copilot version
Copilot CLI version
model
permission configuration
working directory
Copilot home directory
```

## Workflow checkpoint 与 Provider Session 是两套恢复语义

这既是研究机会，也是工程复杂度来源。你必须记录：

```text
workflow checkpoint ID
Copilot service_session_id
agent instance ID
process ID
workspace identity
```

否则一旦出现 residue，很难判断是哪一层保留下来的。

## 它更像官方 Sample，而不是独立产品

论文里应当称为：

> an official MAF shell-enabled agent configuration

不要称为：

> a widely deployed production MAF application

---

# 我的明确推荐

如果目标是**尽快添加第二个权威、简单的 Target**，顺序应该是：

### MAF-1：官方 GitHub Copilot Shell Sample

几乎原样运行，只加 SyncFuzz instrumentation。

### MAF-2：官方 Shell Sample + 官方 Session 恢复

测试 Agent 跨实例恢复。

### MAF-3：官方 Shell Agent 包进官方最小 Workflow + CheckpointStorage

测试 MAF Workflow checkpoint、Copilot session 和 OS state 的三层失同步。

不要直接做复杂的多 Agent Workflow，也不要自己实现一个大型 ShellExecutor。

最准确的结论是：

> **MAF 有官方、简单的 Shell-enabled Agent 示例，即 `GitHubCopilotAgent` 的 Shell sample；但没有一个现成示例同时集成 Shell、Session 和 Workflow checkpoint。最稳妥的 Target 是以该官方 Shell sample 为核心，再用官方 Workflow checkpoint sample 做极薄组合。**

这已经足够权威，也足够简单，同时不会让别人说：“你测到的只是自己写坏的 Shell tool。”

[1]: https://github.com/microsoft/agent-framework/tree/main/python/samples/02-agents/providers/github_copilot "agent-framework/python/samples/02-agents/providers/github_copilot at main · microsoft/agent-framework · GitHub"
[2]: https://github.com/microsoft/agent-framework "GitHub - microsoft/agent-framework: A framework for building, orchestrating and deploying AI agents and multi-agent workflows with support for Python and .NET. · GitHub"
[3]: https://github.com/microsoft/agent-framework/blob/main/python/samples/02-agents/providers/github_copilot/github_copilot_with_session.py "agent-framework/python/samples/02-agents/providers/github_copilot/github_copilot_with_session.py at main · microsoft/agent-framework · GitHub"
[4]: https://github.com/microsoft/agent-framework/blob/main/python/samples/03-workflows/checkpoint/workflow_as_agent_checkpoint.py "agent-framework/python/samples/03-workflows/checkpoint/workflow_as_agent_checkpoint.py at main · microsoft/agent-framework · GitHub"
[5]: https://github.com/microsoft/agent-framework/blob/main/python/samples/03-workflows/_start-here/step2_agents_in_a_workflow.py "agent-framework/python/samples/03-workflows/_start-here/step2_agents_in_a_workflow.py at main · microsoft/agent-framework · GitHub"


