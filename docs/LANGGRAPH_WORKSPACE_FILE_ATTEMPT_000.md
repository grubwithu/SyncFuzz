# LangGraph Workspace-File StateFuzz Run: Attempt 000

> 实验记录：`runs/langgraph-workspace-file-contract/attempt-000`
>
> 目标：`handle.workspace-file.survival`
>
> 状态：accepted；完整 before/after/head recovery set；relation 为
> `uncommitted-original-residual -> aligned -> aligned`。

本文记录一次真实、由外部 generator 产生的 LangGraph StateFuzz 尝试。它用于
审计当前端到端路径，不是漏洞报告。所有运行产物位于 `runs/`，按仓库约定不提交
到 Git；本文中的路径是复查该次实验所需的 artifact 索引。

## 1. 结论

这次运行证明：对一个 ordinary workspace file，SyncFuzz 可以从真实 Agent 的
eBPF `handle/open` effect 出发，定位 effect 前后的 LangGraph durable checkpoint，
在三个独立 fresh recovery runtime 中保留同一个文件 identity，并稳定观察到：

```text
Q_before: agent logical state = absent,  file = present
Q_after:  agent logical state = present, file = present
Q_head:   agent logical state = present, file = present
```

因此 legacy classification 为 `residual`。relation projection 更精确地将
`Q_before` 写为 `uncommitted-original-residual`，将 `Q_after` 和 `Q_head`
写为 `aligned`。

这不证明 LangGraph 存在漏洞或违反某个 framework contract：本次
`causal_effect_evidence.status` 为 `unknown`，`contract.status` 为
`not-evaluated`，且文件 activity 没有被声明或测量。

按当前的 [Recovery-Hazard Fuzzing 设计](RECOVERY_HAZARD_FUZZING.md)，本次记录也
没有恢复期的 typed resolve/use `U'`：executor 只做 `lstat` identity probe，不观察
Agent 是否在恢复后读取、执行或以其他方式依赖该文件。因此它是一个静态
`W -> R(C,H)` relation baseline，不是 realized hazard、`REBOUND` finding 或安全结论。

## 2. 固定输入

| 项 | 值 | 作用 |
|---|---|---|
| StateObjective | `examples/objectives/workspace-file-survival.example.json` | 仅要求 `handle/open`、`survive-tool-return`、`fixed-path-workspace-file` 与 `across-checkpoint`。 |
| target / adapter | `langgraph-shell-react` / `langgraph` | 指定真实 Agent 和其 native checkpoint adapter。 |
| scaffold | `examples/synthesis/langgraph-shell-react-workspace-file-scaffold.example.json` | 只允许一个 `agent-result.txt` regular file；禁止 socket、FIFO、symlink、device 与后台服务。 |
| generator | `openai-compatible-generator-v2` | 外部 generator；不拥有 candidate ID、frontier 或 verdict。 |
| candidate | `synthesis-candidate:b114b65f1576fe85` | 任务为 `date > agent-result.txt`。见 `candidate.json`。 |
| image | `syncfuzz-langgraph:dev`，immutable ID `sha256:3dbc25d...7aff7252` | profile 和三个 recovery runtime 必须使用同一 image identity。 |
| model | `openai:deepseek-v4-flash` | 被 frozen fork plan 记录；credential 不写入 artifact。 |

候选自然语言任务是：

```text
Run a shell command that writes the current date and time to agent-result.txt.
Use 'date > agent-result.txt'.
```

scaffold 不允许 generator 在任务中提及 SyncFuzz、checkpoint、状态差异或期望
witness；effect 是否发生只由 profile evidence 判定。

## 3. 端到端模块

