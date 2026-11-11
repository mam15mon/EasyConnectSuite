# 四端发布与完整模块测试验收

## Goal

补齐 Windows、macOS、Linux 的嵌套核心模块测试，发布同一提交的四端产物并明确验证限制。

## Requirements

- Windows、macOS Intel / Apple Silicon、Linux GUI / headless 和 Android 产物来自同一提交，并发布到 GitHub Release。
- Windows、macOS、Linux CI 均运行根模块及独立 zju-connect 模块的既有测试和 vet；三端均运行前端测试。
- 修正已复现的 Windows 文件权限测试断言：保留文件内容、PCAP 双向数据等功能检查，只在支持 POSIX 权限的平台检查 0600。
- 保留既有协议与功能边界，不添加产品兜底或更改认证、连接、TUN 行为。
- 记录系统测试跳过项、真实网关与实体设备未验证范围；Linux 用户入口仅代理，Android 为 EasyConnect 原型。

## Acceptance Criteria

- [ ] 本地根模块、嵌套模块、前端测试与 vet 通过，跳过项可追溯。
- [ ] 同一提交的 Windows、macOS 双架构、Linux、Android Actions 全部成功。
- [ ] Release 包含六个二进制下载文件及 SHA256 校验清单，校验发布资源与 Actions 产物一致。
- [ ] 发布说明给出提交、CI 链接、平台功能范围和未完成的真实环境验收。

## Notes

- 本任务为有限的 CI、测试平台断言及发布文档修改，按轻量任务执行。
- 当前会话已授权验证、推送及 GitHub 多端发布，遵循项目约束直接执行。
