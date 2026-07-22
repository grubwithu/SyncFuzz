# Phase 5B：Contract-Aware Validation

## FSE 路线重置：A–O recovery consistency

面向 FSE，SyncFuzz 的中心对象现在是 recovery consistency：
`S = <A, O>`，其中 `O = <N, Pi, H, E>` 分别表示 namespace、process /
descriptor、runtime or shell context、external effect。论文主线收敛为：

```text
typed lifecycle query
  -> resource footprint
  -> state-probe plan
  -> deterministic differential + root-cause evidence
```

因此，eBPF、feedback scheduling、structured mutation 和 LLM 都是可单独
评估的支撑机制，而不是要在同一篇论文中同时成立的核心 claim。LLM 以后
只能从源码或明确 contract 中提出 query/probe 候选；deterministic artifact、
compliance、oracle 与 replay/minimize 仍然承担判断责任。

当前已实现该主线的第一段：`syncfuzz target footprint` 从 Scenario IR 和
target-run artifact 导出 `resource-footprint.json`；`syncfuzz target
plan-probes` 再导出 `observation-plan.json`。它覆盖 artifact-visible 的
filesystem、process、FD、Unix socket、shell context，并为 socket ->
filesystem/process/FD、FD -> process 应用静态 dependency closure。该版本
只编译离线计划，固定四个 checkpoint（`before-plant`、`after-plant`、
`after-recovery`、`after-activation`），保留 full-probe fallback；它不是
eBPF collector，尚未声称 runtime causal trace。

这两个 artifact 还共享 `syncfuzz.lifecycle-query.v1`，显式固化
`q = <Init, Plant, Boundary, Recovery, Activation, Witness>`。各 stage 保存
Scenario IR 的 component identity、kind 与 summary；`violation_hypothesis`
只描述要由后续 differential oracle 回答的 recovery-consistency relation，
不是一个自动判定或 LLM judge。

target runner 现已通过 `--observation-plan` 消费该 schema：它把验证后的
plan 复制进新 run，并生成 `targeted-probe-report.json`，以 query-specific
path/process/FD selection 投影 broad snapshot。默认保持 shadow mode；可选的
`pruned-filesystem` mode 已把前/后/late 的 workspace snapshot 切到 exact
planned paths，再用一次最终 full snapshot 作为 fallback，并把未规划对象
写入 report。本地 `pruned` mode 还会先按 plan selector 匹配 process identity，
只为命中的 PID 读取 FD，再保留最终 broad process/filesystem fallback。generic
command adapter 现提供 opt-in `$SYNCFUZZ_LIFECYCLE_MARKER`：目标在实际 plant /
recovery / activation 完成后调用对应 marker，runner 在命令存活期间验证 JSONL
marker 并采集 P4/P6/P7 filesystem/process artifact，再 ack helper，保证目标不会
在 capture 前进入下一阶段；未调用 `after-plant` marker 时才把 P5 coverage 明确
标为 partial。
`target refine-plan` 已将 fallback evidence 接入 compiler：未规划路径可
扩展一次；socket 会自动补齐 filesystem/process/FD dependency，并把 added
paths 固化到 refined plan。接下来的直接目标是评估 plan-selected collection、
marker 覆盖率与 refine-once 的 fallback coverage；完成后才评估 time-boxed、
特权环境可用的 OS trace/eBPF evidence source。

target runner 还会为每次 run 生成 `target-checkpoint-differential.json`：以 P0
为 baseline，按 checkpoint 引用 marker/fallback artifact，并复用 filesystem
metadata delta 与 process lineage delta。它只提供可复核的 differential evidence；
`target compare` 现在可对 matching-query 的 control/target run 生成
`target-pair-differential.json`，以忽略 timestamp/PID 的方式给出 target-only
state 与 target/control difference candidate。当 target oracle confirmed 且 control
为 negative 时，v2 artifact 会先输出 `contract_calibration`：只有 control/target
都 task-compliant、解析到同一 contract profile/rule、target 为
`contract-violation` 且 control 为 `contract-consistent`，才会给出
checkpoint-bound root-cause hypothesis，明确标记为
`contract-calibrated-evidence-hypothesis` 并附带 profile/rule/source strength。
generic target、contract-unresolved、不兼容或 contract-consistent pair 只保留
descriptive evidence。下一步是在 controlled campaign 中量化这层 calibration 的
覆盖率与未解析原因，不能把任何汇总本身宣传为因果判定。

`target calibration-summary` 已将这类 campaign 的离线统计固化为
`target-pair-calibration-summary.json`：它递归收集每个 v2 pair artifact，给出
calibration coverage、unresolved reason、contract-rule 分布以及逐报告 provenance。
它刻意不自动声称 hypothesis precision；只有提供独立的
`syncfuzz.target-pair-root-cause-review.v1` candidate-level review manifest 时，
才按 `supported / (supported + unsupported)` 输出 reviewed precision，
`inconclusive` 不进入分母。

为了让 counterfactual control 本身可复现，`target pair-campaign` 读取
`syncfuzz.target-pair-campaign-manifest.v1`：每个 entry 先声明
`baseline`、`fresh-runtime`、`branch-cleanup`、`namespace-restore` 或 `custom`
control，再在同一 query identity 上运行 compare，并把 manifest、per-pair report、
campaign result 与 calibration summary 固化到一个 artifact directory。它只比较
预先记录的 run，不会隐式调用真实 target 或重新执行模型。

