# 002. 在 C 组件中 vendor cJSON 处理 JSON

Status: Accepted
Date: 2026-07-27
Affects: PRD §7；M1 C CLI；未来的 M5 C CLI

## Context

当前 M1 的 C loader 手写 JSONL 序列化。它不能安全地读取并原子更新既有的
`manifest.json`，也会让不同 C 程序各自维护 JSON 转义和错误处理。人类要求使用
[cJSON](https://github.com/DaveGamble/cJSON)，并将其源文件直接放入项目代码中，而
非要求通过系统包或运行时下载。

PRD §7 冻结技术栈，`AGENTS.md` 规定新增第三方代码必须先有 ADR 并等待确认。
源码被 vendor 并不会改变其第三方来源、许可证或安全更新责任；本 ADR 因而明确
其来源、固定版本和更新边界，而不把它视作可由构建环境任意替换的系统依赖。

## Decision

人类已确认 vendor 方式，并指定导入当时上游默认分支的最新提交。固定快照如下：

- 上游：`https://github.com/DaveGamble/cJSON`
- 默认分支：`master`
- commit：`fb16e5cf358798aabb049655975cde8427101056`
- 导入时的上游许可：MIT（`LICENSE`）
- `cJSON.c` SHA-256：`607e756460fa0de37d20a7a9181f2de29c97bfb7ce5a0e6c2f548243836cd852`
- `cJSON.h` SHA-256：`25b0145150d500498e4d209cec69c18c42cf818bffcc54690be3b895a2a16dee`
- `LICENSE` SHA-256：`a36dda207c36db5818729c54e7ad4e8b0c6fba847491ba64f372c1a2037b6d5c`

人类已额外授权创建并使用 `syncfuzz/third_party/`；该目录仅容纳原样的第三方
源码，不受项目自编代码的行数、函数圈复杂度和 Overview 注释规范约束。该例外不
扩展到任何 SyncFuzz 自编代码或测试。

1. 在 `syncfuzz/third_party/cjson/` 原样 vendor cJSON 的上游 `cJSON.c`、
   `cJSON.h` 和许可证文本。M1 和未来获授权的 C 模块可各自把该确定的源码编译进
   自己的可执行文件；不引入下载步骤、pkg-config 查找、共享库链接、git submodule
   或其他 C JSON 库。该源码目录不定义 SyncFuzz 模块间的运行时 API。
2. 只接受以上精确 commit 的文件和校验值；不得使用浮动分支或 “latest”。
3. M1 的 C 代码所有 JSON 序列化与反序列化均只经 cJSON 进行；其中包括严格读取、
   修改并原子替换已有的 `manifest.json`，以及写出 `kevents.jsonl` 的每行 JSON。
   写出的对象仍必须满足 PRD §4 冻结 schema；cJSON 不改变 artifact 格式或字段。
4. 当前 M0 仍是 Python/Pydantic v2 模块，因而不迁移为 C。若将来 M0 新增获授权的
   C 可执行程序，该程序同样遵循本 ADR。未来 P1 的 M5 若为 C 程序也遵循本 ADR，
   但本 ADR 不授权创建或实现 M5。

## Compatibility and safety

该提议不改变任何 §4 schema、artifact 名称、JSONL 编码、模块边界或 M1 CLI 参数。
M1 仍须在 JSON 解析失败、缺少字段或字段类型不符时 fail fast；不得以 cJSON
解析结果为由静默丢字段或发布部分 artifact。vendor 的源文件只随使用它的模块一同
编译，避免把它包装成跨模块共享运行时接口。

## Alternatives considered

1. 继续手写 JSON：实现短期较少，但会重复 JSON 转义、解析及 manifest 更新的
   风险，且不符合人类的库选择。
2. 使用系统 cJSON 包：依赖发行版版本和 ABI，构建不可复现，也与“源文件放到项目
   中”的要求不符。
3. 引入另一套 C JSON 库：不符合已指定的 cJSON 选择，并扩大技术栈变更。

## Implementation note

`cJSON.c` 原文件有 3,206 行。人类已在 OQ-007 中明确授权
`syncfuzz/third_party/` 内的原样第三方源码不受本项目代码行数等编程规范约束。
