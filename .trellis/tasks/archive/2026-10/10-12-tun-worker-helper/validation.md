# TUN Worker 验收记录

日期：2026-10-12。任务分支：codex/tun-worker-helper，基线 main / 182bc1b。主会话 inline 完成实现、审查和测试；保留同工作区已提交的 VPN Suite 界面改造。本记录不包含真实用户配置、网关、凭据或私有日志。

## 交付结果

普通权限 GUI 保留 VPN 认证、网络栈、resolver、SOCKS5/HTTP 代理。开启 TUN 时仅以 runas 启动同一程序的无窗口 --tun-worker 子进程，不再调用 MessageBox 或退出重启 GUI。已连接 VPN 可以独立附着、分离 TUN；TUN 失败通过独立状态报告。单 Worker 承载多个附着并复用其生命周期内的授权，最后一个附着及准备预约释放后退出。

Worker 只接收版本化网络策略，通过本地命名管道回接 GUI 会话；不读取 profile，不接收认证凭据。身份、方向、版本和资源校验、有界请求/数据帧、TCP 半关闭、UDP 数据报、双向心跳及父进程退出清理均已实现。关闭到托盘保留原连接及心跳。

## 需求与验证对应

| 验收项 | 已取得的证据 | 范围 |
| --- | --- | --- |
| AC1 独立提权、保留原会话 | asInvoker manifest、最早 Worker 分流、删除旧 GUI 重启及 MessageBox；真实普通父进程与高权限 Worker 测试；App 连接独立性回归 | 普通父进程的系统验收使用测试程序入口；未用真实网关测 Wails GUI 热切换 |
| AC2 保存/授权失败 | 保存先于 UAC、等待不持有 App 锁、取消只回滚目标配置、保存失败不启动 Worker；真实点击 UAC 否通过 | 启动/握手错误返回路径经代码审查；未逐项注入所有 Windows 错误码 |
| AC3 热附着与策略隔离 | Engine/resolver/policy 身份断言；实际已建立 SOCKS5/HTTP CONNECT 流跨四次切换继续；原 slot/前缀/DNS 冲突回归；真实双附着共享 Worker | 会话使用本地模拟 VPN 后端 |
| AC4 旧结果不覆盖新状态 | 原连接实例保护回归、显式配置 ID、旧资源请求/事件乱序测试；过期 Worker policy 拒绝 | 模拟和真实命名管道 |
| AC5 清理与故障隔离 | Stop 取消待附着；Worker 丢失/更新失败保留 VPN；真实双附着分别清理与最后退出；实际父进程强杀清理；控制管道实际 15 秒超时 | 未主动强杀正在使用的用户 VPN；系统权限/命令失败后的全部残留组合未逐项实测 |
| AC6 IPC 安全约束 | 未知/超长/截断帧，64 请求限额，错误 server/client PID，唯一实例，关闭唤醒，方向和旧策略拒绝；代码审查 DACL、本地管道、进程句柄/映像验证、命令行和封闭命令 | 跨管理员账户 UAC 未执行；非授权账户连接未另建账户实测 |
| AC7 日志与质量 | GUI 单写日志、Worker 独立 crash 输出和 IPC 转发；root/相关嵌套模块 test/vet/race、Node 与 Wails 构建通过 | 代码审查与自动检查；不记录真实用户日志 |
| AC8 系统验收记录 | 实际同意、取消、两进程权限、两条真实 Wintun/路由/NRPT、TCP/DNS 与父进程崩溃后的清理，见下文 | 真实 Windows 网络操作，隔离模拟后端 |
| AC9 数据与资源 ACL | 真实管道 TCP 双向/半关闭、UDP 多包及旧策略拒绝；DNS/fake-IP/端口 ACL、域名路由与最长前缀既有回归；真实系统 TCP/DNS | UDP 管道为真实命名管道；未用真实网关测系统 UDP |
| AC10 交错与状态一致性 | Stop 取消 attach、Worker 丢失/更新失败、配置保存/取消回滚、App 热切换状态和前端 UAC 拒绝释放 busy 的断言；race 通过 | 未对所有交错排列、保存回滚失败及系统清理失败逐项故障注入 |