## 当前判断

SyncFuzz 已经不再停留在“框架能不能跑起来”的阶段。基于 `targets/langgraph_shell_react/`，我们已经稳定观测到几类真实 residue：

- persistent shell residue；
- workspace filesystem residue；
- orphan process residue；
- open / deleted-open / inherited FD capability residue；
- active Unix domain socket listener residue。

这说明真实 Agent runtime 确实会把 shell、workspace 和 lifecycle state 带进 SyncFuzz 可观测的 artifact contract 里。

其中 `unix-listener-residue-fork` 已经形成当前最强的一条 active IPC endpoint 实证：在 `before-unix-listener-launch` checkpoint 上 fork 后，successor branch 只执行 witness command，没有重复 listener launch，却仍能连接 discarded branch 留下的 `branch-listener.sock` 并收到 `SYNCFUZZ_UNIX_LISTENER_RESPONSE`。该结果在 `target-result.json` 中表现为：

```text
target_oracle.status = confirmed
target_oracle.attribution = runtime-preserved-residue
task_compliance.status = compliant
contract_interpretation.status = contract-violation
```

后续如果重构 `internal/syncfuzz/`，应使用 [REFACTOR_TESTING.md](REFACTOR_TESTING.md) 作为回归测试清单，特别关注 lifecycle trace 是否能阻止 fork-side relaunch 被误判成真实 residue。

但下一阶段的核心问题已经变化：

> **residue 的存在，本身不自动等于漏洞。**

有些 residue 可能只是 runtime 的既定持久化语义；有些则可能是 replay / fork / discard / resume 的 lifecycle contract 被破坏；还有一些只有在后续 trusted execution 会消费它们时，才会转化成安全后果。

因此，Phase 5B 的目标不是把每个 residue 都直接叫成漏洞，而是把它们系统地区分开。

同时还要再加一个更严格的判断：

> **当前系统已经具备 feedback-guided candidate exploration，但是否已经成为真正意义上的 Fuzzer，还取决于它能否自动合成研究者未直接编码的新场景。**

当前 real-target campaign 的强项在于执行、观测、回放、验证和归因；当前最大的短板则是 mutation 是否真的在创造新的执行语义，而不只是从人工准备的候选列表中挑选。

## 当前成熟度与风险

按论文推进而不是按纯工程堆料来判断，当前成熟度大致是：

| 维度 | 当前状态 |
| --- | --- |
| Target 执行与观测 | 成熟 |
| Differential Oracle | 较成熟 |
| Contract-aware attribution | 很强，是当前主要优势 |
| Corpus / replay / verify | 成熟 |
| Feedback-guided scheduling | 已成立，但效果尚需实验证明 |
| Scenario IR | 部分成立，尚未成为唯一事实来源 |
| 结构化 Mutation | 刚刚真正起步 |
| 自动 Minimization | 已可用，但仍偏表层 |
| 多 Target 外部有效性 | MAF 已起步，尚未完成 |
| 自动发现未知缺陷 | 尚未充分证明 |

如果按整体完成度粗略估算：

- 工程完成度约 `80%`；
- 方法完成度约 `65%`；
- 实验论证完成度约 `45% - 55%`；
- “Fuzzer 自动发现新东西”的证据刚进入关键阶段。

当前最大的风险不是基础设施不够，而是基础设施越来越丰富之后，系统继续变大，但核心科学证据没有同步增长。

## 当前最值钱的三项成果

1. `unix-listener-residue-fork` 已经越过普通文件残留，进入 **active IPC capability residue**。它证明 discarded branch 留下的不是静态 pathname，而是 successor branch 仍可通信的 live listener。
2. `MAF` 已经不是“未来工作”，而是第二条真实 target 线的起点。但它后续的验收线不再是“又补了多少 task”，而是能否消费同一套 portable scenario、进入同一套 campaign / replay / verify / minimize，并把差异归因到恢复模型。
3. `ExecutionPlan` 已经真正控制 runtime execution。candidate 现在不再只是 `task name + prompt variant + metadata`，而是已经开始变成可执行的 `seed + primitive + lifecycle + execution plan + activation + oracle`。这要求后续所有开发都围绕 Scenario IR 收敛。

## 三层判断

我们接下来把真实 target 结果分成三层：

1. **Residue Evidence**
   真实运行中观察到 state residue、state divergence 或 honest negative。
2. **Contract Interpretation**
   该 residue 是否违反 target 的恢复/分叉契约，还是只是显式或隐式允许的持久化语义。
3. **Activation Consequence**
   该 residue 是否会被后续 trusted execution 激活，转化成可利用后果。

SyncFuzz 框架本体优先负责前两层；第三层保留为少量高价值验证实验，而不是把 exploit generation 变成框架主目标。

## Recovery Contract Profile

每个真实 target 都需要一份 `Recovery Contract Profile`，用于明确：

- **state surfaces**
  - graph/thread state
  - workspace filesystem
  - shell session state（PATH、cwd、alias、function、env）
  - child processes / daemons
  - external effects
  - authority / capability state
- **lifecycle edges**
  - run -> continue
  - checkpoint -> replay
  - checkpoint -> fork
  - cancel -> resume
  - discard -> successor branch
  - process restart -> checkpoint restore
