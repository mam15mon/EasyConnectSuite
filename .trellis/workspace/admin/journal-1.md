# Journal - admin (Part 1)

> AI development session journal
> Started: 2026-10-10

---



## Session 1: Trellis 初始化与 Go UniVPN 协议核心验证
<!-- trellis-session: v=2 fp=7694da6efaba4b7b -->

**Date**: 2026-10-10
**Task**: Trellis 初始化与 Go UniVPN 协议核心验证
**Branch**: `codex/univpn-windows-core`

### Summary

安装 Trellis 0.6.17，按用户 Go 方向实现独立 UniVPN 双 TLS IPv4 核心，完成模拟 TLS、并发和 Windows 编译验证。

### Main Changes

- 项目规范、任务上下文和 inline 开发工作流已配置。
- 实现认证、地址路由、心跳、IPv4 报文与通道关闭，保留 LGPL 来源许可。

### Git Commits

| Hash | Message |
|------|---------|
| `9b74da6` | chore: 安装并初始化 Trellis Codex 工作流 |
| `8511ca4` | docs: 补全 Trellis 项目规范并保留用户开发约束 |
| `6b7dd53` | feat: 用 Go 实现 UniVPN 帧与认证资源解析 |
| `b8256a6` | feat: 增加 Go 原生 UniVPN 双 TLS 会话和 IPv4 传输 |
| `cdd247f` | test: 验证 Go UniVPN 认证、双通道、心跳和关闭行为 |
| `bc981d4` | test: 用独立线格式样例核对 UniVPN 编解码 |
| `246f360` | docs: 记录 Go UniVPN 核心许可、验证结果和后续接入边界 |

### Testing

- [OK] Go 包与全项目测试、race、vet 和 Windows build 通过；独立线格式样例通过。
- [OK] 参考原生 codec 通过；Python 43 通过 1 跳过；完整 C Windows 构建失败且不作为运行依赖。

### Status

[OK] **Completed**

### Next Steps

- 接入 GUI、配置和既有 gVisor/TUN/本地代理；获取真实网关和版本后验证兼容性。


## Session 2: UniVPN 多连接 EXE 与 UPX 交付
<!-- trellis-session: v=2 fp=ca8d980ea8fe1354 -->

**Date**: 2026-10-10
**Task**: UniVPN 多连接 EXE 与 UPX 交付
**Branch**: `codex/univpn-windows-core`

### Summary

已将 Go UniVPN 接入 GUI/gVisor/代理/TUN并隔离多配置生命周期、端口、状态与资源。根模块 race/vet、修改的依赖包及前端验证通过。两个本地 TLS UniVPN 会话并发及停止一条后另一条继续传输通过。EXE 20,516,352 字节，UPX ultra-brute/LZMA 压缩到 5,882,368 字节并通过 upx -t。保留旧客户端运行；真实网关/管理员 TUN 未验证，依赖全量测试有两项既有 Windows Unix 权限断言失败。

### Git Commits

| Hash | Message |
|------|---------|
| `eb4242e` | build: record verified UPX multi VPN executable and complete delivery checks |
| `02155ed` | docs: document multiple TUN slots and ownership based cleanup |
| `7f188fc` | fix: release TUN policy lock before waiting for network teardown |
| `21ca12f` | fix: 回收启动失败期间新资源并验证两条 UniVPN 隧道独立关闭 |
| `82d0d10` | fix: 将 GUI 连接操作绑定配置 ID 并阻止退出时新增连接 |
| `441275e` | feat: 按服务器配置管理独立并发连接和全部关闭 |

### Status

[OK] **Completed**


## Session 3: UniVPN 忽略证书开关与未压缩 EXE
<!-- trellis-session: v=2 fp=6add90b740cd4e95 -->

**Date**: 2026-10-10
**Task**: UniVPN 忽略证书开关与未压缩 EXE
**Branch**: `codex/univpn-windows-core`

### Summary

按用户明确要求增加默认关闭的 UniVPN 每配置证书校验忽略开关。保留双 TLS 通道公钥一致性检查，无自动兜底。默认拒绝不受信任证书、显式允许登录、身份不匹配拒绝、配置持久化与独立传递测试通过；根模块 race/vet 和 Wails/Vite 构建通过。按最新要求未运行 UPX，交付 EasyConnectGUI-multi-vpn-cert.exe，20,516,864 字节。真实网关登录未验证。

### Git Commits

| Hash | Message |
|------|---------|
| `df22f80` | feat: add explicit per profile UniVPN certificate verification bypass |
| `95aef5b` | test: cover opt in certificate skip, secure defaults and channel identity |
| `ef71fa7` | build: regenerate certificate option bindings and desktop frontend |

### Status

[OK] **Completed**


## Session 4: UniVPN 认证拒绝返回码诊断
<!-- trellis-session: v=2 fp=678f8ee9645388aa -->

