# 独立提权 TUN Worker，GUI 不重启

## 目标

主 GUI 正常启动时保持普通权限。用户开启 TUN 时只由系统 UAC 为无窗口 Worker 授权，GUI 不退出、不重启，其他已运行 VPN 不受影响。不安装 Windows 服务；关闭 TUN、退出或失去 GUI 通信后，Worker 清理所属网卡、路由、DNS 规则并退出。移除提权前提示和重启错误两处 MessageBox。

用户已同意创建任务并进入规划阶段；当前仅编写计划，不修改产品代码，inline 模式不派发子代理。

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

### R2 — 会话归属

建议一个 GUI 对应一个临时 Worker，共享所有 TUN 会话；普通代理引擎留在 GUI。TUN 的完整 engine.Start 在 Worker 内执行，IPC 传控制、状态和资源，避免新增原始包或每条 TCP/UDP 流转发协议。Worker 存活期间复用授权，启动/停止/事件按 profileId + sessionId 绑定。

保留三协议支持范围、旧配置、证书选项、端口、slot、资源策略、取消启动、托盘、独立停止、全部断开及日志归属。Worker 只接受验证后的 IPC 配置，不读写 GUI 的 vpn_profiles.json。

### R3 — 通信与清理

采用本地命名管道和双向心跳，限制权限/帧大小，校验对端进程身份。凭据不进命令行、任务、日志或临时文件；不提供任意 Shell/文件操作接口。

单条停止只清理该会话。最后一条 TUN 结束且无准备请求时退出 Worker；GUI 真正退出等待清理，关闭到托盘继续运行。GUI 崩溃、管道断开或心跳超时取消所有启动/引擎并清理退出。Worker 故障仅将所属 TUN 会话标为失败，GUI 与普通代理会话保持运行，不自动重启或代理兜底。

残留检查不重启 GUI；首次用户开启 TUN 时由授权 Worker 清理已确认属于本程序的遗留项，取消 UAC 不阻止普通代理连接。每次新启动 Worker 仍可能需要 UAC，不承诺跨 Worker 生命周期免授权。

### R4 — 交付与日志

保持单文件 VPN.exe、未压缩默认构建。GUI/Worker 不同时轮转同一个 engine.log/crash.log；最终设计规定唯一写入者或固定独立文件，保持按配置查看。区分模拟、真实 UAC/权限、系统路由/TUN 与真实网关验收。

## 验收标准（规划草案）

- [ ] AC1/R1：普通 GUI 开启另一配置 TUN 后 PID/权限不变，仅系统 UAC，无 MessageBox，此前会话不停止或重登。
- [ ] AC2/R1：取消授权、启动/握手/保存失败明确返回，配置和已有会话无半完成状态。
- [ ] AC3/R2：多条 TUN 共用 Worker，路由/DNS 冲突保护有效；状态/资源/端口/托盘/取消/停止独立。
- [ ] AC4/R2：断开/重试后旧事件或旧启动返回不覆盖新会话。
- [ ] AC5/R3：单条停止、全部停止、退出、GUI 崩溃、IPC/心跳失联进入所属资源清理；Worker 故障不影响普通代理。
- [ ] AC6/R3：命令行无凭据；未知版本/命令、错误对端 PID、非授权连接、重复会话、超长消息被拒绝，清理不跨其他客户端。
- [ ] AC7/R4：按配置日志可用，无跨进程轮转冲突；根/相关嵌套模块 test/vet/race、前端与 VPN.exe 构建通过。
- [ ] AC8/R1–R3：实际 UAC 同意/取消、两进程权限、网卡/路由/DNS 创建清理分别记录，未执行实网项目不宣称通过。

## 边界与待决策

不安装服务，不改协议/认证/路由范围，不新增自动重试或兜底，不涉及 Android。每次文件写入后提交，不推送远程。

待确认：当前已连接配置是否要求原地开关 TUN。建议保留“本条先断开再切换”，只解除其他 VPN 阻止提权的约束；若要求本条也不断开，则需 TUN 附着/分离设计，不能直接迁移完整引擎。确认后补齐 design.md/implement.md，并收敛最终规划。
