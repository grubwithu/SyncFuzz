# Phase 2 Review

## 结论

Phase 2 的方向是正确的，可以作为 Phase 3 的输入 contract。

项目仍然保持在主动漏洞挖掘主线上：不是 prompt benchmark，也不是防御型 transaction system。新增的 `state-trace.json` contract 正好连接 deterministic known-answer seeds 与后续 fault scheduler：每个 run 都能把 lifecycle phase 映射到 Agent、OS、External、Authority 四层观测。

## 审查发现

- **已修正：container workspace effect 没有完全通过所选 environment 执行。** `branch-leakage` 和 `partial-filesystem-rollback` 之前由 Go host process 直接修改 bind-mounted workspace。可见 artifact 是正确的，但执行语义弱于 container backend contract。现在这两个 case 都改为使用 `env.ExecShell`，local 与 container run 会通过同一个 environment abstraction 执行 workspace effect。
- **接受：layer presence 显式标记，而不是强行制造空 snapshot。** filesystem-only run 会在 `state-trace.json` 里把 External 与 Authority 标记为 absent；external / authority run 也只在确实有 workspace 时标记 OS 观测。这样 schema 统一，同时不引入伪观测。
- **接受：process lineage 当前按 PID 关联。** 对 deterministic MVP 和 container namespace snapshot 足够。后续进入长时间 fuzz campaign 时，可能需要加入 process start time 或 namespace identifier，以降低 PID reuse 带来的歧义。

## Phase 3 准备度

Phase 3 scheduler 现在可以消费：

- `manifest.json`：testcase 设计意图、lifecycle phase、primitive 和 expected signature。
- `state-trace.json`：跨层 artifact 与 lifecycle phase 对齐索引。
- `agent-state.json`：deterministic Agent-layer projection。
- `filesystem-metadata.json` 与 `process-lineage.json`：OS-level feedback。
- `external-before.json` / `external-after.json`：External 与 Authority state projection。

下一步应先实现一个小的 fault-plan abstraction，复用现有 P0-P8 phase vocabulary，再引入随机化或 feedback-guided scheduling。
