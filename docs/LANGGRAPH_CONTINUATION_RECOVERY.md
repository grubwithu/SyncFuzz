# LangGraph Continuation Recovery Protocol

> 状态：设计与当前 LangGraph adapter 的 continuation-evidence contract。
>
> 本文补充 [LANGGRAPH_END_TO_END_CLOSURE.md](LANGGRAPH_END_TO_END_CLOSURE.md) 的
> `HistoricalRecoverySet`。它不替代既有 pure-passive recovery 路径。

## 1. 问题与目标

StateFuzz 的第一条 Agent Query `P` 负责驱动一次真实任务，产生可由 eBPF、
ProfileRun 和 native checkpoint 共同证明的 effect。过去的 recovery set 在恢复
`Q_before`、`Q_after`、`Q_head` 后只做 passive observation；这能够观察恢复切面
与 retained OS state 的关系，但没有让恢复后的 Agent 接收新的正常用户输入。

continuation recovery 在每个恢复控制中再注入同一条冻结的普通 Agent Query `K`，
从而测试：同一真实 Agent 在不同 historical checkpoint 上继续执行时，会怎样与
同一类 retained OS state 交互。

它的目标是生成可审计的 **continuation evidence**，而不是让 LLM 判断结果是否
“正确”、是否是漏洞，或是否符合某个手写场景预期。

## 2. 实验形状

profile、frontier、StateSeed、materialization head 和 recorded plan 的形成过程不变。
对一个 frontier `(C_i, C_{i+1}]`，仍有三个 fresh recovery runtimes：

```text
Q_before = restore(C_i,     H, plan)
Q_after  = restore(C_{i+1}, H, plan)
Q_head   = restore(H,       H, plan)
```

若启用 continuation，则每条 query 的执行过程固定为：

```text
fresh runtime + frozen source snapshot
  -> exact native checkpoint restore(C)
  -> pre passive observation P_pre
  -> inject one frozen generic continuation query K
  -> wait for that continuation turn to finish
  -> post passive observation P_post
  -> emit recovery observation + continuation observation
```

唯一可变的 discovery coordinate 仍是 `C`。`before`、`after`、`head` 必须共享：

- 同一 StateSeed、frontier、materialization head 和 retention policy；
- 同一 immutable image、model/runtime configuration、source snapshot 与 recorded plan；
- 同一个 passive observation scope 与 probe mode；
- 同一条 `K` 的 exact bytes、SHA-256 和 `continuation_query_id`；
- 每条控制各自独立的 fresh runtime/container/workspace。

`Q_head` 仍是 no-rollback control。它不被 continuation 替代：若 head 无法给出可用
证据，before/after 的差异不能归因于 historical checkpoint cut。

## 3. Continuation Query 的约束

`ContinuationQuery` 是一个小的、adapter-neutral 的 input artifact：

```json
{
  "continuation_query_id": "continuation-query:<sha256>",
  "query": "Continue from the current state and briefly report progress. Do not deliberately modify external resources solely to verify them.",
  "query_sha256": "<sha256 of exact query bytes>"
}
```

示例仅说明形式，不规定唯一文本。实际 `K` 必须是普通、通用的 continuation
请求，不能包含：

- objective/resource 的名称、预期文件内容、socket path、checkpoint ID 或 eBPF
  trace；
- “如果资源还在则判成功/失败”之类的分支；
- 为确认某个 witness 而创建、删除、连接、重启或修复资源的特定命令；
- scenario-specific expected answer、LLM judge rubric 或 contract verdict。

因此，`K` 可以要求 Agent 正常继续工作或简短汇报进度，但不能编码“本实验期待
看到什么”。query identity 从 exact bytes 导出；recovery set 建立时复制并冻结其值，
三个 control 任一分支换成不同的 query ID 都必须被拒绝。

LangGraph 的 fork plan 与 recovery set 都记录这一冻结值。executor 在启动容器前
逐字段比较 query ID、exact bytes 与 SHA-256；一个 plan 是 pure-passive 而 set 请求
continuation，或两者文本不同，都会 fail closed。

这条 query 是实验刺激（stimulus），不是 Oracle。Agent 的文本答复、tool-call
文本或 command hash 可以作为审计 evidence，但不直接生成 `residual`、
`consistent`、`duplicate` 或 contract violation verdict，也不会反馈给 StateFuzz
generator 作为任务内容引导。

### 3.1 `K` 不等于未来的 `U'` / `RecoveryUsePlan`

当前 `K` 的约束保持不变：它必须是 generic、resource-agnostic 的正常后续输入，
不能为了验证某个 endpoint 而主动 `connect`、`open`、`exec` 或重建资源。因此一次
`P_pre -> K -> P_post` 完整执行只证明 continuation 行为被记录，**不**证明恢复后的
Agent 实际依赖了 profile 中的 resource。

V3 现在已有一个独立的、**fixture-only** `RecoveryUsePlan` 实现。它由冻结的
`Workload` 和 Unix-socket `EnvironmentProgram E` 派生，记录固定 normal request、
logical name、request digest 与 resource family；local calibration 用真实
`logical resolution -> connect -> role-tagged I/O` 构造 `UseEvidence`，并区分
run-local identity 与跨 fresh runtime 比较的 semantic identity。它不接入当前
LangGraph continuation executor，也不把 `K` 变成 endpoint-specific prompt。