原 PRD/实施计划是完整测试目标；表中边界不因任务归档而成为已通过的声明。没有为测试添加生产时钟接口，心跳直接等待真实 15 秒超时。功能已完成，剩余跨账户及真实网关验证作为明确的环境边界保留。

## 实际 Windows 验收

使用 ECS_TUN_SYSTEM_TEST 显式启用的三个测试，均要求普通权限测试父进程，调用生产 Worker 入口和 WorkerManager。测试仅使用保留槽位 60/61、文档保留网段及 .example 域名，不加载用户配置。

1. `TestTUNSystemUACAndTwoAttachments`：用户同意 UAC；普通父进程 PID 9484 保持不变，高权限 Worker PID 49084。两条真实 TUN 的系统路由与 NRPT 数量依次为 2、1、0；系统 TCP 经 Worker/GUI 管道到本地模拟端点，实际虚拟 DNS 查询成功。通过，14.38 秒。
2. `TestTUNSystemUACCancellation`：早期环境静默授权，不能当作取消成功。用户调整 Windows 配置后重新执行并实际点击否；未保留 peer、预约或附着。通过，1.34 秒。
3. `TestTUNSystemParentCrashCleanup`：普通测试父进程建立真实 TUN 后被测试编排器强制结束；高权限 Worker PID 49880 清理路由/NRPT 并退出。通过，10.79 秒。

测试结束检查槽位 60/61 无网卡、对应 NRPT 数量为 0。最终再次检查亦为 0。后续用户自行启动的 VPN 进程保持运行，不纳入测试清理。

初次系统测试的 PowerShell 计数格式错误已修复并重跑；一次取消测试实际得到授权以及一次等待 UAC 的超时均未记作通过，最终分别取得上述成功证据。未修改用户 Windows 安全配置。

## 自动质量检查与审查

- `go test ./... -count=1 -timeout=180s`：最终源代码通过；root 0.306s、engine 4.024s、topsec 0.427s、univpn 15.343s、tunipc 15.069s。交互系统测试默认跳过，单独结果见上文。
- `go vet ./...`：通过。
- `go test -race ./... -count=1 -timeout=180s`：实现后全模块通过；最终新增回归后 `go test -race . ./internal/tunipc -count=1 -timeout=90s` 再次通过（root 2.143s，tunipc 17.096s）。使用已有 MinGW GCC 和 CGO，无新工具安装。
- 嵌套 zju-connect 的 service、dial、resolve、stack/gvisor、client/easyconnect：普通测试、vet 和 race 均通过；该模块没有本任务生产改动。
- `node --check frontend/src/main.js`、`node --check frontend/src/preview.mjs`：通过；Node 14 项测试通过，包括捕获配置 ID、拒绝 UAC 后释放 busy、资源乱序及搜索范围。
- 脱敏 840×660 preview：连接保留时的开关/等待、不同配置、单独 TUN 错误与代理可用状态已检查；不以 preview 证明系统提权。临时服务已停止、标签已关闭、窗口尺寸恢复。
- trellis-check 审查 Engine/IPC/Windows/API/UI/日志契约；修复管道共享 os.File 偏移竞态、pending ConnectNamedPipe 的关闭顺序、资源更新 slot 身份及前端请求配置捕获。没有新增自动重试或登录补偿。
- `.trellis/spec/backend/tun-worker.md` 写入七段可执行契约，前后端索引及用户说明已同步。Wails 绑定/models/dist 已同步，`git diff --check` 通过。

## 最终构建

`wails build -m -nosyncgomod -trimpath` 最终成功，windows/amd64、production、Compress=false，包含 VPN Suite 最终界面。输出 `build/bin/VPN.exe`，20,864,512 字节。

SHA256：`21CD327FC7C2CE8FA548DE10C1014913BE273CD7AB717A17D5E2268F21E72B21`。

build/bin 根目录仅保留 VPN.exe；现有许可证通知及源码合规目录保留。该构建文件为交付产物，未作为 Git 源码提交。

## 未执行的环境验证

真实 EasyConnect/UniVPN/TopSAP 网关登录与长时稳定性、跨管理员账户 UAC，以及休眠恢复/强杀 Worker/系统命令失败的完整真实环境矩阵未执行。现有协议/配置兼容性由自动回归支持，不能表述为真实网关验收。TopSAP 实网任务仍独立保留。