**Date**: 2026-10-10
**Task**: UniVPN 认证拒绝返回码诊断
**Branch**: `codex/univpn-windows-core`

### Summary

用户确认官方 10781.16.0.0729 同账号可登录，设置无认证域。修正所有非零状态被误报密码错误的问题，保留十进制/十六进制返回码与typed cause，仍一次拒绝终止。根模块 race/vet 和未压缩 EXE 构建通过，交付 EasyConnectGUI-multi-vpn-authdiag.exe。读取私有配置仅输出匹配布尔与字段名，未主动登录。真实协议兼容性仍未解决，下一步需要用户用诊断版发起一次连接并提供返回码。

### Git Commits

| Hash | Message |
|------|---------|
| `90d360d` | fix: preserve UniVPN authentication status without assuming bad password |

### Status

[OK] **Completed**


## Session 5: UniVPN MAC 报文对齐与认证日志
<!-- trellis-session: v=2 fp=321a24d78cfa5931 -->

**Date**: 2026-10-10
**Task**: UniVPN MAC 报文对齐与认证日志
**Branch**: `codex/univpn-windows-core`

### Summary

Actual diagnostic result is AUTH code 2; no verified semantic mapping. Observed official MAC format matches pinned working Python prototype, so align Go stable MAC and independently validate complete AUTH frames. Persist numeric rejection codes without credentials. Root race tests, vet and uncompressed Wails build pass. Artifact EasyConnectGUI-multi-vpn-authfix.exe is 20519424 bytes; SHA256 030142FD02B0EDCF5D53EC689134938FCB66D8F32B8709D96CC2E9171BCF586D. No real login attempted; user retry required to establish live compatibility.

### Git Commits

| Hash | Message |
|------|---------|
| `c0b3414` | fix: align UniVPN MAC TLV with observed format and persist rejection codes |
| `68780e4` | test: include terminating NUL in escaped password vector length |
| `e47d38c` | 移除 UniVPN MAC 格式调整后未使用的导入 |
| `3f60862` | 记录 UniVPN 认证报文差异、日志验证及未压缩交付结果 |
| `63e87ae` | 保留 UniVPN Python MAC 算法来源与 MIT 许可说明 |

### Status

[OK] **Completed**


## Session 6: TopSAP Go 协议接入与未压缩测试版
<!-- trellis-session: v=2 fp=5091b69c3fe0e07a -->

**Date**: 2026-10-10
**Task**: TopSAP Go 协议接入与未压缩测试版
**Branch**: `codex/univpn-windows-core`

### Summary

完成 TopSAP 标准模式 Go 认证、资源和 IPv4 数据通道，接入三协议会话管理。真实样本解析及当前原生设备标识对照通过；模拟 TCP/UDP、证书、拒绝、取消、并发隔离、Go test/vet/race 和最终未压缩 Wails 构建通过。交付包 20633088 字节；实际 Go 网关登录及系统 TUN 共存待用户验证，任务保持进行中。

### Git Commits

| Hash | Message |
|------|---------|
| `760f782` | 实现 TopSAP Go 认证资源与 IPv4 TLS 数据通道 |
| `412247b` | 接入 TopSAP 多会话引擎与 GUI 协议选择 |
| `69c3d10` | 对齐 TopSAP 原生设备查询的跨网卡缓冲区字节数 |
| `811c1d5` | build: 更新包含 TopSAP 证书选项的前端资源 |

### Status

[OK] **Completed**

### Next Steps

- 用户使用新版 EXE 验证真实 TopSAP 登录、内网访问及系统 TUN 共存


## Session 7: 修复 TopSAP 改密权限误判
<!-- trellis-session: v=2 fp=7f0fc702de83a485 -->

**Date**: 2026-10-10
**Task**: 修复 TopSAP 改密权限误判
**Branch**: `codex/univpn-windows-core`

### Summary

对照成功官方抓包、资源缓存及本机门户脚本，确认 change_pswd=on 是允许改密而非强制改密。将字段补入成功模拟响应先复现失败，删除错误拦截及无用字段后全量 Go 测试/vet 通过，生成未压缩 topsec-fix1.exe。真实登录和内网访问仍待用户验证。

### Git Commits

| Hash | Message |
|------|---------|
| `a65d322` | test: 用允许改密的成功报文复现 TopSAP 登录误判 |
| `e59960e` | fix: 不再将 TopSAP 改密权限当作强制改密条件 |

### Status

[OK] **Completed**

### Next Steps

- 用户退出旧版并使用 topsec-fix1.exe 重新连接，核查真实登录及内网访问


## Session 8: 修复 TopSAP 资源请求并补齐诊断日志
<!-- trellis-session: v=2 fp=5f290a7709233045 -->

**Date**: 2026-10-11
**Task**: 修复 TopSAP 资源请求并补齐诊断日志
**Branch**: `codex/univpn-windows-core`