- **contract fields**
  - expected behavior: `preserve` / `reset` / `unspecified`
  - source strength: `explicit` / `implicit` / `unspecified`
  - evidence source: docs / code / maintainer statement / experiment

这样，未来的 oracle 就不只是“看见 residue”，而是“看见了一个相对于 contract 的 residue”。

## 结果分类

Phase 5B 的结果解释要逐步从 observation-first 升级到 contract-aware：

- `confirmed`: 确认观察到 residue 或已定义的强负结果；
- `negative`: 确认观察到 clean replay / clean fork / clean reset；
- `inconclusive`: 证据不足，无法区分 residue、task drift 或缺失观测。

在此基础上，再增加一层 contract interpretation：

- `contract-violation`
- `contract-consistent`
- `contract-unknown`

这层信息将来应该进入 `target-result.json`、suite summary 和 corpus metadata。

## Residue 分组

下一阶段不再优先按 Unix 对象类型罗列状态面，而是按 residue 保留下来的**安全能力**来组织：

1. **Storage capability**
   file、directory、symlink、hardlink、删除状态、mode、append、FIFO 等 workspace 对象与元数据。
2. **Execution context**
   PATH、cwd、shell env、alias/function、umask。
3. **Active execution**
   orphan process、process group、setsid、double-fork daemon。
4. **Resource access**
   open fd、deleted-but-open fd、继承的文件访问能力。
5. **Communication**
   FIFO、Unix socket、listener、message queue。
6. **Authority**
   single-use capability、SSH agent、credential helper、token cache。
7. **External effect**
   外部提交、副作用 receipt、幂等性与重复执行。
8. **Isolation topology**
   namespace、mount、cgroup、sandbox binding。

这样分组的意义是：论文与框架结果可以直接回答“discarded branch 留下了哪种能力”，而不是只回答“留下了哪种文件对象”。

## Scenario IR

下一阶段不应继续把每一种新 residue 都直接落成一个新的独立任务名。否则项目会逐渐退化成 testcase 仓库。

更合理的方向是把场景抽象成一段可组合的 Scenario IR，至少显式表达：

- `setup`
- `plant`
- `lifecycle`
- `activation`
- `fault`
- `oracle`

这样 mutation 才能作用于场景结构，而不只是作用于任务名或 prompt 文案。真正关键的不是再增加多少 `*-residue-fork`，而是让 Fuzzer 可以从较小的语义原语中自动合成此前不存在的新场景。

## 结构化 Mutation

完整 taxonomy 仍然可以覆盖五类 mutation，但接下来真正优先实现的是四个 semantic mutation family：

1. `primitive substitution`
   在保持 lifecycle 与 activation 基本不变的前提下替换状态原语。例如把 `prepend PATH` 换成 `change cwd`、`modify umask`、`keep FD open`、`start Unix listener`、`spawn delayed process`。它回答的是：**同一个 lifecycle edge 对哪些能力最危险？**
2. `lifecycle splice`
   把一个 seed 的 `plant`、另一个 seed 的 `lifecycle`、第三个 seed 的 `activation` 组合起来，形成研究者未直接写好的新场景。例如 `open FD plant + fork/discard lifecycle + future secret activation`。
3. `activation substitution`
   保持 plant 与 lifecycle 不变，替换 witness / activation。Unix listener 不应只停在 `ping`，还应扩成 `传递 secret`、`返回恶意配置`、`伪造 tool status`、`诱导后续命令` 等 trusted activation。
4. `fault-phase / phase-shift mutation`
   把 fault 或恢复边界系统性地移动到 `before dispatch`、`after process spawn`、`after first effect`、`before result delivery`、`before checkpoint persistence`、`after checkpoint but before acknowledgment` 等 phase。`phase-shift-single-process` 是第一条样例，但后续要把它升级成正式 mutation family。

`cross-seed crossover` 仍然是关键方向，但它应建立在上述四类 mutation 已经成形之后，再作为更强的组合发现机制推进。

## Guidance 升级

当前的 feedback 已经能很好地做候选排序，但下一阶段要把 guidance 从“候选新颖度”升级到“状态转换新颖度”：

1. **Cross-layer transition novelty**
   记录此前是否出现过某条跨层因果边，而不只是记录“本次涉及 open FD / socket / branch discard”。
2. **Causal phase novelty**
   区分 effect 发生在 checkpoint 前后、result 返回前后、discard 前后等不同 phase。
3. **Activation progress**
   按阶段跟踪 testcase 到达了哪里：
   - `P0` 原语未执行
   - `P1` 状态已植入
   - `P2` 状态跨生命周期边界存活
   - `P3` 后续 trusted action 接触残留
   - `P4` 产生安全 effect

Fuzzer 后续应优先保留进入更高 activation 阶段的 testcase，而不只是 `confirmed_count` 更高的候选。

## Feedback 有效性实验

当前 guidance 字段已经很多。下一步不应继续优先“加更多字段”，而是尽快回答一个更重要的问题：

> **feedback-guided campaign 是否真的比 random 或固定枚举更快到达高价值 activation？**

建议先在一个受控但非平凡的 IR 空间中做小型实验：

- `5` 个 primitive；
- `4` 个 lifecycle；
- `4` 个 activation；
- `3` 个 process mode；
- `3` 个 fault phase。

总空间约为：

