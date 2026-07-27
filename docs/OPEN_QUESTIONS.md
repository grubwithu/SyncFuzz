# Open Questions

## OQ-001：P0 artifact 表示与 bootstrap 授权

Status: Resolved (2026-07-27)

### Context

`docs/PRD.md` §4.1 将跨模块 artifact 固定为 `kevents.jsonl`、
`aevents.jsonl`、`memo.jsonl` 和 `ckpt_index.jsonl` 等文件名；但
`docs/skeleton.md` B.5 将同一组 artifact 固定为 `.jsonl.zst`，并要求
M0 的 `RunDir` 依据该映射读写。`AGENTS.md` 已将 artifact 名称和编码视为
冻结契约，因此不能自行选择其中一份规范。

此外，Skeleton 的 P0 骨架要求 `pyproject.toml`、`constraints.txt`、
`.importlinter`、`schemas/` 与 golden fixture 根目录；当前写入授权明确禁止这些
路径。当前 Python 环境也没有 `pydantic`、`zstandard` 或 `pytest`，所以不能按
冻结技术栈实现和验证 M0。

### Options

1. 以 PRD §4.1 的无压缩 `.jsonl` 文件名为冻结 artifact 表示；Skeleton 的
   `.jsonl.zst` 映射仅作过时示例。
2. 以 Skeleton B.5 的 `.jsonl.zst` 映射为冻结 artifact 表示，并通过 ADR 更新
   PRD §4.1 中的文件名与编码。
3. 指定另一种明确的 artifact 命名/压缩方案，并通过 ADR 同步所有冻结文本。

### Resolution

人类已确认：

1. P0 artifact 使用 PRD §4.1 中的原始 `.jsonl` 文件名，不压缩；
2. 允许创建依赖声明/锁定文件、`schemas/`、架构 lint 配置及 shared golden
   fixture 路径；
3. 允许安装依赖，但 `zstandard` 不再是 P0 依赖。

该决定已记录于 `docs/adr/001-raw-jsonl-artifacts.md`。

### Impact

M0 的 `RunDir`、schema 导出与测试环境可按该决定实施。后续实现不得恢复
`.jsonl.zst` 或引入 `zstandard`。

## OQ-002：ResourceId 文件键与 Violation 分类 enum

Status: Resolved (2026-07-27)

### Context

冻结 PRD §4.4（也在方法论 §4.1 重复）定义文件/路径身份为
`(dev, ino) + content hash`。但 Skeleton B.3 的 `ResourceId._KEYS["file"]`
只包含 `("dev", "ino")`，会把同一 inode 上内容已改变的对象判为同一身份。

冻结 PRD §4.5 将 `Violation.cls` 限定为 `REBOUND`、`RESIDUE`、`MISSING`、
`DUPLICATE`、`ORPHAN`、`ESCAPED`、`BELIEF_DIVERGENCE`；`CONSISTENT` 不是
违规记录。Skeleton B.4 则让 `Violation` 与 M6 的 `Verdict` 共用一个包含
`CONSISTENT` 的 `ViolationClass`。

### Options

1. 按 PRD 实现：文件键为 `(dev, ino, content_hash)`；为 M6 的 `Verdict` 单设可含
   `CONSISTENT` 的 enum，而 `Violation.cls` 保持 PRD 的七种违规类。
2. 按 Skeleton 实现：文件键仅为 `(dev, ino)`，且 `Violation.cls` 也允许
   `CONSISTENT`；这需要 ADR 修改 PRD §4.4 与 §4.5。
3. 指定其他精确的 identity key 与 enum 分层方案，并通过 ADR 固化。

### Resolution

人类已确认按 PRD 实现。所有 Skeleton 与 PRD 不一致的 identity 或分类语义均以
PRD 为准：文件键包含 `content_hash`；可执行文件键包含解析后的 `abs_path`；
`Verdict` 与 `Violation` 使用不同 enum，只有前者允许 `CONSISTENT`。不创建
兼容字段或双语义比较逻辑。

## OQ-003：Timeline 的冻结 Pydantic 容器形状

Status: Resolved (2026-07-27)

### Context

PRD §3 的 M0 API 要求提供名为 `Timeline` 的 Pydantic v2 model，PRD §4.1 又将
timeline 定义为 `timeline.jsonl` artifact。但 §4 未定义 `Timeline` 字段，
`docs/skeleton.md` B.4 只定义了单条 `TimelineEntry`，没有定义 `Timeline` 容器。
这会影响 M5 的 JSONL producer，以及 M6 `gamma(u, t: Timeline, b: Timeline)` 的
输入类型。

### Options

1. `TimelineEntry` 是 JSONL 的单行 Pydantic model；`Timeline` 是包含
   `entries: tuple[TimelineEntry, ...]` 的冻结 Pydantic container，供 M6 使用。
2. 将每条 JSONL record 命名为 `Timeline`，不额外定义 container；M6 的两个
   timeline 参数改为记录序列类型。
3. 指定另一种字段与容器方案，并通过 ADR 固化。

### Resolution

人类已确认方案 1：`TimelineEntry` 是 `timeline.jsonl` 的单条 Pydantic record；
`Timeline(entries: tuple[TimelineEntry, ...])` 是供 M6 接收完整共轴时间线的冻结
container。两种形态各司其职，不提供双语义 alias。
