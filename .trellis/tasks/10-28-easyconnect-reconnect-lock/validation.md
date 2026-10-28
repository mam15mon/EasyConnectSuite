# 验证记录

## 根因与边界
源代码和已观察调用栈共同表明：重连在认证成功后同步关闭旧 gVisor TCP 流，该调用从 TCP 发包栈进入，Close 再次等待端点锁；同时 attachment 读锁跨 backend Dial/DNS 持有，可阻塞 Update 写锁。未将网关终止会话的原因或另一台机器退出时间写成已确认事实。不保留私有日志、地址、配置及凭据。

原实现回归失败：旧 Close 阻塞时 Update 无法返回；pending Dial 阻塞 Update 且旧结果能够发布。修复后针对性测试通过，覆盖独立旧流关闭、同步 Dial 重入更新、Dial/DNS 并发更新/停止、过期结果及持久化诊断。

## 已执行
- 根 `go test ./...`、`go vet ./...` 通过。
- 根 `go test -race ./... -count=1 -timeout=180s` 通过。使用现有 MinGW GCC，CGO_ENABLED=1，工具目录加入当前命令 PATH；首次仅指定 CC 未配置该目录时 cgo 构建失败，环境修正后通过，未安装工具。
- 嵌套模块 `go test ./client/easyconnect ./stack/gvisor ./log`、对应 vet 通过，无内嵌模块代码修改。
- `wails build -m -nosyncgomod -trimpath -s -skipbindings` 通过，Windows amd64 生产构建。前端和绑定未改动，复用现有构建资源。

## 未验证
未对真实双机抢占、真实网关重连或系统 TUN 路由做现场回归。当前运行旧进程不包含修复；新包需退出旧进程后使用。不自动中断其它正在运行的 VPN。
