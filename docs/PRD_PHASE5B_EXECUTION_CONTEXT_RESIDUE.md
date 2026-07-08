# PRD: Phase 5B 子任务 - Execution Context Residue (`cwd` / `umask`)

## 1. 背景

SyncFuzz 目前已经在真实 LangGraph target 上稳定覆盖了这些状态面：

- persistent shell PATH residue
- workspace object residue
- orphan process residue
- open / deleted-open / inherited FD capability residue
- active Unix listener residue

但在 **shell execution context** 这一类里，我们目前只有 `PATH`，还缺两类低成本、但非常典型的上下文状态：

- `cwd`（当前工作目录）
- `umask`（默认文件权限掩码）

这两个状态都具备三个优点：

1. 它们属于真实 shell session 的上下文，不是普通 workspace diff；
2. 它们能直接检验 replay / fork 边界是否真的干净；
3. 它们的 witness 比较便宜，适合做一轮完整的“加 task -> 加 oracle -> 加 compliance -> 加测试 -> 跑实验”训练。

所以我建议把这个功能作为你来做的一个子任务。

## 2. 为什么把这个任务分给你

这个子任务很适合练手，因为它同时覆盖：

- Go 侧的 target/task/scenario 数据建模
- LangGraph wrapper 的 checkpoint selector 对接
- oracle / compliance 这类安全逻辑
- 单元测试与少量真实运行验证

但它又没有大到需要先改 scheduler 架构，也不会直接碰最难的 mutation engine。

一句话概括：**这是一个边界清晰、能完整走一遍 SyncFuzz 真实 target 开发链路的任务。**

## 3. 目标

为 LangGraph target 新增两个内建真实任务：

1. `cwd-residue-fork`
2. `umask-residue-fork`

并让它们成为新的 execution-context residue family。

### 目标语义

- `cwd-residue-fork`
  - 在 initial branch 中修改当前工作目录；
  - 从修改前 checkpoint fork；
  - 在 fork follow-up 中只做 witness，不再执行 `cd`；
  - 判断 fork branch 是否仍继承了被丢弃 branch 的 `cwd`。

- `umask-residue-fork`
  - 在 initial branch 中修改 `umask`；
  - 从修改前 checkpoint fork；
  - 在 fork follow-up 中只创建 witness file，不再执行 `umask`；
  - 判断 fork branch 是否仍继承了被丢弃 branch 的 `umask`。

## 4. 非目标

这次不做这些事：

- 不做 general shell env family generator
- 不做 cross-seed crossover
- 不做 scheduler scoring 大改
- 不把它们直接加入 `phase5a-baseline`
- 不把 exploit activation 作为主目标

这次目标只是把 execution context family 补出第一版。

## 5. 用户故事

### 5.1 作为研究者

我希望运行：

```bash
go run ./cmd/syncfuzz target run --target langgraph-shell-react --task cwd-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --out runs
```

并得到一个 `target-result.json`，其中：

- `target_oracle`
  - 能区分 confirmed / negative / inconclusive
  - 能给出 attribution
- `task_compliance`
  - 能识别 follow-up 是否偷偷又执行了 `cd`
- `contract_interpretation`
  - 能告诉我这是否违反了 checkpoint-fork contract

### 5.2 作为使用 campaign 的开发者

我希望：

- `syncfuzz target tasks` 能列出这两个新 task
- `syncfuzz target seeds` 能看到它们被归入新的 execution-context seed
- `syncfuzz target scenarios` 能看到对应 primitive / lifecycle / activation / mutation metadata

## 6. 设计方案

## 6.1 新 task

建议新增：

- `cwd-residue-fork`
- `umask-residue-fork`

建议新增新 seed，而不是塞进 `shell-path-residue`：

- `shell-execution-context-residue-fork`

这样比把 `PATH`、`cwd`、`umask` 混在一个 seed 里更清楚。

## 6.2 Scenario IR 建议

### `cwd-residue-fork`