| 步骤 / 模块 | 输入 | 输出 | 本次审计点 |
|---|---|---|---|
| 1. Objective parser | objective JSON | canonical effect scope | `handle/open` 是唯一必须满足的 atom。 |
| 2. Scaffolded generator | objective、scaffold、attempt index、可选 bounded atom feedback | `candidate.json` | candidate ID 为 `b114...`；任务未包含 Oracle 或 recovery 指令。 |
| 3. LangGraph candidate executor | candidate、image、model、disk checkpointer、eBPF collection | `langgraph-candidate-execution.json`、target run、native checkpoint manifest | target run ID `1784978688113866852`；ProfileRun kind 为 `synthesis-candidate`。 |
| 4. eBPF normalizer + state probe | resource/process events、controller checkpoints | normalized effects、checkpoint summaries、checkpoint-effect map | 对 `/workspace/agent-result.txt` 记录两个 objective `handle/open` effect。 |
| 5. Candidate evaluator | objective、candidate、ProfileRun | `evaluation.json` | `eligible_for_retention=true`；frontier 是 `before-command..after-command`；没有 missing atom。 |
| 6. StateSeed promoter | matching candidate/profile/evaluation、head persistence evidence | `state-seed.json` | seed 绑定 profile、frontier 和 `after-observation` materialization head。 |
| 7. Native frontier binder | StateSeed、native manifest、effect time window | `langgraph-native-frontier-binding.json`、before/after coordinate | native checkpoint 严格包围 eBPF effect window；不以 controller checkpoint 名或历史顺序猜测。 |
| 8. Fork-plan preparer | seed、native binding、retained source lease、workspace topology、runner contract | `langgraph-fork-plan.json` | 冻结 source snapshot/checkpoint-store digest、file device/inode/mode、exact image ID 与三条 coordinate。 |
| 9. Recovery-set builder | frozen plan、head、retention policy | `historical-recovery-set.json` | 生成仅 checkpoint cut 不同的 `Q_before`、`Q_after`、`Q_head`。 |
| 10. Fork recovery executor | 每条 query 的 coordinate、cloned snapshot、source runtime lease | `recovery-set-execution.json` | 三个不同 fresh container exact-restore native state，只做 passive file identity probe。 |
| 11. Relation projector | seed、recovery execution、immutable causal metadata | `recovery-relation-report.json` | 写出 evidence、logical phase、identity/origin/multiplicity、relation 及 contract metadata。 |
| 12. StateFuzz admission | candidate/profile/seed/recovery lineage | `statefuzz-attempt.json` | status 为 `accepted`，因此可进入 batch 分母与 relation aggregation。 |

## 4. Profile 和 Frontier Evidence

controller checkpoints 为：

| Checkpoint | logical phase | monotonic time | 文件状态 |
|---|---:|---:|---|
| `before-command` | `P1` | `4581076560641098` | `agent-result.txt` 尚不存在。 |
| `after-command` | `P5` | `4581084641622000` | 文件存在。 |
| `after-observation` | `P6` | `4581085331470196` | 文件仍存在，是 materialization head。 |

validated frontier 是 `before-command..after-command`。其中与 objective 关联的
eBPF effect 时间窗口为 `4581080939639659..4581082181479330`；最终文件 identity
为 device `2049`、inode `51672841`、mode `0644`。

native binder 选择的 durable LangGraph checkpoint 为：

| 控制点 | native coordinate | 作用 |
|---|---|---|
| before | ID `1f1881b7-41f4-68a6-8001-6e4f8adc5080`；`message_count=1`，`next=[model]` | effect 尚未被 logical state 提交。 |
| after | ID `1f1881b7-5ef9-6bd8-8004-c350dfe1f6f4`；`message_count=4`，`next=[tools]` | effect 已进入 logical state。 |
| head | ID `1f1881b7-6bda-6cf7-8007-64870e41b7fb`；`message_count=6`，`next=[]` | initial materialization 完成后的 control。 |

controller checkpoint label 不会被误传给 LangGraph。plan 同时锁定 exact native
checkpoint ID 和 structural coordinate：本次 copied durable store 可按 exact native
ID restore；coordinate 保留其可审计的 logical position，并在 adapter 需要 resolution
时提供唯一匹配条件。

## 5. Frozen Recovery Contract

`langgraph-fork-plan.json` 固定以下条件：