### Summary

用户 fix1 认证成功后缺少资源；对照官方成功样本和原生请求构造发现 TYPE=phone 仅取联系信息，改为 TYPE=all。补齐会话编号、HTTP/Ret/资源、数据TLS/ACK、首包和清理日志，并验证成功、拒绝和缺失资源时落盘且无凭据令牌。全量 test/vet/race 与未压缩 fix2 打包通过。实际网关资源返回及内网访问待用户验证。

### Git Commits

| Hash | Message |
|------|---------|
| `75c104d` | test: 复现 TopSAP 联系信息响应缺少完整资源的问题 |
| `4060e7f` | fix: 按原生逻辑请求 TopSAP 完整配置并记录认证资源阶段 |
| `3bc80fd` | feat: 补齐 TopSAP 分阶段结果失败原因和隧道清理日志 |
| `b6350a4` | test: 验证 TopSAP 完整配置请求及脱敏阶段日志落盘 |
| `3a0c647` | test: 验证缺失资源错误和注销记录持久化且不泄露凭据 |

### Status

[OK] **Completed**

### Next Steps

- 用户使用 topsec-fix2.exe 重连后，直接查看 engine.log 确认完整资源和隧道建立


## Session 9: 多VPN双栏工作台与稳定性日志交付
<!-- trellis-session: v=2 fp=4cbbbc263da98493 -->

**Date**: 2026-10-11
**Task**: 多VPN双栏工作台与稳定性日志交付
**Branch**: `codex/univpn-windows-core`

### Summary

用户确认 fix4 TopSAP 内网访问成功后，完成左侧独立连接列表与右侧资源/设置/日志页签。补全带 PID/engine/profile/session 的生命周期、停止来源、重连原因、逐项清理日志；TopSAP 每分钟记录累计/增量/最后收发年龄，空闲不误报。Windows 低功耗通知只记录，实际休眠未测试。日志8MiB轮转保留.old，当前配置最新200条显式ID读取并防异步错位。根模块test/vet/race、嵌套gVisor/service test/vet、前端语法和4项过滤测试、CUA脱敏默认/840x660交互、Wails未压缩生产构建通过。交付workspace-v2 20685312字节，SHA256 4F11AC1174AB978DAEF77F34E865CE782B5F704A4343E19C50190F9D1895E83C。未操作真实会话；长期稳定性、真实UDP和多厂商TUN共存继续由TopSAP任务保留。

### Git Commits

| Hash | Message |
|------|---------|
| `db6cf49` | fix: use TopSAP full configuration virtual IP for data traffic |
| `dc6cee0` | ui: redesign workspace with persistent connection sidebar and focused detail tabs |
| `badcee7` | ui: bind independent sidebar controls and scoped log tabs with keyboard navigation |
| `7b8459e` | 构建双栏多VPN工作台及完整诊断日志交付资源 |
| `a8a7bb1` | 避免连接日志上下文被当作格式字符串解析 |
| `f715d62` | 记录双栏界面与稳定性诊断验收及未压缩交付版本 |

### Status

[OK] **Completed**


## Session 10: 多VPN托盘独立操作与v3交付
<!-- trellis-session: v=2 fp=6d8f757457dc75b1 -->

**Date**: 2026-10-11
**Task**: 多VPN托盘独立操作与v3交付
**Branch**: `codex/univpn-windows-core`

### Summary

托盘改为全部VPN状态汇总及每配置子菜单，已连接勾选不依赖工作台选择；连接/重试/取消/断开捕获profile ID，仅查看详情切换选择。全部断开和退出复用既有清理，图标数据加锁并在所属线程销毁窗口，UTF16长名称不拆emoji。取消根据本次启动上下文识别，失败显示MessageDialog，操作记入日志。6项新增回归覆盖原生隐藏菜单结构、状态启禁、空配置、Unicode、选择/顺序改变后独立停止、未知ID与取消后校验错误。根Go test/vet/race和Wails Windows生产构建通过，无真实认证或原生桌面点击。交付workspace-v3 20695040字节，Compress=false，SHA256 204E636CDE286C3F00E275673170213245D085A020C3634787383E309F6A467C。TopSAP长期实际测试任务保留。

### Git Commits

| Hash | Message |
|------|---------|
| `90dfb50` | 托盘汇总全部VPN状态并绑定独立配置操作 |
| `34c5a10` | 实现逐条VPN托盘菜单并同步提示更新与退出 |
| `471a300` | 验证原生多VPN托盘菜单与固定ID会话隔离 |
| `82225f4` | 按本次连接上下文识别托盘取消避免误报失败 |
| `835fe0a` | 覆盖托盘空配置与取消后配置校验错误 |
| `14f1b6b` | 记录多VPN托盘验收与v3未压缩交付 |

### Status

[OK] **Completed**
