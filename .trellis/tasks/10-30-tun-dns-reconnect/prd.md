# 修复 TUN DNS 探测与重连锁循环

## Goal

将 DNS 探测移出 TUN 操作锁，拒绝过期结果，验证重连与清理不再互等。

## Requirements

- TBD

## Acceptance Criteria

- [ ] TBD

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