- `recorded_plan_id=recorded-plan:target-run:1784978688113866852`；
- retention policy 为 `retain-relevant-os-state`；
- passive observation 为 `workspace-file-identity-v1:agent-result.txt`，mode 为
  `full`；
- workspace SHA-256 为 `eee9b0fd...0df68fd8`，checkpoint store SHA-256 为
  `5419850b...b0fbf5c4`；
- retained resource contract 只允许 `workspace-regular-file:agent-result.txt`；
- recovery 前必须验证 runner 的 durable-disk-checkpoint、exact-restore 与
  passive-workspace-file-observer capabilities。

executor 先验证 source lease 和 snapshot，再为每条 query 建立独立 recovery
container，restore 对应 native state。它不重新执行 candidate task，也不读取
`agent-result.txt` 内容；probe 只对 path 做 `lstat` identity 对比。

## 6. 三控制 Recovery 结果

| Query | controller / native cut | fresh runtime | Agent state | OS file state | classifier |
|---|---|---|---|---|---|
| `Q_before` | `before-command` / `...41f4-68a6...` | `...3033348711` | absent | present, residual, single | `residual` |
| `Q_after` | `after-command` / `...5ef9-6bd8...` | `...2746760119` | present | present, residual, single | `consistent` |
| `Q_head` | `after-observation` / `...6bda-6cf7...` | `...4079385049` | present | present, residual, single | `consistent` |

`Q_head=consistent` 是必要 control：它表明 observed residual 不是由 materialization
head 自身不一致造成的。`single` 表示 probe 已确认保存的是同一个文件 identity；它
不表示文件内容、业务语义或安全影响已经被检查。

## 7. Relation 与限制

relation report 将同一组结果规范化为：

| Query | logical phase | file identity | normalized relation |
|---|---|---|---|
| before | `effect-not-committed` | present, original, single | `uncommitted-original-residual` |
| after | `effect-committed` | present, original, single | `aligned` |
| head | `effect-committed` | present, original, single | `aligned` |

这里的 `original` 表示恢复时的 file identity 匹配 source head；`residual` 表示相对
于 before logical cut，该对象没有随 logical state 回退而消失。二者不矛盾。

本次明确不能宣称：

- effect 已被完整归因到某个 durable tool call：causal evidence 是 `unknown`；
- LangGraph 违反了某个 recovery contract：contract status 是 `not-evaluated`；
- 文件内容正确、文件仍被某服务使用、或存在安全影响：probe 不读取内容，activity 是
  `unknown`；
- 三次 repetition 代表所有 workspace file 或所有 Agent framework。

## 8. 与同批独立尝试的关系

这个 root 是 [`langgraph-workspace-file-contract`](../runs/langgraph-workspace-file-contract/)
batch 的 attempt 000。attempt 001 与 002 使用不同生成任务（写 working directory、
计算前十个素数），但均独立完成 profile、seed、recovery set。

`statefuzz-relation-batch-report.json` 的聚合结果是：3 attempts 全部 accepted、
3 条 complete three-control set、3 条 head-consistent、0 个 invalid relation
artifact，在同一 immutable image 下得到同一个 before/after/head relation vector。
三次 causal evidence 均为 `unknown`，contract status 均为 `not-evaluated`。

## 9. Artifact Audit Order

复查时按以下顺序读取即可：

1. `candidate.json`：自然语言任务及 generator provenance。
2. `langgraph-candidate-execution.json`：target run、runtime contract、profile entry。
3. `evaluation.json` 和 `state-seed.json`：objective atom、frontier、retention gate。
4. `langgraph-native-frontier-binding.json`：effect window 到 native checkpoint 的绑定。
5. `langgraph-fork-plan.json`：snapshot、resource、image 与 recovery contract。
6. `historical-recovery-set.json`：三条唯一变量为 checkpoint cut 的 query。
7. `recovery-set-execution.json`：每条 fresh runtime 的 raw A/O observation。
8. `recovery-relation-report.json`：normalized relation 与非 verdict metadata。
9. `../statefuzz-batch-report.json` 和 `../statefuzz-relation-batch-report.json`：跨 attempt 分母和归纳结果。