```text
5 x 4 x 4 x 3 x 3 = 720
```

在相同预算下比较：

- `uniform random`
- `fixed enumeration`
- `feedback-guided`
- `full SyncFuzz`

每组例如给 `50` 次执行预算，重点比较：

- 到达 `plant` 的比例；
- 到达 `survive` 的比例；
- 到达 `activation` 的比例；
- 产生 `security consequence` 的数量；
- `time-to-first-impact`；
- `unique mismatch signature` 数量。

只有这组实验成立，当前 feedback / frontier / pivot / coverage_gain 才能从“复杂报表系统”升级成方法贡献。

实现上，target matrix / suite / campaign 已经先补齐对照实验所需的调度入口：`--selection-policy fixed` 保留矩阵枚举顺序，`--selection-policy random --random-seed N` 提供可复现 uniform-random baseline，`explore` / `feedback` 则继续使用当前 coverage-gap 与 feedback-ranked 路径。下一步可以在同一 candidate universe、相同 `candidate-limit` 和相同 prompt profile 集合下运行四组实验，而不再为 baseline 另写一次调度器。

## LLM-assisted Prompt Synthesis

当前 prompt 层仍然偏手写模板：例如 inherited-FD leakage 这类有效 finding 的核心 prompt 来自内嵌 seed 文本，再叠加 deterministic `lifecycle-boundary` / `mutation-focus` / `activation-focus` variant。它足以证明 Scenario IR、target execution、compliance、oracle 与 contract pipeline 能闭环，但还不能证明 SyncFuzz 可以系统性地产生更自然、更多样、跨 target 更鲁棒的任务表达。

下一步应加入一个受约束的 `prompt-mutator` / `prompt-repair` 层，但它只拥有 proposal authority：

```text
Scenario IR + previous failure taxonomy
-> LLM proposes prompt patch / prompt variant
-> static guardrail checks
-> target run
-> task compliance + oracle + contract interpretation
-> minimization / replay
```

LLM 不应直接替代 Scenario IR、oracle 或 contract profile。它的输入应当是固定的 `plant / lifecycle / activation / oracle` component、mutation focus、target adapter 风格，以及上一轮失败类别，例如 `task-noncompliant`、`execution-not-reached`、`activation-not-triggered` 或 `reconstruction-risk`。它的输出应当是结构化 prompt candidate：

- `prompt_variant_id`；
- prompt text 或 patch；
- declared intent，例如 `preserve lifecycle boundary`、`avoid relaunch`、`focus activation`；
- touched Scenario IR component IDs；
- guardrail notes，例如禁止重建 witness、禁止提前创建 activation artifact、禁止外部路径存储。

第一版实验不追求让 LLM 生成全新 Scenario IR，而是在已有 candidate universe 中，为同一个 Scenario IR 生成 `2 - 3` 个 prompt variant，与当前 deterministic `base / lifecycle-boundary / mutation-focus / activation-focus` 对照。核心指标是：

- activation reached 是否提升；
- task-noncompliant 是否下降；
- valid contract violation rate 是否保持或提升；
- minimizer 是否能把 LLM 文案缩回更小的 deterministic PoC；
- LLM variant 是否引入更多 reconstruction / prompt drift。

如果这组实验成立，LLM prompt synthesis 可以成为 V4 的主要增益点：它补足自然语言任务表达和 target-specific repair，但 SyncFuzz 的可信判定仍然由 deterministic artifacts、compliance、oracle、contract 和 replay/minimize 负责。

## Verify Failure Taxonomy

`corpus verify` 和 real-target replay 失败，后续不再只保留“failed / unconfirmed / error”这类粗粒度结果，而是细分为：

- `execution-not-reached`
- `task-noncompliant`
- `lifecycle-not-triggered`
- `state-not-planted`
- `residue-not-observed`
- `activation-not-triggered`
- `oracle-inconclusive`
- `clean-negative`

这样我们才能判断问题出在 prompt、adapter、runtime 语义、还是 oracle 设计本身。

## Minimization 校准

当前 minimizer 已能删除 prompt 行并收紧部分 `ExecutionPlan` 字段，但它还可能输出：

> 一个更短的自然语言任务，而不是一个更小的生命周期 PoC。

更有价值的缩减顺序应当是：

1. 删除无关 prompt instruction；
2. 删除 Scenario component；
3. 删除 lifecycle event；
4. 简化 primitive；
5. 简化 activation；
6. 缩小 fault timing 窗口；
7. 尽量把 LLM 行为替换为确定性 action；
8. 最后确认是否仍保持安全影响。

最终理想产物应尽量接近：

```text
plant
lifecycle
activate
observe
```

而不是一大段模型 prompt。

同时，minimization fidelity 已经从单一“全字段必须一致”升级成三档：

1. `Exact Fidelity`
   completion、oracle、attribution、signature、compliance、contract 全部一致。适合严格回归。
2. `Semantic Fidelity`
   保留 mismatch class、causal relation、security consequence、contract classification。允许具体路径、输出文本或 attribution 子标签变化。适合论文 PoC。
3. `Impact Fidelity`
   只要求仍能产生同一安全影响。适合探索更小但语义更自由的证明样例。

## 已完成的 Phase 5B 基础设施

