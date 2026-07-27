# 001. P0 artifact 使用未压缩 JSONL

Status: Accepted
Date: 2026-07-27
Affects: PRD §4.1, §7

## Context

PRD §4.1 固定了 `kevents.jsonl`、`aevents.jsonl`、`memo.jsonl` 与
`ckpt_index.jsonl` 等 artifact 文件名；`docs/skeleton.md` B.5 则使用
`.jsonl.zst`。两者会令模块间文件契约不兼容。P0 的 artifact 规模不需要压缩，
而当前环境也不提供 `zstandard`。

## Decision

P0 的 JSONL artifact 一律使用 PRD §4.1 中的未压缩 `.jsonl` 文件名。M0 的
artifact IO 不依赖 `zstandard`；`manifest.json`、`provenance.json`、`gap.json`
与 `ablation.json` 继续使用 JSON 单对象文件。

## Consequences

M1、M2 与 M8 必须以原始 JSONL 文件互通，不能接受或产生 `.jsonl.zst`。
Skeleton B.5 中的压缩映射仅作为被取代的实现示例。由于项目尚无历史 run，无需
迁移或重跑。

## Rejected Alternatives

1. 使用 `.jsonl.zst` 并更新 PRD：这会无必要地改变已明确的 run 布局并增加依赖。
2. 同时支持 `.jsonl` 与 `.jsonl.zst`：双格式会扩大冻结契约、掩盖生产者与消费者
   的不一致，且 P0 不需要兼容层。