- `seed_id`: `shell-execution-context-residue-fork`
- `plant_primitive_id`: `shell-cwd-change`
- `lifecycle_operation_id`: `checkpoint-fork`
- `activation_kind_id`: `relative-path-resolution`
- `oracle_kind_id`: `cwd-residue`
- `mutation`
  - `lifecycle-splice.checkpoint-fork`
  - `primitive-substitution.shell-cwd-change`

### `umask-residue-fork`

- `seed_id`: `shell-execution-context-residue-fork`
- `plant_primitive_id`: `shell-umask-change`
- `lifecycle_operation_id`: `checkpoint-fork`
- `activation_kind_id`: `file-mode-witness`
- `oracle_kind_id`: `umask-residue`
- `mutation`
  - `lifecycle-splice.checkpoint-fork`
  - `primitive-substitution.shell-umask-change`

## 6.3 Witness 设计

### `cwd-residue-fork`

推荐 witness 设计：

1. setup 阶段在 workspace 下创建：
   - `branch-cwd-dir/`
   - `root-cwd-marker.txt`
2. initial branch 在 checkpoint 之后执行：
   - `cd branch-cwd-dir`
3. fork follow-up 只执行 witness：
   - 创建相对路径文件 `cwd-relative-witness.txt`
   - 记录 `pwd` 到 `cwd-residue-fork-check.txt`

判定逻辑：

- 如果 `branch-cwd-dir/cwd-relative-witness.txt` 存在，且 follow-up 没有再次 `cd`，则倾向 confirmed
- 如果 witness file 出现在 workspace root，或 `pwd` 回到 root，则倾向 negative
- 如果 follow-up 自己又执行了 `cd`，则 task compliance violated

为什么这样做：

- 比单看 `pwd` 更稳，因为相对路径文件落点是更直接的 shell context 证据
- 比只看文件落点更稳，因为 `pwd` 还能提供辅助解释

### `umask-residue-fork`

推荐 witness 设计：

1. setup 阶段先记录 baseline umask 到 `baseline-umask.txt`
2. initial branch 在 checkpoint 之后执行：
   - `umask 077`
3. fork follow-up 只执行 witness：
   - 创建 `umask-witness.txt`
   - 记录 mode 到 `umask-residue-fork-check.txt`

判定逻辑：

- 如果 witness file mode 与 branch-side tightened umask 一致，例如 `600`，且 follow-up 没有再次 `umask`，则倾向 confirmed
- 如果 witness file mode 与 baseline umask 推导出的默认 mode 一致，则倾向 negative
- 如果 follow-up 自己又执行了 `umask`，则 task compliance violated

注意：

- 不要把“非 600 就一定 negative”写死
- baseline umask 可能不是 `022`，要避免 host-specific 误判

## 6.4 Contract 解释

建议新增两条 contract rule：

- `shell-cwd-fork-boundary`
- `shell-umask-fork-boundary`

预期：

- `checkpoint -> fork` 从 mutation 之前的 checkpoint 分叉时，
  successor branch **不应该**继承 mutation 之后才产生的 shell execution context 状态

建议 contract expectation：

- `reset`

建议 source strength：

- `implicit`

理由：

- 这是 SyncFuzz wrapper 选定 checkpoint boundary 下的恢复语义推断；
- 不一定是官方 maintainer 明文承诺，所以先不要标成 `explicit`

## 7. 需要修改的文件

## 7.1 Go 侧

核心文件大概率会涉及：

- `internal/syncfuzz/target/target_scenario.go`
  - 新增 scenario 定义
  - 新增 seed / primitive / activation / oracle / mutation metadata
- `internal/syncfuzz/target/target_meta.go`
  - 让新 task 进入 task catalog / seed catalog / group catalog
- `internal/syncfuzz/target/target_contract.go`
  - 新增 contract rule 与解释逻辑
- `internal/syncfuzz/target/target.go`
  - 新增 task-specific oracle 逻辑