- LangGraph target 已有 conservative contract profile，并能在 `target-result.json` 中输出 `contract_interpretation`。
- real-target suite / matrix / campaign 已能聚合 attribution、compliance 和 contract status。
- real-target suite / matrix / campaign 现在也会聚合 `outcome_summaries` 与 `activation_summaries`，把“命令执行了”“activation 到了”“residue 确认了”这几层分开统计。
- real-target matrix / campaign 现在还会输出 `dimension_coverage`，按 `scenario / seed / task / primitive / lifecycle / activation / oracle / mutation` 统计本轮真正覆盖到的维度值，以及还未触达的空洞。
- 当 matrix / campaign 被 `candidate_limit` 截断时，这份 `dimension_coverage` 现在也会继续以完整候选宇宙为基准，而不是只盯住这一小批真正执行到的 candidate。
- feedback scheduler 现在会直接消费这些 coverage 空洞，在下一轮优先选择能够补齐缺失维度的 candidate，而不是只按已有 summary 分数重复局部热点。
- 同一份结果里还会产出 `frontier_candidates`，直接列出当前宇宙中下一批最值得继续执行的未跑 candidate，方便人工分析和后续自动调度共用同一份 guidance。
- `target-campaign-result.json` 的每一轮现在还会写出 `coverage_gain`，明确指出这一轮相对前几轮到底新补到了哪些维度值。
- 每一轮还会把这些增益压缩成 `coverage_gain_stats`，其中 `weighted_score` 会同时考虑维度权重和进度层级，方便后续直接用于 budget / round ranking。
- target campaign 现在已经支持一个保守的 early-stop 机制：当 `weighted_score` 连续若干轮低于阈值时，可以提前结束，避免在已耗尽的局部热点上空转。
- 当 campaign 提前停止时，结果里还会给出 `pivot_recommendations`，优先提示尚未覆盖到的 seed family、prompt profile 和其他 built-in 维度；如果没有可推荐项，则用 `catalog_exhausted` 明确说明当前内置宇宙基本已经跑满。
- 在显式开启 `auto_pivot` 后，同一个 stagnation 信号也可以不再直接停机，而是沿着推荐维度自动扩展当前 campaign，并把过程记录到 `pivot_history`。当前实现已经收紧到 conservative single-step pivot：每次只扩一个值，并用 frontier gap / novelty 去解释为什么先走这一步。
- target matrix suite / campaign 已经补齐 `fixed` 与 deterministic `random` candidate selection policy，并把 `selection_policy` / `random_seed` 写入 suite、matrix result 和 campaign result；这为 `random / fixed enumeration / feedback-guided / full SyncFuzz` 四组小预算对照实验提供了同一套候选宇宙与 artifact schema。
- `corpus analyze` 与 `corpus verify` 已具备 target-heavy corpus 的 outcome taxonomy。
- `target scenarios` 已提供第一版 executable Scenario IR view：seed、primitive、lifecycle operation、activation、oracle、mutation operator，以及可落到 replay / fork 运行参数的 execution plan。
- Scenario IR 已冻结第一版 schema：`syncfuzz.target-scenario.v1`。每个 component 现在都有稳定的 `component_id`、`role` 与 `kind_id`，role 集合覆盖 `setup / plant / lifecycle / activation / fault / oracle`；built-in scenario 在进入 matrix、执行与 artifact 前会统一 normalization 和 validation，并自动补齐由 primitive、lifecycle、activation、oracle 元数据要求的结构组件。
- real-target matrix candidate 的 `execution_plan` 已经从只读 metadata 接入真实执行路径：candidate 可以控制 replay / fork、checkpoint selector、checkpoint backend、process mode 和 fork follow-up，并把实际采用的 plan 固化进 `target-task.json`。candidate 自带的 late-observation timing 也会被 suite 消费，corpus replay / verify 也会恢复该 plan，避免 semantic mutation 在复验时退回 built-in 默认值。
- matrix / suite 现在会把 candidate 的完整 Scenario IR 直接交给 `target run`，而不是在执行时仅按 `task_id` 重新拼回 built-in metadata。生成 candidate 的 scenario identity、seed、components、mutation provenance 和 execution plan 会一起固化进 `target-task.json`，并被 corpus replay 与 minimization trial 原样恢复。这补上了“generator 产生了新 IR，但 artifact 又退回 task 默认值”的断点，为继续扩展 primitive / activation substitution 与实现 IR component reduction 提供稳定事实来源。
- 第一条真正改变执行语义的 generator 已落地：所有 split-process checkpoint candidate 会自动派生 `phase-shift-single-process` sibling，在保留 task / activation / oracle 的同时，把 initial 与 resumed phase 从跨进程改成同进程执行。该 candidate 会携带 `phase-shift.process-mode.single-process` mutation provenance，并进入正常 feedback / coverage / campaign 路径。
- 第一组可执行 `primitive substitution` 已进入 matrix，并开始分成 same-run / replay / fork 三层：PATH same-run seed 现在会自动派生 `persistent-shell-poisoning/primitive-shell-env-export`、`persistent-shell-poisoning/primitive-shell-function-define`、`persistent-shell-poisoning/primitive-shell-cwd-change` 与 `persistent-shell-poisoning/primitive-shell-umask-set`，保留 `run -> continue` lifecycle，并通过通用 `env-residue` / `function-residue` / `cwd-residue` / `umask-residue` oracle 与 compliance dispatch 同时覆盖 LangGraph 和 MAF；LangGraph 还会为这些 generated same-run scenario 分别绑定 `shell-env-generated-within-run`、`shell-function-generated-within-run`、`shell-cwd-generated-within-run` 与 `shell-umask-generated-within-run` contract rule，而不是退回 PATH baseline 解释。PATH replay seed 现在也会自动派生 `persistent-shell-poisoning-replay/primitive-shell-env-export` 与 `persistent-shell-poisoning-replay/primitive-shell-function-define`，保留 `checkpoint -> replay` lifecycle，并分别生成 `before-env-export` / `before-function-define` selector与 replay-safe prompt；它们的 oracle / compliance / contract 会显式区分 direct replay residue、replay-side reexecution 与 final-call reconstruction。PATH fork seed 则继续自动派生 `primitive-shell-env-export` 与 `primitive-shell-function-define`，保留 `checkpoint -> fork` lifecycle，同时分别生成 `before-env-export` / `before-function-define` selector、初始 branch prompt 与 fork activation message。每个 candidate 的 oracle、compliance、contract rule 和 mismatch signature 都由生成后的 Scenario IR 决定，不会退回 PATH task 的默认解释。这已经形成一个小型 compatibility-aware family，但尚不能表述为任意 primitive cross-product。
- 第一组可执行 `activation substitution` 已进入 matrix，并开始分成 same-run、post-return 与 fork 三层：`unix-listener-residue` seed 现在会派生 `unix-listener-residue/activation-trusted-action`，保留 `run -> continue` lifecycle，但把被动 socket reachability 替换为 later shell call 中的固定 trusted policy；同一 generated Scenario IR 已可同时在 LangGraph 和 MAF 上执行。`orphan-process-long-delay` 现在会派生 `activation-trusted-action`，保留 `target-command -> post-return` lifecycle，但把 passive `late-effect` 换成 late-only fixed trusted-action artifacts，用于判断残留后台进程是否能操纵 future trusted state。`unix-listener-residue-fork` seed 仍会派生 `activation-trusted-action`，保留初始 branch 的 Unix listener plant 与 `checkpoint -> fork` lifecycle，但把被动 socket reachability 替换为 successor branch 中的固定 trusted policy。`open-fd-residue-fork`、`deleted-open-fd-residue-fork` 与 `inherited-fd-branch-leakage` seed 现在也会派生同名后缀的 trusted-action candidate，保留 discarded-branch fd holder 与 fork lifecycle，但把被动 fd observation 替换为固定 trusted policy 与本地 consequence artifact；process 与这三个 FD candidate 现在都显式携带 `cross-seed-crossover` mutation provenance，表示 active-execution / capability-residue plant 与 active-IPC trusted-action activation/oracle pattern 已经形成一个小型受控组合 family。它们的 follow-up 都只依据固定 policy 决定是否写入本地 marker，不执行 response text、recovered marker 或 recovered secret；oracle / compliance 会联合检查 artifact 与 trace，并把 listener relaunch、fd-holder relaunch、process-command drift 或其他 reconstruction 归类为 reconstruction。
- 第一条生成式 `lifecycle splice` 也已进入 matrix：`unix-listener-residue-fork` seed 现在会派生 `lifecycle-splice-checkpoint-replay` candidate，保留 Unix listener plant，但把 lifecycle 从 `checkpoint -> fork` 改成 `checkpoint -> replay`。prompt 本身是 replay-safe 的：如果 replay 时 `branch-listener.sock` 与 `branch-listener-pid.txt` 已经存在，就只做观察；不存在时才允许 replay 侧合法 relaunch。这样 oracle / compliance / contract 可以把 direct runtime residue、legitimate reexecution 与 clean replay 分开，而不是把所有 replay positive 都混成同一种结果。
- matrix 会从 Scenario IR mutation metadata 派生 `lifecycle-boundary`、`mutation-focus` 和 `activation-focus` prompt candidate；其中 activation variant 只约束初始 branch 保留后续 trusted activation 所需的状态，不会要求 setup 阶段提前执行 activation。
- feedback guidance 已从二值 activation reached 扩展为 execution、task compliance、lifecycle、plant、activation、reached 多阶段进度；candidate ranking、coverage gain 和 prompt repair 都会消费该进度。frontier 还能把修复意图标记为 `lifecycle-repair`、`state-plant-repair` 或 `activation-repair`。
- `target suite` / `target matrix` 的每个 confirmed result 现在会写出 `minimization_plan`，把 Scenario IR component、mutation axis、expected artifact 和 oracle 约束转换成可执行前的 delta-debugging 清单。component step 已携带稳定 `component_id` 与 `component_kind_id`，mutation step 也会携带稳定 `mutation_id`。reduction runner 现在已经能按 IR identity 尝试删除非必需 `setup` / `fault` component、清除可缩减的 component summary metadata、删除可缩减的 mutation provenance，并能在 `Semantic / Impact Fidelity` 下清除可缩减的 `plant` metadata；在 `Impact Fidelity` 下也能清除可缩减的 `lifecycle` metadata component 与 `LifecycleEdge` / `LifecycleOperationID`，但会保留实际 fork / replay selector、backend、process mode 和 activation text；同一 impact 模式还可以清除可缩减的 `activation` metadata component 与 `ActivationKindID`，以及可缩减的 `oracle` metadata component 与 `OracleKindID`，前提是 oracle status 与 impact / oracle identity 仍保持。完整 lifecycle command rewriting 仍等待后续语义化 reducer。
- `target minimize --from ...` 已经可以从 suite / matrix 结果中抽取 `target-minimization-plan.json` batch，为下一步自动 reduction runner 提供稳定输入。
- `target minimize --execute` 已接上自动 reduction runner：它从 source run 的 `target-task.json` 恢复真实命令，在新 workspace 中先做有上限的 prompt line deletion 与 concrete command line deletion，再尝试保守的 optional Scenario IR component deletion、component summary deletion、mutation provenance deletion、semantic plant metadata reduction、impact lifecycle / activation / oracle metadata reduction、fork activation message line reduction，最后逐项尝试清除 process mode、checkpoint backend、checkpoint selector、fork follow-up 与 replay。默认 `Exact Fidelity` 要求 completion、oracle status、attribution、完整 mismatch signature、task compliance 与 contract interpretation 全部保持；显式 `Semantic Fidelity` 允许 attribution / primitive-operation drift，但保持 lifecycle / phase / state-class / relation / impact、compliance 与 contract status；`Impact Fidelity` 只保留 oracle status 与同一 security impact，并允许 activation metadata-only drop 在 oracle identity 不变时把 impact metadata 清空。结果写入 `syncfuzz.target-minimization-result.v1`，同时保存 fidelity、original/minimized prompt / command line count、component / mutation count、execution plan 与 accepted step。当前尚未执行 semantic command rewrite、non-fork activation-command、完整 lifecycle command rewriting、cross-seed reduction。
- LangGraph lifecycle trace 已进入 oracle / compliance 路径，用于识别 fork-side relaunch、workspace reconstruction 和 task drift。
- `unix-listener-residue-fork` 已给出 active IPC endpoint 的有效 contract-violation 样例。
- `MAF-1` 当前已经不只覆盖 PATH / env / function / cwd / umask 这类 shell execution context residue，也已经补齐第一批 same-run workspace-object residue：`file-residue`、`directory-residue`、`delete-residue`、`symlink-residue`、`rename-residue`、`mode-residue`、`append-residue`、`hardlink-residue`、`fifo-residue`。这让 MAF 结果可以直接和 LangGraph 的 object-residue family 做并排比较，只是 lifecycle 语义目前仍然是 `run -> continue` 而不是 `checkpoint -> fork`。
- `MAF-2` 已经启动第一条轻量 session restore 线：`maf-session-continuity` 会在一个 MAF turn 写入 workspace marker，然后把 `AgentSession` 序列化并恢复到新构造的 runtime object，再由 restored turn 写出 continuity witness。它不是 Workflow checkpoint，但已经能把 `same logical session, different runtime object` 变成 suite / matrix / campaign 可消费的 candidate。
- `MAF-3` 已经有第一批最小 Workflow checkpoint smoke path：`maf-workflow-checkpoint-continuity` 使用官方 `WorkflowBuilder` / `Executor` / `FileCheckpointStorage`，在 checkpoint 后重建 workflow object 并从 checkpoint restore；`maf-workflow-external-effect-replay` 把 post-restore activation 换成 non-idempotent ledger append；`maf-workflow-http-effect-replay` 把同类 effect 推进到 workflow 外的 HTTP service boundary，默认自带 in-process fallback，也可通过 `MAF_WORKFLOW_EFFECT_SERVICE_URL` 调用独立 mock service process；`maf-workflow-resource-replay` 则把 service commit 扩展为 external resource creation；`maf-workflow-authority-token-replay` 把服务端 authority token 的 issue / consume / replay conflict 纳入同一条恢复语义；`maf-workflow-partial-commit-replay` 则在下游失败后验证 partially committed external effect 是否会被 restore 后再次执行；`maf-workflow-approval-pending-replay` 使用官方 functional Workflow 的 `request_info` / `responses` 路径，把 pending approval restore 后的 response replay 纳入同一个 oracle / compliance 体系；`maf-workflow-rehydrate-divergence` 则直接对照 same-instance resume 与 recreated workflow rehydrate 的 effect 差异。后续继续把这些 authority/token 场景推进到更真实的 SSH / OAuth / CI token 边界。

