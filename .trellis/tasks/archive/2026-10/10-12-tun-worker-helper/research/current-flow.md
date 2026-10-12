# 提权与 TUN 数据路径研究（2026-10-12）

基线 182bc1b，只读研究，产品代码未改。

## 当前路径

SetProfileTUNMode 在 a.mu 内保存配置，非管理员且任何连接运行时拒绝；随后 MessageBox → ShellExecuteExW runas → --tun-restart PID configPath → Wails Quit。新 GUI 等待原进程退出并加载原配置。startup 的 recoverStaleTUN 残留清理也复用此链。

frontend 把 API bool 当 restarting；托盘直接调 StartProfileVPN，故新提权准备必须在后端统一覆盖，不能只改按钮。现有测试证明保存、取消回滚、显式 ID、通过拒绝提权来保护其他连接、等待旧进程；新测试须正向证明 GUI/其他连接持续运行。

## 初轮隔离选择（后续用户已要求热附着，不采用完整引擎迁移）

engine/engine.go:197–273 创建 gVisor/policy/router/ingress。tun_windows.go:219–444 的 TCP/UDP/DNS handler 依赖会话 dialer/stack/resolver，并建立域名动态 /32 路由和绑定。依赖 sing-tun 的 stack_system_windows.go:fixWindowsFirewall 还会创建防火墙规则；只分离管理员路由命令不够。

推荐一个共享 Worker 承载 TUN 配置的完整 Engine，GUI 使用实现 Stop/Resources 的远程句柄继续管理连接，保留原生 TCP/UDP/DNS/重连及 tunPolicies。该 TUN 会话认证和代理也在 Worker 内，凭据仅走本地验证管道；普通代理会话不搬迁。

若要求同一已运行连接不重登即可开关 TUN，则需要 Worker 持有 Wintun/路由、GUI 用远程设备读写，或 Worker ingress 经每流 IPC 回调 GUI dialer；两者需额外数据通道及 system-stack 权限改造，不能当作默认轻量扩展。每配置一个完整引擎 Worker 会分裂 tunPolicies，需再设计共享冲突管理，故不推荐。

## Windows 官方依据

- runas 触发系统 UAC；SEE_MASK_FLAG_NO_UI 保留安全提示。SEE_MASK_NOCLOSEPROCESS 获取进程句柄，调用者负责关闭。SW_HIDE 控制显示，Worker 自身仍须绕过 Wails。
  https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-shellexecuteinfow
- 管道必须有显式 DACL，不能沿用默认 Everyone 读取权限，并禁用远程客户端。跨账户 UAC 可能使用另一管理员身份，不能仅允许 GUI 用户后假定连接总成功；限定授权主体并验证实际启动进程。
  https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights
- 双向 PID API 校验真实对端，保留对应进程句柄/创建时间/映像避免 PID 重用；GUI 预创建唯一管道，FIRST_PIPE_INSTANCE 防抢占。不要信任消息里的自报 PID。
  https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeclientprocessid
  https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeserverprocessid
  https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createnamedpipea
- 本机 x/sys v0.45.0 已有管道/PID API 及 FIRST_PIPE_INSTANCE、PIPE_REJECT_REMOTE_CLIENTS，无需新增管道库。

## 必须继续设计的边界

- engine/log_capture.go 为 process-local hub，不能让两进程轮转同一路径。固定 Worker 日志并合并 profile_logs.go，或 GUI 唯一写 IPC 日志；现有 OnLog 向每引擎广播相同行，不能直接每 callback 转发造成重复。不抓 stdout 推断状态。
- worker 入口在 GUI setupCrashLog/NewApp/Wails/单实例锁前分流，不读用户配置文件。普通 GUI 启动不自动提权。
- parent death、pipe EOF、心跳超时进入同一清理链；关闭 GUI 到托盘不等于退出。强杀 Worker 无法保证 defer，仍需下次用户授权后的所属残留清理。
- UAC 只在现存 Worker 生命周期内复用；Worker 已退出再开启可能再次确认。
- 保留 slot 0–63、相同前缀/重叠 DNS 拒绝、更具体路由优先、默认路由拒绝、未知配置 ID 错误和旧回调身份防护。

下一步确认“本条已连接配置是否需原地开关”，再收敛最终 IPC/日志/生命周期设计。尚未运行任何真实提权、系统网络修改或网关连接。

## 用户决策后的收敛

用户明确回复“需要拆分tun的附着”。最终采用 GUI 保留全部 VPN Engine/resolver/代理，Worker 只持有 ingress/系统配置，每流 TCP/UDP 数据管道和 DNS RPC 回调 GUI。详细 design.md/implement.md 已编写，前文完整引擎 Worker 建议仅保留为研究历史，不能实施。

补充证据：loadPolicy 的 conf.TUNMode/DisableZJUConfig 会改变代理资源，热开关不可重建并交换该 policy；需在扩展前保存原始资源，单独构造 TUN view。原 resolver/IPPool 保留，TUN 后端过滤域名授权并保持 TCPPrefL3/AppID/fake-IP上下文。原 answerDNS 添加系统动态路由，需拆 GUI DNS/ACL 与 Worker路由提交。relogin 中 Update 失败目前会传播到 VPN 恢复，新的 TUN失败应隔离，不触发会话重登。

技术边界在最终设计中明确：单控制管道与独立按流数据管道、typed/versioned请求、双向真实PID校验、GUI独占 engine.log、Worker不接收认证数据、per-attachment/session/policy版本及取消。产品决策已解决；只待用户审阅完整最终方案。