LangGraph 现在已有一个受限的 target-side recovery-use collector：当 immutable `E` 已锁入
fork plan 且执行 frozen continuation 时，executor 会先 gated-create/start fresh recovery
container、绑定其独立 cgroup 的 resource collector、再释放 Agent。它要求同一 trace 中有
`connect(/workspace/<E.endpoint>)`，并在 retained active listener 的 append-only observer
log 中找到同一时间窗、同一 role 的 accept record。该 record 只保留 bounded request
length/digest、server-side response-sent 和固定 acknowledgement；不保留 application payload，
但 acknowledgement 证明客户端读取并确认响应。这个 collector已有 unit/artifact-contract
覆盖，但尚未完成一次真实 LangGraph target 的 five-control 运行。因此当前它是 `U'` 的
`connect + completed normal exchange` evidence；其缺口是 live validation、identity adapter
和 controls，而不是把 generic continuation 误称为 I/O。

`RecoveryUsePlan` 不是把特定路径、预期结果或 attack consequence 塞回 `K`。它是
独立的 typed observation contract；当前 generic continuation 继续作为 V2 的无偏
stimulus，不能因 local fixture 或上述受限 collector 的存在而升级为 target-side
realized-hazard evidence。

## 4. Pre/Post Evidence

每条带 continuation 的 `RecoveryObservation` 必须绑定完整的
`ContinuationEvidence`：

```json
{
  "continuation_query_id": "continuation-query:<sha256>",
  "pre_evidence": ["exact-checkpoint-restored", "<P_pre artifact/reference>"],
  "post_evidence": ["continuation-completed", "<P_post artifact/reference>"]
}
```

adapter 应在独立的 continuation observation artifact 中记录至少以下事实：

- selected native coordinate 被唯一解析并 exact restore；
- `P_pre` 与 `P_post` 的固定 passive observation；
- 注入的 user message 的 query ID 与 SHA-256，且只调用一次；
- continuation 是否完成、其 message/tool lifecycle 的摘要，以及 continuation 后
  durable checkpoint history 的摘要。

`P_pre` 的作用是界定“continuation 开始前已经保留的状态”；`P_post` 的作用是
界定“continuation 期间之后的状态”。它们不能读取被保护的文件内容、主动连接
listener，或通过为验证而重建资源来改变观察对象。任何缺少 pre/post evidence、
query ID 不匹配、restore 非唯一、runtime 不 fresh 或 continuation 不完整的结果都
应拒绝进入 relation projection。

pre/post 时间切分有助于区分 retained residue 与 continuation 期间出现的变化，
但它本身不证明后者由某条具体 tool call 因果产生，也不说明产品 contract 被违反。
需要 causal attribution、multiplicity proof 或 contract interpretation 的结论仍由
各自独立的 evidence 层负责。

## 5. 与纯 Passive Recovery 的兼容性

纯 passive recovery 是保持支持的第一类实验：`ContinuationQuery` 为空，恢复后
只执行原有的 passive observer，并保留当前 artifact/分类语义。这是已有实验的
可比基线，尤其适合先确认 `<A_C, O_H>` 本身是否存在 mixed state。

启用 continuation 时，adapter 在一次 exact restore 后执行 `P_pre -> K -> P_post`；
不启用时，它继续执行纯 passive 路径，不伪造 continuation evidence。两类结果必须
在 artifact 中显式区分，不能将 passive-only 的 absence of activity 解释成
continuation 的负结果，也不能把 continuation 造成的 post-state 变化倒灌为恢复前
residue。

推荐实验顺序是：

1. 先用 pure-passive `Q_before/Q_after/Q_head` 建立 StateSeed、head consistency
   与基础 A/O relation；
2. 冻结一个 generic `K`，在相同 seed/plan 下运行完整 continuation set；
3. 比较每个 control 的 `P_pre`、continuation evidence 和 `P_post`，必要时做独立
   重复试验；
4. 只将证据投影为 relation、causal-evidence status 或显式 contract verdict，绝不
   从 LLM 自由文本直接升级为安全结论。

## 6. 解释边界

一个成功的 continuation run 只说明：在受冻结条件约束的 fresh recovery runtime
中，Agent 接收了同一条后续输入，且前后 passive evidence 被完整记录。它可以帮助
发现“逻辑历史不同但 retained OS state 相同”如何影响后续 Agent 行为。

它不能单独说明：

- retained state 必然是 bug 或 security vulnerability；
- Agent 的文字回答真实反映 OS 状态；
- `P_post` 的变化由某个特定 tool call 造成；
- 不同 control 的模型行为可在单次运行中统计比较。

后两项分别需要 causal evidence 和独立重复/模型稳定性设计；安全结论还需要一个
单独声明、可审计的 recovery contract。这样 continuation 扩展的是测试的行为面，
不是把框架带回“按场景手写最终 Oracle”的路线。
