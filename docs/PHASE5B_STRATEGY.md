# Phase 5B：Contract-Aware Validation

## 当前判断

SyncFuzz 已经不再停留在“框架能不能跑起来”的阶段。基于 `targets/langgraph_shell_react/`，我们已经稳定观测到几类真实 residue：

- persistent shell residue；
- workspace filesystem residue；
- orphan process residue。

这说明真实 Agent runtime 确实会把 shell、workspace 和 lifecycle state 带进 SyncFuzz 可观测的 artifact contract 里。

但下一阶段的核心问题已经变化：

> **residue 的存在，本身不自动等于漏洞。**

有些 residue 可能只是 runtime 的既定持久化语义；有些则可能是 replay / fork / discard / resume 的 lifecycle contract 被破坏；还有一些只有在后续 trusted execution 会消费它们时，才会转化成安全后果。

因此，Phase 5B 的目标不是把每个 residue 都直接叫成漏洞，而是把它们系统地区分开。

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

当前真实 target 结果建议按三类整理：

1. **Visible workspace residue**
   文件、目录、symlink、删除状态、workspace 内落盘产物。
2. **Latent execution-state residue**
   PATH、cwd、shell env、alias/function、后台进程、open fd、socket。
3. **External / authority residue**
   外部提交、副作用 receipt、single-use capability、授权状态。

这样分组的好处是：后续 fuzzing 可以沿状态面系统扩展，而不是一直围绕 PATH 一个点打转。

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

## 下一步

1. 为 LangGraph 写出第一份 `Recovery Contract Profile`。
2. 把 contract-aware classification 接进 target result / suite / corpus。
3. 保留一条小规模 activation 验证线，但不让它绑架框架主线。
4. 让真实 target 消费 Phase 4 的 matrix candidate，争取拿到至少一个非手写 discovery。
5. 接入第二个真实 target，优先 AutoGen。

## Phase 5B 退出标准

- LangGraph contract profile 已成文；
- target result 能区分 residue observation 与 contract interpretation；
- verify 失败具备稳定 taxonomy；
- 至少一个真实 target 可以消费 matrix/campaign；
- 至少形成一个非手写真实 target discovery，或一个可重复的强负结论；
- 第二个真实 target adapter 启动。
