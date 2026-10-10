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
- 凭据、令牌、真实用户配置和私有日志不得写入任务资料或 Git。