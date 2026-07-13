# Phase 5B：Contract-Aware Validation

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

下一阶段的 mutation 要至少覆盖五类：

1. `primitive substitution`
   例如把 `prepend PATH` 替换为 `change cwd`、`modify umask`、`keep FD open`、`start Unix listener`、`spawn delayed process`。
2. `lifecycle splice`
   例如把 `fork -> discard` 改写为 `checkpoint -> replay`、`crash -> resume`、`result-lost -> retry`。
3. `phase shift`
   交换或插入 `checkpoint`、`activation`、`delay`、`observation`、`discard`、`runtime restart` 等步骤。
4. `activation substitution`
   不再把每个 residue 严格绑定到唯一的手写 activation，而是允许同一 residue 被不同 trusted action 消费。
5. `cross-seed crossover`
   从多个 seed 中提取 `plant / lifecycle / activation` 片段重新组合，争取产生研究者未直接编码的缺陷场景。

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
- `corpus analyze` 与 `corpus verify` 已具备 target-heavy corpus 的 outcome taxonomy。
- `target scenarios` 已提供第一版 executable Scenario IR view：seed、primitive、lifecycle operation、activation、oracle、mutation operator，以及可落到 replay / fork 运行参数的 execution plan。
- LangGraph lifecycle trace 已进入 oracle / compliance 路径，用于识别 fork-side relaunch、workspace reconstruction 和 task drift。
- `unix-listener-residue-fork` 已给出 active IPC endpoint 的有效 contract-violation 样例。
- `MAF-1` 当前已经不只覆盖 PATH / env / function / cwd / umask 这类 shell execution context residue，也已经补齐第一批 same-run workspace-object residue：`file-residue`、`directory-residue`、`delete-residue`、`symlink-residue`、`rename-residue`、`mode-residue`、`append-residue`、`hardlink-residue`、`fifo-residue`。这让 MAF 结果可以直接和 LangGraph 的 object-residue family 做并排比较，只是 lifecycle 语义目前仍然是 `run -> continue` 而不是 `checkpoint -> fork`。

## 下一步

1. 把当前 task-centric real-target candidates 重构为更可组合的 Scenario IR。
2. 让真实 target 消费 Phase 4 的 matrix candidate，并推进 primitive substitution / lifecycle splice。
3. 把 `unix-listener-residue-fork` 向 trusted-client activation 和 response poisoning 场景推进，并把它们纳入 active IPC seed family。
4. 保留一条小规模 activation 验证线，但不让 exploit generation 绑架框架主线。
5. 接入第二个真实 target，优先 MAF Workflow / GitHubCopilotAgent 路线，AutoGen 退到历史对照位。
6. 用 [REFACTOR_TESTING.md](REFACTOR_TESTING.md) 支撑即将进行的 `internal/syncfuzz/` 结构整理。

## 下一轮任务顺序

### Sprint 0：Scenario IR

- 把 real-target candidates 重构为可组合的 Scenario IR；
- 让 `plant / lifecycle / activation / oracle` 成为显式可变异部件；
- 为后续 minimization、crossover 和 discovery 结果归因提供统一表示。

这是当前最高优先级。没有 IR，后面增加的每个新 primitive 都仍然会先落成手写 testcase。

### Sprint 1：低成本补全

- `MAF` same-run workspace-object residue（已完成第一批）
- `umask-residue-fork`
- `cwd-residue-fork`

这一轮原本的目标是先把 shell execution context 和低成本 workspace residue 补齐。现在 MAF 这一侧的 same-run object family 已经到位，所以这部分后续只做零散修整，不再作为主攻方向。

### Sprint 2：突破文件快照边界

- `open-fd-residue-fork`（已落地）
- `deleted-open-fd-residue-fork`（已落地）
- `inherited-fd-branch-leakage`（已落地）
- `unix-listener-residue-fork`（已落地并形成有效阳性）

这一轮的目标是证明：

> 路径与文件系统状态即使已经“干净”，资源访问能力仍可能存活。

这会迫使 SyncFuzz 把 probe 从普通 workspace diff 扩展到 capability-aware state probe。

`open-fd-residue-fork`、`deleted-open-fd-residue-fork` 和 `inherited-fd-branch-leakage` 已经落地到真实 LangGraph target。Sprint 2 的第一版目标已经完成：SyncFuzz 现在不仅能看到 fd holder 是否跨 fork 存活，还能验证 successor branch 是否能通过 inherited fd 读回 discarded branch secret。`unix-listener-residue-fork` 已把这条线推进到 active IPC endpoint，并通过 lifecycle trace 修正了 fork-side relaunch 误判问题。

