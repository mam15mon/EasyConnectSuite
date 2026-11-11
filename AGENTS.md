<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

## 项目约束（用户要求，优先于生成的工作流默认行为）

- 未经明确要求，禁止添加兜底方法。
- 每次命令修改项目文件后，提交该次修改；提交消息概括修改内容。只读命令不创建空提交。
- 先说明假设和可验证目标；坚持最小实现，只改与请求直接相关的代码。
- 用户在当前会话已经授权的安装、配置或开发工作，直接推进，不重复索取任务创建或实现许可。
- 原有深信服配置需要保持兼容。协议支持范围、构建成功、模拟测试和真实网关验证分别记录。
- 本项目是多端项目：Windows、macOS（Intel / Apple Silicon）、Linux（GUI / headless）和 Android。交付时必须将对应提交推送到 GitHub，通过 GitHub Actions 完成各端构建和既有测试；不得仅交付本机 EXE 或将单端构建称为多端完成。
- 构建通过后，将同一提交的各端产物发布到 GitHub Release，并向用户提供可用的 GitHub 下载链接。明确各端实际支持范围及尚未验证的项目；Android 当前是 EasyConnect 原型，不代表具备桌面端的全部协议和功能。
- 凭据、令牌、真实用户配置和私有日志不得写入任务资料或 Git。
- 后续新增或修改认证、连接、重连、TUN、IPC 和清理等关键流程时，必须同步提供可定位的日志：连接/操作身份、步骤、耗时、错误及取消状态；阻塞路径需要自动保留受限且脱敏的线程证据。只记录已观察到的事实，不把推测写成已确认根因。具体契约见 `.trellis/spec/backend/logging-guidelines.md`。
