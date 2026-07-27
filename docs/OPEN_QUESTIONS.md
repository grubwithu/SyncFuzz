# Open Questions

## OQ-010：P0 Agent 集成的运行镜像

Status: Pending human decision (2026-07-27)

### Context

M1 的真实 smoke/acceptance 已在隔离的 cgroup v2 Linux 容器中完成：容器内只运行
合成的文件、socket 与进程操作，不启动 Agent。该验证只证明 M1 的 BPF hooks、ring
buffer、cgroup filtering、marker 与 artifact 契约正确，不能替代 P0 最终 Gate。

P0 的最终 Gate 仍要求 5–10 个包含 shell tool 的普通 LangGraph 任务运行，并产出
Gap。真实 Agent 与其 shell/tool 子进程必须位于 M1 追踪的目标 cgroup 内。PRD 尚未
指定该运行时镜像的固定 digest，也未授权创建 Dockerfile；使用浮动 tag 会破坏可复现性，
并使 `RunManifest.image_digest` 没有可审计来源。

### Options

1. 人类提供已固定 digest、包含 Python 3.11 与所需 LangGraph 依赖的 Agent runtime
   image；同时给出启动命令。
2. 人类明确授权创建并提交 P0 Agent runtime image 定义，指定允许的路径、基础镜像及
   digest；镜像内运行 LangGraph Agent 与 shell tool。
3. 人类提供另一套精确的 runtime 镜像、启动命令与依赖安装策略。

### Decision needed

请选择一个选项，并给出固定 image digest（或授权其定义文件的位置）。在此之前不得把
M1 合成容器验收表述为 P0 Agent 集成验收，也不得自行选择浮动基础镜像。
