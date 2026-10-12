# TUN 热附着 / 分离与独立提权 Worker 设计

## 1. 范围与架构

用户明确要求当前 VPN 不断线地切换 TUN。采用单任务顺序交付，内部各阶段有独立测试，但不能独立发布：Engine 生命周期、IPC 和 GUI 状态须同时兼容，故不拆父子任务。

```text
普通权限 VPN.exe（Wails GUI）
  App -> Engine（认证/协议会话、gVisor、resolver、SOCKS5/HTTP）
           └ TUN 会话桥接：原会话 Dial / DNS / 授权资源视图
  WorkerManager -> 命名控制管道（附着、分离、更新、心跳、日志）
                     └ 按流命名数据管道（TCP 字节 / UDP 数据报）
系统 UAC -> 高权限 VPN.exe --tun-worker
  Worker -> 多个 tunIngress（Wintun/system stack、路由、NRPT、动态 /32）
          -> 各流经 IPC 回到对应 GUI Engine
```

Worker 不启动 VPN 引擎，不持有 VPN 凭据，不绑定对外代理端口。GUI 不执行提升自身权限的重启链。Worker 的 system stack 继续处理其已存在的防火墙配置需求，避免普通 GUI 再调用需要提权的 stack.Start。

## 2. 最小代码边界

- Engine 拆出可独立附着的 TUN 句柄和 TUN 会话后端接口。引擎启动/登录/代理监听不迁移；只将原 startTUNSlot 调用替换成控制器附着，Stop 纳入 TUN 分离。
- tunIngress 不直接依赖 GUI 私有 connectionPolicy 指针；改为版本化网络快照和只提供 Dial/DNS 的后端接口。现有路由管理、slot、冲突登记和 system stack 留在 Worker。
- App 增加一个 WorkerManager，统一按钮、已保存 TUN 配置连接及托盘连接；UAC、握手、IPC等待和网络清理均不持有 a.mu。
- 新增 internal/tunipc 放共享消息/帧/Windows 管道实现，Engine 和根包引用同一契约。没有通用 RPC 框架或外部服务。
- UI 只开放已连接时的 TUN 开关及其实际附着状态/错误，不改代理开关约束或页面外观。

## 3. 拟定接口与状态

```go
// engine: Options 增加 TUNController；控制器只负责系统 ingress。
type TUNController interface {
    Attach(context.Context, TUNSpec, TUNBackend) (TUNAttachment, error)
}
type TUNAttachment interface {
    Update(context.Context, TUNSpec) error
    Close(context.Context) error
}
// TUNBackend 由 GUI Engine 实现，Worker 端使用 IPC 代理。
// Dial 返回与原 VPN 相连的流；DNS 返回应答和允许新增的动态路由。
// 具体类型共用 internal/tunipc，避免调用点自行解析 map[string]any。
func (e *Engine) SetTUN(ctx context.Context, enabled bool) error
func (e *Engine) TUNState() TUNStatus

// root/Wails: 删除原 restarting bool，调用完成或返回错误。
func (a *App) SetProfileTUNMode(id string, enabled bool) error
func (a *App) SetTUNMode(enabled bool) error
```

保留基本 vpnEngine 的 Stop/Resources 接口，为 TUN 使用窄的可选能力接口并显式拒绝不支持的测试对象，不以重启或停止替代。运行状态增加 tunState=off/attaching/on/detaching/error、tunError，tunMode 表示实际附着；已停止配置仍展示保存的 tunMode 偏好。保持 VPN State/Running/IP/Duration/资源独立；TUN 出错不能把 VPN 状态改为 error。

每个连接有 sessionId；每次附着有 attachmentId，重连策略有递增 policyVersion。保持此前 c 实例检查，并扩展至附着操作。Engine 持有独立附着操作互斥与取消上下文；Stop 先撤销新附着资格、取消操作并等待分离，再走现有 VPN 清理。回调/网络等待禁止持有 Engine 状态锁或 a.mu。

## 4. TUN 与代理资源策略隔离

loadPolicy 目前在 conf.TUNMode 时禁用 ZJU 扩展；不能热修改 conf 再替换代理 policy，否则破坏本次要求。读取服务端资源时保留扩展前的不可变原始快照和明确的资源读取错误；TUN view 由该快照及现有合法自定义资源构建，排除自动 ZJU 补充项。附着验证原有“未返回可用资源”和禁止默认路由规则。

TUN view 使用原 GUI 栈及原 resolver/fake-IP 注册表；不得因开关 TUN 重建/关闭 resolver、修改 SetupIPPool 或替换代理 dialer。DNS 沿用原解析器，但 GUI 根据 TUN view 过滤授权域名/端口绑定，不能把代理侧 ZJU 扩展绑定当作 TUN 授权。GUI 在附着会话内维护 DNS 解析产生的 IP→授权绑定，不信任 Worker 自报 ACL/AppID。

DNS 请求经 IPC 到 GUI，现有 answerDNS 的非系统部分迁至 TUN backend；GUI 返回 DNS wire bytes 与已验证的动态 /32。Worker 先按原冲突检查添加路由，再发送 DNS 应答；失败返回 SERVFAIL。若原服务端前缀已覆盖该地址，不添加 /32，保持更具体外部路由优先。域名假 IP 和 TCPPrefL3/AppID 上下文仍由 GUI 原 resolver/栈提供。

TUN Dial 始终走强制 VPN 的独立 tunDialer，按当前附着绑定和端口/协议 ACL 校验；拒绝资源外目标，不走 DIRECT。代理仍走原 dialer。

真正的 VPN 重连由现有机制执行，建立 next policy 时同步构建新 TUN view。更新只推送网络资源快照；DNS discovery 在读循环恢复后执行，避免重连回调内查询死锁。TUN 路由/冲突/IPC 更新失败只拆除该 TUN、上报 tunError，不能使已经恢复的 VPN 重复认证。不同 policyVersion 的旧 DNS、流和更新结果不能添加到新策略。

