# Linux TUN 与三协议功能对齐

## Goal

为 Linux GUI 与 headless 开放系统 TUN、代理及 EasyConnect/UniVPN/TopSAP 当前桌面协议范围，补齐日志测试并发布四端同提交产物。

## Requirements

- Linux GUI 与 headless 支持现有桌面协议范围：EasyConnect、UniVPN、TopSAP 标准模式；保持代理并可显式开启资源分流 IPv4 TCP/UDP TUN。
- GUI 主进程保留普通权限，经独立 Worker 获取所需系统权限；headless 提供明确的非图形授权用法。TUN 开关和失败不重启 VPN、不破坏原有代理会话。
- 兼容原有深信服配置、多连接身份、资源范围、重连及清理契约；只清理本程序创建的接口/路由/DNS。
- Linux TopSAP 设备身份必须基于官方 Linux 客户端的可核对证据，不能套用其他平台算法、伪造身份或添加未要求的兜底。
- 认证、连接、TUN、IPC 和清理保留操作身份、阶段、耗时、错误/取消与受限脱敏阻塞证据。
- 同一提交经四端 GitHub Actions 构建测试后发布 GitHub Release，分别记录模拟、真实 Linux 内核/系统服务和真实网关验收。

## Acceptance Criteria

- [ ] Linux GUI 支持 TUN、代理、双模式及三种协议，保存/启动/热切换/失败回滚行为可验证。
- [ ] Linux headless 提供 TUN 与代理模式，取消/信号退出能清理自有系统状态。
- [ ] 真实隔离 Linux TUN 的 TCP/UDP/DNS、两连接隔离、资源更新、崩溃/断连清理通过，取消和迟到响应不会污染其他接口。
- [ ] TopSAP Linux 身份有官方实现证据及独立测试向量，三协议至少完成模拟网关集成。
- [ ] 既有 Windows/macOS/Android 测试与构建保持通过；四端同提交发布并验证下载链接/哈希。

## Notes

- “所有客户端”指 Windows/macOS 已有的三种协议范围，不包含那些平台尚未实现的认证模式/IPv6/DTLS。
- Linux 首发运行目标沿用 Ubuntu 24.04 / x86-64；真实网关和实体设备结论须以实际验证为准。
- 现有内部 Linux TUN/Worker 已有系统测试，但 DNS 请求完成和设备生命周期、用户接线与 TopSAP 设备身份仍待完成。
- 用户已授权实现并发布，项目约束优先于生成技能的重复许可步骤。
