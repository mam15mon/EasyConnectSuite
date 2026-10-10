# UniVPN 认证 MAC 格式对齐与返回码日志

## Goal

读取实际错误得到 AUTH 返回码2；官方成功日志显示认证域与Go一致，但MAC为14字符大写连字符分组，Go为17字符冒号格式。对齐工作原型的稳定MAC编码并持久化拒绝返回码。

## Requirements

- TBD

## Acceptance Criteria

- [ ] TBD

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
