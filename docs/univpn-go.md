# UniVPN 接入与多 VPN EXE

UniVPN Go 核心已接入 GUI、gVisor TCP/UDP、本地 SOCKS5/HTTP 代理及 Windows 资源 TUN，编译进同一个 EXE。可同时运行多个 EasyConnect、UniVPN 配置；选择配置不会停止已有连接。用户确认使用账号密码及官方 UniVPN 10781.16.0.0729，官方客户端可正常登录；不能据模拟测试推断所有固件兼容。

## 使用

从托盘退出旧客户端后启动当前未压缩版本 `EasyConnectGUI-multi-vpn-authfix.exe`。新建配置时选择 UniVPN，填写服务器、端口、账号、密码及可选认证域。认证域按网关配置填写，允许为空。每个配置保存独立的 SOCKS5/HTTP 端口，新配置自动选择尚未被其他配置使用的端口对；实际绑定失败会显示连接错误。

本次证书选项版本交付未压缩的 `EasyConnectGUI-multi-vpn-cert.exe`。若网关报 `x509: certificate signed by unknown authority`，断开该配置后编辑，勾选“忽略服务器证书校验（不安全）”、保存再连接。开关默认关闭，旧配置也保持关闭，仅对当前 UniVPN 配置生效。开启后跳过证书信任链、有效期和服务器名称验证，无法验证网关身份；账号密码可能被冒充的网关获取。两条 TLS 通道的证书公钥仍必须一致。程序不在证书错误后自动重试关闭校验，也不修改 Windows 证书存储。

认证诊断版本为未压缩的 `EasyConnectGUI-multi-vpn-authdiag.exe`，保留证书开关和多连接功能。它修正了把所有非零认证结果说成“密码错误”的提示，改为显示服务器原始返回码（十进制／十六进制）。官方 UniVPN 10781.16.0.0729 能正常登录而本程序拒绝认证的情况仍在排查；没有验证过的返回码含义不会猜测。认证拒绝仍立即停止，不自动切换认证域或重复尝试报文。该版本完成的是诊断信息修复，尚未证明真实网关兼容性问题已解决。

实际诊断界面返回 `2 / 0x00000002`，此前 engine.log 只记录到认证阶段，未保存这个拒绝码。官方成功日志的认证域与网关一致，MAC 字段为 14 位大写 `XXXX-XXXX-XXXX`；Go 原来采用 17 位冒号格式。当前 `authfix` 版本按同源工作 Python 原型修正稳定 MAC 的生成与编码，并在 `%APPDATA%/EasyConnectVPN/logs/engine.log` 保存数字拒绝码，不记录账号、密码、令牌或认证报文。返回码 2 的含义没有确认，真实登录仍需用新版本验证，没有自动尝试其他 MAC、认证域或协议。

该版本为 20,519,424 字节，SHA256 `030142FD02B0EDCF5D53EC689134938FCB66D8F32B8709D96CC2E9171BCF586D`。完整 AUTH 字段的独立字面量样例、TLS 模拟认证、失败日志持久化、`go test -race ./... -count=1 -timeout=75s`、`go vet ./...` 和 Wails 生产构建通过；Wails 显示 Compress=false，未运行 UPX。

连接第一个配置后，选择第二个配置并连接。连接列表显示每条会话的协议和状态；主界面状态与资源属于当前选中的配置。支持逐条断开和“断开全部”，程序退出关闭所有会话。正在运行或停止中的配置不能编辑或删除。

使用系统流量分流时开启 TUN，需要管理员权限。已有其他连接时先断开全部再切换权限，避免管理员重启打断会话。不同会话使用独立虚拟网卡、地址、路由所有权和 NRPT 标记。同一程序内完全相同的路由前缀，以及父子关系或相同名称的 DNS 命名空间会报冲突。不同长度前缀仍使用最长匹配；若两条 VPN 需要访问相同目的地址，可通过各自本地代理选择会话。

## 已实现

- 两条 TLS 连接：控制通道完成 FirstConnect、账号密码认证和地址请求，数据通道发送完整 IPv4 报文。
- 默认使用系统信任库验证证书；用户可以按配置明确选择忽略证书校验。无论开关状态，两条通道的证书公钥必须一致，发送数据通道令牌前完成检查。
- 认证字段转义、32 字节令牌、分配 IPv4 地址及 IPv4 路由解析；严格校验帧、TLV 和报文长度。
- 控制通道每 10 秒发送心跳，数据通道每 15 秒发送心跳；数据和心跳写入使用同一把锁。
- 第一次认证拒绝返回 `ErrLoginRejected`，核心不重复登录。控制通道失败或调用方取消时关闭两条通道，并解除阻塞的数据读取。

