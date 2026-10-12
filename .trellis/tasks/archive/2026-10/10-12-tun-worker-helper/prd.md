# 独立提权 TUN Worker，GUI 不重启

## 目标

主 GUI 正常启动时保持普通权限。用户开启 TUN 时只由系统 UAC 为无窗口 Worker 授权，GUI 不退出、不重启，其他已运行 VPN 不受影响。不安装 Windows 服务；关闭 TUN、退出或失去 GUI 通信后，Worker 清理所属网卡、路由、DNS 规则并退出。移除提权前提示和重启错误两处 MessageBox。

用户已同意创建任务并进入规划阶段；用户随后明确“开始”，已进入实现及验收，inline 模式不派发子代理。

## 已确认背景

基线 main / 182bc1b，详细依据见 research/current-flow.md。

- tun_mode_windows.go:42–110 的 API 保存模式、置 restarting 并退出 Wails；非管理员且其他 VPN 运行时拒绝开启 TUN。
- tun_mode_windows.go:119–207 用 runas、--tun-restart 和等待原进程退出；:129、:202 为两处 MessageBoxW。
- main.go:62–81 在 GUI 创建前处理重启；app.go:162 启动残留清理也可能提权重启。
- connections.go:241 构造每配置 engine.Options；vpnEngine 接口只有 Stop/Resources，状态/资源回调已有会话身份保护。
- engine/engine.go:266–273 在引擎内创建 TUN；engine/tun_windows.go 的 ingress 依赖会话 policy、gVisor、DNS 和 TCP/UDP dialer。只搬路由命令不足以隔离。
- frontend/src/main.js:360 当前禁止本条连接运行时切换 TUN；:540–605 有管理员重启交互。托盘连接也需后端统一准备。
- engine/tun_identity_windows.go 的冲突登记为进程内共享状态；多个独立 Worker 会丢失现有跨 TUN 冲突保护。

## 需求

### R1 — 独立提权入口

同一 VPN.exe 增加 --tun-worker，在 Wails、托盘、GUI 单实例锁和配置加载前分流。只对子进程使用 runas，无额外窗口、控制台或 MessageBox。UAC 取消/启动失败在 GUI 内明确提示，原配置与会话不变。删除 GUI 自退出、管理员替换进程和“先断开所有 VPN”的流程。

### R2 — TUN 独立附着与分离（用户已明确要求）

已连接的本条 VPN 可直接开关 TUN，不断开、不重新认证、不重建 VPN 网络栈或代理监听。VPN Engine、认证、gVisor、资源策略、DNS resolver、SOCKS5/HTTP 始终由 GUI 持有；一个临时高权限 Worker 只持有所有 TUN ingress、Wintun、系统路由/DNS及现有 system-stack 所需的网络配置。Worker 不接收用户名、密码、TOTP或认证令牌，不运行 engine.Start，不读取 GUI 配置文件。

Worker TCP/UDP/DNS 流经命名管道接入 GUI 内本条会话。移除 TUN 只关闭本条 TUN 的流、网卡及系统配置，不关闭 VPN 栈、resolver、代理或其他 TUN。单 Worker 保留进程内跨 TUN 冲突检查，存活期间复用授权；请求/事件按 profileId、sessionId、attachmentId、policyVersion 绑定。

三协议、旧配置、证书选项、端口、slot、资源策略、托盘、取消/独立断开/全部断开及日志归属保持兼容。TUN 使用独立授权资源视图，附着不更改当前代理策略、resolver/fake-IP 映射；原 ZJU 扩展不被误当成 TUN 的服务端资源。

### R3 — 通信与清理

采用本地命名管道和双向心跳，限制权限/帧大小，校验对端进程身份。凭据不进命令行、任务、日志或临时文件；不提供任意 Shell/文件操作接口。

单条 TUN 分离只清理该附着。最后一个附着及显式准备预约释放时退出 Worker；GUI 真正退出等待清理，关闭到托盘继续运行。GUI 崩溃、管道断开或心跳超时取消全部 TUN 请求/流并清理退出。Worker 故障或 TUN 资源更新失败只将 TUN 标为错误；原 VPN 和已有代理自然继续运行，不能自动重启 Worker、重新授权或修改连接模式代替失败。

残留检查不重启 GUI；首次用户开启 TUN 时由授权 Worker 清理已确认属于本程序的遗留项，取消 UAC 不阻止普通代理连接。每次新启动 Worker 仍可能需要 UAC，不承诺跨 Worker 生命周期免授权。

### R4 — 交付与日志

保持单文件 VPN.exe、未压缩默认构建。GUI 为 engine.log 的唯一写入者，Worker 的脱敏结构化日志经控制管道转交 GUI，保留按配置查看。Worker 不打开 GUI crash.log。区分模拟、真实 UAC/权限、系统路由/TUN 与真实网关验收。

## 验收标准

- [ ] AC1/R1：普通 GUI 开启另一配置 TUN 后 PID/权限不变，仅系统 UAC，无 MessageBox，此前会话不停止或重登。
- [ ] AC2/R1：取消授权、启动/握手/保存失败明确返回，配置和已有会话无半完成状态。
- [ ] AC3/R2：本条连接热附着/分离前后认证次数、Engine/gVisor/resolver/代理 listener 身份不变，已建立代理流继续；仅 TUN 流随分离关闭。多条 TUN 共用 Worker，原冲突保护有效。
- [ ] AC4/R2：断开/重试后旧事件或旧启动返回不覆盖新会话。
- [ ] AC5/R3：关闭 TUN、断开本条 VPN、全部停止、退出、GUI 崩溃、IPC/心跳失联都完成所属清理；Worker 故障只显示 TUN 错误，原 VPN/代理继续。
- [ ] AC6/R3：命令行无凭据；未知版本/命令、错误对端 PID、非授权连接、重复会话、超长消息被拒绝，清理不跨其他客户端。
- [ ] AC7/R4：按配置日志可用，无跨进程轮转冲突；根/相关嵌套模块 test/vet/race、前端与 VPN.exe 构建通过。
- [ ] AC8/R1–R3：实际 UAC 同意/取消、两进程权限、网卡/路由/DNS 创建清理分别记录，未执行实网项目不宣称通过。
- [ ] AC9/R2：IPC 实际 TCP 双向数据、UDP 分包、DNS/fake-IP/域名 ACL、动态 /32 和重连资源更新测试通过；拒绝未授权目的、端口、旧策略和默认路由，不破坏最长前缀规则。
- [ ] AC10/R1–R3：附着与断开/取消/重复开关/资源更新交错时，停止胜出、旧结果无效、无泄漏/死锁；配置保存失败/回滚失败/清理失败均明确显示实际状态，不重新登录补偿。

## 边界与规划状态

不安装服务，不改协议/认证/路由范围，不新增自动重试或兜底，不涉及 Android。每次文件写入后提交，不推送远程。

用户已选择拆分 TUN 附着，完整引擎 Worker 迁移方案不采用。连接中/正在停止的配置不做并行附着：保持其开关暂不可用；已连接时可热切换，reconnecting 时仅允许移除现有 TUN。未连接时保存开关，开启会准备 Worker；后续连接由同一后端入口执行附着。没有阻塞的产品决策，design.md/implement.md 已完成；用户已审阅并明确“开始”；实现及验收证据见 validation.md。