## 5. IPC 契约与身份

GUI 预创建随机实例名的本地命名管道作为 server，FIRST_PIPE_INSTANCE、PIPE_REJECT_REMOTE_CLIENTS、显式 DACL（GUI 身份、管理员/系统的必要权限）。通过 ShellExecuteExW runas 启动同一绝对路径的 --tun-worker；SW_HIDE，NO_UI、NOCLOSEPROCESS，保留进程句柄。

CLI 参数仅有管道实例名和父 PID，不含配置文件、VPN 凭据或私密握手 token。GUI 验证实际 client PID 是本次启动句柄对应 Worker；Worker 验证 server PID 为父 GUI，保留父进程句柄、核对创建时间/映像及自身提升状态。跨账户管理员 UAC 能连接，但任意管理员客户端不能替代实际 Worker。认证失败前不得执行系统操作或泄漏策略。

控制管道：4 字节长度 + UTF-8 JSON，最大 1 MiB；公共头包含 version=1、requestId、profileId、sessionId、attachmentId、policyVersion，消息体使用封闭类型。命令为 hello/ping/prepare/attach/update/detach/shutdown；Worker 请求 GUI 的 dial/dns、事件 progress/tun-state/log 各有方向约束。未知命令/版本、超限/截断/错误 ID 明确拒绝。快照传 slot、IPv4 前缀、DNS namespace，无任意接口名、执行命令或路径。

TCP/UDP 不混入控制管道。每个授权 dial 建立短期数据管道，名称由 GUI 生成并与全部身份字段关联，双方再次验证 PID；仅允许 IPv4 TCP/UDP 和合法端口。TCP 分帧传字节、双向 EOF、cancel，支持背压和半关闭；UDP 独立长度帧保留数据报边界，单包上限 65,535，禁止把 UDP 当字节流。DNS wire 单次不超过 65,535。数据帧上限 64 KiB（UDP 受单包上限），同时活动流上限 1,024/GUI，超限明确失败，不无界 goroutine/队列。

控制 reader 分发后不阻塞心跳或执行长操作；writer 序列化、有界队列。流量只阻塞自身数据管道。dial 15 秒、DNS 10 秒、网络命令沿用 30 秒；控制请求支持取消，detach 收到后先阻止新流并取消请求。3 秒双向心跳，15 秒无有效心跳/父进程退出/控制 EOF 均进入清理，计时通过可注入时钟测试。超时不自动重启 Worker；睡眠恢复触发此失败时在 GUI 明确展示。

## 6. 操作事务与 Worker 生命周期

连接稳定时允许附着/分离；connecting/stopping 禁用开关，reconnecting 只允许移除现存 TUN。操作以捕获 ID 注册进行中标记，禁止本条删除/编辑冲突，不全局锁住其他连接。

开启：保存模式意图（沿用保存失败不启动的约束）→预约/启动并握手 Worker→取得最新会话与策略→附着→公布实际状态。取消 UAC/失败/停止胜出后恢复该配置原意图，撤销本操作创建的 ingress 和预约；回滚保存失败一起返回，不影响其他配置更新。UAC 等待不得占用 GUI 主循环或 App 锁。

关闭：保存关闭意图→撤销本条新流权限→Worker 幂等分离并等待确认→实际状态 off。失败则回滚意图并呈现实际 error/残留状态；不能偷偷重新附着来伪造回滚。没有通过清理确认前不显示完全关闭。对实际系统状态和保存偏好分别记录，部分失败不声称原子事务。

未连接开启时 Worker 只有显式准备预约，不创建空网卡；后续 StartProfileVPN 完成原会话登录后消费预约附着。保存为开启的配置从按钮或托盘连接也走同一准备入口。用户关闭该模式、删除该配置、取消启动、失败或退出时释放预约。无附着/预约后 Worker 清理退出，非长期服务。一个附着关闭不会退出仍承载其他附着的 Worker。

GUI shutdown 先取消附着操作、请求全部 TUN 分离并等待结果，再退出/停止 Engine；Worker 自身在 parent death/EOF/超时重复同一幂等清理。Worker 强杀不能保证 defer；下次授权前 probe + Worker 仅清理确认为本程序所属的残留。普通 GUI startup 不自动 UAC、不重启，残留提示不阻止代理连接。系统清理不删除其他客户端或管理员手工配置的路由。

## 7. 日志、兼容与回滚

VPN 日志仍为 GUI 唯一 engine.log 写入/轮转。Worker 不调用 engine.Start/startLogCapture，标准日志仅通过专用脱敏控制事件传 GUI；GUI 添加 PID、profile/session/attachment、阶段、耗时、计数及错误后写现有日志链，GetProfileLogs 的固定路径/200 条语义保持。GUI 普通日志记录关键请求/完成，即使 Worker 失联仍有证据。Worker panic stderr 使用其本账户固定 APPDATA 下独立 tun-worker-crash.log，不能共享 GUI crash.log；不接受 GUI 指定任意高权限输出路径。

不改持久化 profile.tunMode 字段。新增运行字段、Wails API签名与绑定/预览/dist 同步交付；旧 --tun-restart 模式明确拒绝或删除分支，不回到 GUI。主 manifest 明确 asInvoker；正常双击不提升 GUI，用户自行用管理员方式启动不作为程序主动提权验收。

可逆提交顺序：先行为保持的 TUN backend 拆分和测试，再 IPC/Worker，再生命周期/UI。若回滚必须同步 Engine/IPC/API/绑定/dist，不留下引用缺失。每次修改命令后提交；不混入其他任务或远程推送。