## 下一步

1. 让 Scenario IR 成为 testcase 的主事实来源，而不是继续扩大手写 task catalog。
2. 继续扩展 compatibility-aware `primitive substitution` 与 `activation substitution` family，并实现 `lifecycle splice`、`fault-phase / phase-shift mutation`。
3. 把当前 `MAF` 已经消费到的 same-run portable scenario 继续扩展到更高价值 family，并进入相同的 campaign / replay / verify / minimize。
4. 把 minimizer 从 prompt / execution-plan reduction 扩展到 Scenario IR component reduction，并增加 `Semantic Fidelity` 模式。
5. 用已落地的 `selection-policy` 跑 feedback-guided campaign 小预算对照实验，证明它比 random / fixed enumeration 更快到达高价值 activation。
6. 加入受 Scenario IR 约束的 LLM `prompt-mutator` / `prompt-repair` 层，让 LLM 生成候选 prompt variant，但由 static guardrail、task compliance、oracle、contract、replay 与 minimization 筛选。
7. 在已落地的 Unix-listener、process、open-FD、deleted-open-FD 与 inherited-FD trusted-action family 上继续扩展更强 activation consequence，并把当前 cross-seed crossover 推广成更一般的 lifecycle / activation / oracle 组合机制，而不是继续围绕更多普通 workspace object 扩面。