`Session.Read/Write` 接收或返回完整 IPv4 报文，适配器复用项目已有的 gVisor TCP/UDP 栈和重连流程。第一次明确认证拒绝即停止登录或重连；临时网络失败等待网络恢复，手动停止取消正在认证或重连的连接。每条连接持有自己的上下文、清理回调、代理监听器与活动连接，停止一条不会执行另一条的清理。进程共享日志捕获在其他引擎停止后仍可继续写入。

## 验证结果（Windows，2026-10-10）

| 检查 | 结果 |
| --- | --- |
| `go test -race ./... -count=1 -timeout=75s` | 通过，包含应用、引擎及协议核心，使用临时 CGO/MinGW 环境 |
| `go vet ./...` | 通过 |
| `go build ./...` | Windows 编译通过 |
| `go test ./engine/univpn -run TestWireReferenceVectors -count=1` | 独立线格式样例通过 |
| 本地 TLS 模拟网关 + gVisor | TCP/UDP 载荷往返通过；两个独立 UniVPN 会话并发，停止第一条后第二条继续传输 |
| 多配置管理与代理 | 独立状态、端口、错误隔离、取消启动、逐条停止和关闭活动连接通过 |
| TUN 内存替身 | 独立标识、相同前缀/DNS 冲突、更具体路由和多网卡遗留清理通过 |
| 修改涉及的 zju-connect 包测试及 vet | 通过；EasyConnect、service、gVisor 的 race 测试通过 |
| zju-connect 全模块测试 | 两项既有 Unix 权限断言在 Windows 失败：keylog/PCAP 要求 `0600`；其余通过 |
| 前端构建和资源过滤测试 | 通过 |
| Wails 生产 EXE 构建 | 通过，原版 20,516,352 字节 |
| UPX ultra-brute/LZMA 与 `upx -t` | 通过，压缩版 5,882,368 字节，为原版 28.67% |

测试覆盖分段帧读取、认证转义、错误密码只连接一次、地址和路由、两条 TLS 通道身份绑定、不受信证书拒绝、IPv4 报文往返、两种心跳、控制连接丢失、连接期间与会话期间取消，以及损坏的帧、TLV、路由和数据报文。

协议参考的原生 codec 测试通过；上游 Python 测试 44 项中 43 项通过，1 项 Unix 文件权限测试在 Windows 跳过。完整 C/OpenConnect Windows 构建未通过，遇到 TLS 声明／结构字段相关编译错误；该构建不作为 Go 核心的运行依赖。

## 协议来源与许可

- [Bruce-lhq/univpn-connect](https://github.com/Bruce-lhq/univpn-connect/tree/037acad1b5101c756f5583a3aa8effaa260a81db)，固定版本 `037acad1b5101c756f5583a3aa8effaa260a81db`。
- [已观察协议](https://github.com/Bruce-lhq/univpn-connect/blob/037acad1b5101c756f5583a3aa8effaa260a81db/docs/protocol.md)与该版本的 OpenConnect C 补丁。
- [上游验证范围](https://github.com/Bruce-lhq/univpn-connect/blob/037acad1b5101c756f5583a3aa8effaa260a81db/docs/validation.md)：实验性支持，不能推断所有 UniVPN 固件兼容。
- OpenConnect 参考版本 `70d1e79d1e55849dfc71dcc199b1edb535b547e4`，源码归档 SHA256 `c193f6f0f197bf4cdcc15d74c8803f9073406ddcd2f4d4ce8455ee1f76dcb72f` 已核对。
- Go 移植包保留 LGPL-2.1-or-later 许可和来源说明，见 `engine/univpn/LICENSE` 与 `engine/univpn/NOTICE.md`。上游 Python 管理层的 MIT 许可不能用于替换协议核心许可。

## 验证边界

已只读核对用户提供的网关配置、官方版本及成功认证日志的非敏感格式信息；Go 客户端尚未验证真实登录成功。TUN 测试使用内存替身，不创建真实网卡或修改本机网络；管理员权限下的真实 TUN、真实网关以及其他 VPN 客户端共存仍需现场验证。

未实现其他厂商协议、MFA、浏览器认证、IPv6、DTLS、协议压缩或固件自动协商。UPX 只压缩 EXE 文件。地址／路由 TLV 不提供已验证的 DNS、域名资源信息，因此不推测或创建 UniVPN 的 Windows NRPT 规则。当前也没有额外的 UniVPN DNS 配置功能；EasyConnect 原有域名资源与分域 NRPT 保持支持。

## Trellis

已安装 `@mindfoldhq/trellis` 0.6.17 并初始化项目 Codex 配置、技能、项目规范及任务记录。后端规范来自本项目实际代码，任务执行采用 inline 模式。CLI 和任务上下文命令已验证；安装项目文件不代表全局 Codex 自动 hook 已启用。