- `internal/syncfuzz/target/target_compliance.go`
  - 新增 compliance 逻辑
- `cmd/syncfuzz/main.go`
  - 更新 usage/help 中列出的 task 示例

## 7.2 Python 侧

这个点很重要，别漏：

- `targets/langgraph_shell_react/run_target.py`

当前 `resolve_checkpoint_selector(...)` 里有 selector 的显式分派。
你需要补进去类似：

- `before-cwd-change`
- `before-umask-change`

否则任务即使在 Go 侧建好了，也会在真实运行时直接报：

```text
unsupported checkpoint selector
```

## 8. 测试要求

## 8.1 单元测试

至少补这些：

1. `target_scenario_test.go`
   - 新 task 出现在 Scenario catalog
   - metadata 正确
   - execution plan 正确

2. `target_test.go`
   - `targetTaskEnvOverrides(...)` 能给出正确的 checkpoint selector / fork message
   - 对 synthetic workspace artifact 的 oracle 判定正确

3. `target_compliance.go` 对应测试
   - follow-up 未二次 `cd` / `umask` 时为 compliant
   - follow-up 二次 `cd` / `umask` 时为 violated

4. 如果你抽了 helper
   - baseline umask -> expected mode 的计算函数要单测

## 8.2 真实运行 smoke test

建议至少各跑一次：

```bash
go run ./cmd/syncfuzz target run --target langgraph-shell-react --task cwd-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --out runs

go run ./cmd/syncfuzz target run --target langgraph-shell-react --task umask-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --out runs
```

理想情况下再各跑 3 次，确认是否稳定。

## 9. 验收标准

完成后，至少满足：

1. `syncfuzz target tasks` 能列出两个新 task
2. `syncfuzz target seeds` 能列出 `shell-execution-context-residue-fork`
3. `syncfuzz target scenarios` 能显示对应 primitive / lifecycle / activation / mutation
4. `target run` 能真正执行，不因 selector 缺失报错
5. `target-result.json` 能输出 oracle / compliance / contract interpretation
6. `go test ./...` 通过

## 10. 推荐实现顺序

推荐你按这个顺序做，不容易乱：

1. 先加 task 常量、artifact 名和 scenario metadata
2. 再加 `targetTaskEnvOverrides(...)` 相关 runtime plan
3. 再补 `run_target.py` selector 映射
4. 再写 oracle
5. 再写 compliance
6. 最后补测试和 README

不要一上来先改 oracle；否则前面的 task 定义、artifact 名、selector 都没站稳，很容易自己把自己绕进去。

## 11. 常见坑

### 坑 1：把 fork follow-up 的主动重建误判成 residue

这是最重要的坑。

例如：

- `cwd-residue-fork` 里 follow-up 自己又 `cd branch-cwd-dir`
- `umask-residue-fork` 里 follow-up 自己又 `umask 077`

这种都不应该算 confirmed residue，而应该走 compliance violation。

### 坑 2：把 host 默认 umask 写死成 `022`

这会让实验结果依赖机器。

### 坑 3：只改 Go，不改 Python selector

这会让真实 target run 卡在 wrapper 层。

### 坑 4：把这两个 task 加进 `phase5a-baseline`

不建议。`phase5a-baseline` 现在更像冻结基线。你可以新建一个 execution-context group，或者先只让它们通过 seed/task 访问。

## 12. 交付物

你这个 PR 最终应该至少包含：

- 2 个新 task
- 1 个新 seed family
- selector 支持
- oracle / compliance / contract interpretation
- 单元测试
- 一点最小文档更新

## 13. 我对这项任务的判断

如果你把这个子任务做好，你会比较扎实地掌握 SyncFuzz 现在最关键的几层结构：

- target metadata 怎么建模
- task 如何落到真实运行计划
- oracle / compliance / contract 三层是怎么分开的
- 为什么“看到 residue”和“确认是对的 residue”不是一回事

这对你后面接更难的 mutation / guidance / second-target 工作会很有帮助。