## 接下来两周

### 第一周

- 冻结 Scenario IR schema；
- 把 `3 - 5` 个现有 LangGraph testcase 从 task-centric 表达迁移到纯 IR；
- 把当前 `PATH -> env/function` fork substitutions 扩展到更多兼容 primitive pair；
- 把当前 Unix-listener trusted-action substitution 扩展到更多 activation / consequence pair；
- 把当前 same-run portable IR 继续扩展到更高价值的 replay / fork / trusted-activation family。

### 第二周

- 把当前 `cross-seed-crossover` 从三个 FD trusted-action 个例扩展成可枚举的 crossover family；
- 把 minimizer 扩展到 IR component reduction；
- 使用 `--selection-policy fixed|random|feedback` 运行前三组小预算实验，并把 full SyncFuzz 设为开启 `auto_pivot` 的第四组；
- 实现第一版受约束 LLM prompt synthesis 实验：对固定 Scenario IR 生成少量 prompt variant，并与 deterministic prompt variant 比较 activation、compliance 和 valid violation；
- 继续把已自动合成的 `Unix socket / FD / process` trusted-activation 场景扩展到更多 consequence 与生命周期边界。

这两周内不再把“接更多 target”或“继续补十几个 workspace object 类型”作为主任务。

## 什么结果才算“通过 Fuzz 发现了新的缺陷”