### Sprint 3：活跃 IPC

- `unix-listener-residue-fork`（已稳定，作为后续 IPC 场景基准）
- `discarded-server-trusted-client`
- `socket-response-poisoning`

这一轮关注 discarded branch 留下的不是静态对象，而是仍能被 trusted branch 消费的活动通信端点。

下一步不再继续围绕 `unix-listener-residue-fork` 本身调参，而是把它作为 active IPC 的 anchor seed，用于派生更现实的 trusted-client activation 和 response poisoning 场景。

### Sprint 4：MAF 第二目标

- `MAF-1`：官方 `GitHubCopilotAgent` shell sample
- `MAF-2`：官方 shell sample + session restore
- `MAF-3`：官方 shell-capable agent + 最小 Workflow + `CheckpointStorage`

`MAF-1` 现在已经完成第一轮 same-run residue 对齐，因此这一轮后续的目标不再是补更多普通 workspace object，而是引入新的恢复模型：

> `executor -> superstep -> checkpoint -> resume / rehydrate`

并围绕它测试 external effect replay、pending approval replay、same-instance resume 和 new-instance rehydrate 的状态分叉。

### Sprint 5：现实 Authority

- `ssh-agent-key-residue`
- `discarded-branch-authority-use`
- 本地 SSH server 激活 PoC

这一轮把 synthetic `authority-resurrection` 往真实 target 迁移，补上当前四层状态模型里最明显的现实缺口。

### Sprint 6：组合发现与 Minimization

- 为 campaign 增加组合发现模式，而不是只在预设矩阵上排序；
- 支持 primitive substitution、lifecycle splice、phase shift、activation substitution、cross-seed crossover；
- 实现自动 minimization，逐步删除 instruction、event、delay、primitive、activation step，只保留最小 PoC。

## 什么结果才算“通过 Fuzz 发现了新的缺陷”

项目内部后续应使用更严格的标准。至少同时满足：

1. 初始 seed 中没有直接编码最终缺陷场景；
2. testcase 由一个或多个 mutation operator 生成；
3. 产生了此前未见的 mismatch signature 或跨层状态转换；
4. feedback scheduler 自动选择并保留了它；
5. minimizer 能删除无关步骤并输出更小的 PoC；
6. 最终形成可复现的安全影响或明确的 contract violation。

在满足这些条件前，我们更准确的表述应是：

> SyncFuzz 已经能在真实 target 上稳定发现并排序 residue / contract-violation candidate，但尚未证明系统可以自动合成新的漏洞家族。

## Phase 5B 退出标准

- LangGraph contract profile 已成文；
- target result 能区分 residue observation 与 contract interpretation；
- verify 失败具备稳定 taxonomy；
- 重构测试清单能覆盖 CLI、synthetic、target、corpus verify 和 LangGraph active IPC gate；
- 至少一个真实 target 可以消费 matrix/campaign；
- 至少形成一个非手写真实 target discovery，或一个可重复的强负结论；
- 第二个真实 target adapter 启动，并完成 MAF-1 smoke path。

## MAF 定位

相对于 LangGraph，MAF 对 SyncFuzz 的主要价值不在于“再接一个 shell agent”，而在于引入一套不同的恢复与记账边界：

- LangGraph 主要暴露 `node / thread / checkpoint` 语义；
- MAF Workflow 主要暴露 `executor / message / superstep / checkpoint` 语义；
- checkpoint 发生在 superstep 同步屏障之后，因此更适合测试 `effect first, bookkeeping later`。

因此第二个真实 target 的默认路线调整为：

1. `MAF-1`
   官方 `GitHubCopilotAgent` shell sample，验证最小 shell-enabled 官方对象。
2. `MAF-2`
   官方 shell sample 加 session restore，验证 logical session 与 OS state 是否分叉。
3. `MAF-3`
   官方 shell-capable agent 进入最小 Workflow + `CheckpointStorage`，正式测试 superstep checkpoint、resume / rehydrate、pending request / response 与 external effect replay。

AutoGen 仍然有价值，但角色改为：

- 历史架构对照；
- command executor 风格 target；
- 与 LangGraph / MAF 做恢复语义比较。
