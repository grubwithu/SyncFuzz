# SyncFuzz 分阶段路线

## Phase 0：项目创立与问题收束

目标：把方向从泛泛的 Agent Security 收束到主动漏洞挖掘。

产物：

- 项目定位：跨层状态失同步漏洞挖掘。
- 技术边界：Shell-enabled Agent，暂不做 GUI CUA、通用 Agent Transaction、完整 prompt fuzzing。
- 状态模型：`S = (A, O, X, C)`，即 Agent、OS、External、Authority 四层状态。
- MVP 骨架：Go CLI、TypeScript mock server、文档路线。

## Phase 1：Known-Answer MVP

目标：不用 LLM，先跑通可复现的状态失同步样例。

必须实现：

- deterministic minimal harness；
- filesystem snapshot；
- structured trace JSONL；
- mismatch signature；
- `orphan-process` seed；
- `action-replay` seed；
- `authority-resurrection` seed；
- `persistent-shell-poisoning` seed；
- `partial-filesystem-rollback` seed；
- `branch-leakage` seed；
- EffectServer 与 AuthorityServer mock；
- suite runner、`suite-result.json` 汇总、`interesting.json` novelty 记录、`corpus/` 登记、corpus replay 与 corpus verify；
- `local` / `container` environment backend 与统一 `--env` 参数通道；
- `orphan-process`、`persistent-shell-poisoning`、`branch-leakage` 的 local/container process snapshot artifacts 与 `process-lineage.json` 摘要；

完成标准：

> 能自动复现 known-answer mismatch，导出最小 PoC artifacts，批量运行所有 seed 生成 suite-level summary、novelty discoveries 和 corpus entries，并能把 corpus 作为持续回归验证集批量检查复现性。

## Phase 2：Cross-Layer Tracing

目标：把单纯文件快照扩展为跨层状态观测。

状态：已完成第一版，可进入 Phase 3。

已实现：

- container-aware process lineage 扩展到所有 workspace-backed seeds，并沉淀为可回归验证的 artifact contract；
- persistent shell probe；
- filesystem metadata probe；
- mock external state probe；
- mock authority state probe；
- lifecycle event alignment。
- 每个 run 输出 `agent-state.json` 与 `state-trace.json`，统一索引 Agent、OS、External、Authority 四层 artifact。

第一版可使用 Go + `/proc` + shell probe。后续再引入 BCC/bpftrace 或 libbpf CO-RE。

完成标准：

> 每个 run 能生成 Agent、OS、External、Authority 的统一 trace 与 snapshot。

审查记录见 [PHASE2_REVIEW.md](PHASE2_REVIEW.md)。结论：方向正确，已修正 workspace-backed seed 在 container backend 下的执行语义一致性问题。

## Phase 3：Fault Scheduler 与 Differential Oracle

目标：系统性注入 lifecycle fault，并自动判断 mismatch 是否有安全意义。

状态：已启动。第一步已实现 deterministic fault-plan catalog 和 `fault-plan.json` artifact，先把 known-answer seed 的隐含 fault phase 变成 scheduler 可消费的结构化输入。

Fault phase：

- P0 before tool intent
- P1 after intent before dispatch
- P2 after shell receives command
- P3 after child process created
- P4 after first OS/external effect
- P5 after command finishes before result delivery
- P6 after result delivery before checkpoint persistence
- P7 after checkpoint persistence before acknowledgment
- P8 during replay/resume

Oracle：

- Rollback Residue
- Forgotten External Effect
- Authority Resurrection
- Branch Leakage
- Cancel/Resume Split-Brain

完成标准：

> 不依赖人工判断，能从 control run 与 fault run 生成结构化 mismatch signature。

下一步：

- 定义 `control` / `fault` run pair；
- 从 `state-trace.json` 自动读取 observation coverage；
- 生成 pair-level differential report；
- 再引入随机或 feedback-guided fault timing。

## Phase 4：Feedback-Guided Fuzzing

目标：从固定 seed 进入状态原语组合搜索。

第一批 mutation 原语：

- background process
- double-fork daemon
- delayed write
- PATH/cwd modification
- shell alias/function
- untracked file
- symlink
- chmod/xattr
- open FD
- Unix socket
- external API commit
- single-use capability
- concurrent file replacement

Feedback：

- 新 mismatch signature；
- 新状态层组合；
- 更接近 privileged effect；
- 更高复现率；
- 更小 PoC。

完成标准：

> 相比 random fault timing，更快发现 known-answer case，并开始发现未知 mismatch。

## Phase 5：真实 Target Adapter

目标：从最小 harness 迁移到真实 Agent runtime。

顺序：

1. LangGraph + persistent shell；
2. AutoGen command executor；
3. OpenHands runtime/sandbox；
4. Crab、Shepherd、Cordon、DeltaBox 等研究原型。

每个 adapter 至少统一：

- run/reset；
- checkpoint/replay；
- cancel/resume；
- fork/discard；
- lifecycle events；
- workspace/sandbox binding。

完成标准：

> 同一个 seed 能在至少两个 runtime 上运行，并比较状态语义差异。

## Phase 6：漏洞确认与论文评估

目标：从测试框架变成漏洞挖掘工作。

实验：

- baseline：random timing、tool-boundary-only、syscall novelty、manual replay；
- metrics：unique vulnerabilities、time-to-first-vuln、false positive rate、reproducibility、PoC size、overhead；
- ablation：去掉 scheduler、去掉 state probe、去掉 feedback；
- real report：向维护者提交可复现 PoC。

完成标准：

> 至少形成若干真实系统的确认漏洞、确认风险语义或可维护者复现的安全报告。