项目内部后续应使用更严格的标准。至少同时满足：

1. 初始 seed 中没有直接编码最终缺陷场景；
2. testcase 由一个或多个 mutation operator 生成；
3. 产生了此前未见的 mismatch signature 或跨层状态转换；
4. feedback scheduler 自动选择并保留了它；
5. minimizer 能删除无关步骤并输出更小的 PoC；
6. 最终形成可复现的 trusted activation 后果；如果暂时只有 contract violation，则应记为高价值 candidate，而不是直接宣称为“自动发现的新缺陷”。

在满足这些条件前，我们更准确的表述应是：

> SyncFuzz 已经能在真实 target 上稳定发现并排序 residue / contract-violation candidate，但尚未证明系统可以自动合成新的漏洞家族。

## Phase 5B 退出标准

1. `Scenario IR` 成为 testcase 的主事实来源；
2. 至少形成四类 semantic mutation：`primitive substitution`、`activation substitution`、`lifecycle splice`、`fault-phase / phase-shift mutation`；
3. 至少支持一次 `cross-seed crossover`；当前已有 process、open-FD、deleted-open-FD 与 inherited-FD trusted-action crossover provenance，后续需要扩展到更系统的可枚举 family；
4. minimizer 可以删除 `IR component`，而不只删除 prompt 行或 `ExecutionPlan` 字段；
5. `LangGraph` 和 `MAF` 都能消费同一 portable scenario；
6. 两个 target 都能进入 `campaign / replay / verify / minimize`；
7. 至少一个 finding 满足：初始 seed 没有直接编码最终场景、由 mutation 或 crossover 自动生成、被 feedback 保留、经 oracle 确认、被 minimizer 缩减，并产生 trusted activation 后果。

## MAF 定位

相对于 LangGraph，MAF 对 SyncFuzz 的主要价值不在于“再复制一套 file / directory / symlink / hardlink / FIFO residue”，而在于它暴露了不同的恢复模型：

- LangGraph 主要暴露 `node / thread / checkpoint` 语义；
- MAF Workflow 主要暴露 `executor / message / superstep / checkpoint` 语义；
- checkpoint 发生在 superstep 同步屏障之后，因此更适合测试 `effect first, bookkeeping later`。

MAF 后续应优先集中在它独特的四类机制：

1. `Superstep Partial Commit`
   `Executor A effect committed -> Executor B fails -> checkpoint not formed -> resume / rehydrate -> A executes again`
2. `Same-instance Resume vs New-instance Rehydrate`
   系统性比较 workflow identity、executor identity、process / workspace identity、pending request、external ledger、authority state 是否分叉。
3. `Pending Approval Replay`
   测试 approval response 在 checkpoint 边界前后是否被重复消费、错误绑定或恢复出旧状态。
4. `Workflow State vs Provider Session State`
   如果 provider 自身也维护 session，就要显式比较 workflow checkpoint、provider session、OS / external world 之间的版本错位。

因此第二个真实 target 的默认路线仍然分成：

1. `MAF-1`
   官方 `GitHubCopilotAgent` shell sample，验证最小 shell-enabled 官方对象。
2. `MAF-2`
   官方 shell sample加 session restore，验证 logical session 与 OS state 是否分叉。当前第一步是 `maf-session-continuity`。
3. `MAF-3`
   最小 Workflow 加 `CheckpointStorage`，重点测试 superstep checkpoint、resume / rehydrate、pending request / response 与 external effect replay。现有 `maf-workflow-*` family 应优先围绕这几条恢复语义继续加深，而不是追求对象类型对齐。

AutoGen 仍然有价值，但角色改为：

- 历史架构对照；
- command-executor 风格 target；
- 与 LangGraph / MAF 做恢复语义比较。
