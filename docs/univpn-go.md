# UniVPN Go 协议核心

当前 `engine/univpn` 是 Go 原生的报文传输核心，按华为／联软 UniVPN 的已观察协议子集实现。它尚未接入 GUI、gVisor、Windows TUN 或本地代理，也没有连接用户的真实网关。现有 EXE 不包含 UniVPN 支持。

## 已实现

- 两条 TLS 连接：控制通道完成 FirstConnect、账号密码认证和地址请求，数据通道发送完整 IPv4 报文。
- 默认使用系统信任库验证证书；两条通道的证书公钥必须一致，发送数据通道令牌前完成检查。
- 认证字段转义、32 字节令牌、分配 IPv4 地址及 IPv4 路由解析；严格校验帧、TLV 和报文长度。
- 控制通道每 10 秒发送心跳，数据通道每 15 秒发送心跳；数据和心跳写入使用同一把锁。
- 第一次认证拒绝返回 `ErrLoginRejected`，核心不重复登录。控制通道失败或调用方取消时关闭两条通道，并解除阻塞的数据读取。

`Session.Read/Write` 接收或返回完整 IPv4 报文，后续接入时复用项目已有的 gVisor TCP/UDP 栈。测试中的 TCP、UDP、ICMP 报文往返只证明封装传输，不等于真实应用网络访问已验证。

## 验证结果（Windows，2026-10-10）

| 检查 | 结果 |
| --- | --- |
| `go test ./engine/univpn -count=1 -timeout=45s` | 通过，本地真实 TLS 模拟网关 |
| `go test -race ./engine/univpn -count=1 -timeout=60s` | 通过，使用临时 CGO/MinGW 环境 |
| `go test ./...` | 通过，包含既有应用和 engine 测试 |
| `go vet ./...` | 通过 |
| `go build ./...` | Windows 编译通过 |
| `go test ./engine/univpn -run TestWireReferenceVectors -count=1` | 独立线格式样例通过 |

测试覆盖分段帧读取、认证转义、错误密码只连接一次、地址和路由、两条 TLS 通道身份绑定、不受信证书拒绝、IPv4 报文往返、两种心跳、控制连接丢失、连接期间与会话期间取消，以及损坏的帧、TLV、路由和数据报文。

协议参考的原生 codec 测试通过；上游 Python 测试 44 项中 43 项通过，1 项 Unix 文件权限测试在 Windows 跳过。完整 C/OpenConnect Windows 构建未通过，遇到 TLS 声明／结构字段相关编译错误；该构建不作为 Go 核心的运行依赖。

## 协议来源与许可

- [Bruce-lhq/univpn-connect](https://github.com/Bruce-lhq/univpn-connect/tree/037acad1b5101c756f5583a3aa8effaa260a81db)，固定版本 `037acad1b5101c756f5583a3aa8effaa260a81db`。
- [已观察协议](https://github.com/Bruce-lhq/univpn-connect/blob/037acad1b5101c756f5583a3aa8effaa260a81db/docs/protocol.md)与该版本的 OpenConnect C 补丁。
- [上游验证范围](https://github.com/Bruce-lhq/univpn-connect/blob/037acad1b5101c756f5583a3aa8effaa260a81db/docs/validation.md)：实验性支持，不能推断所有 UniVPN 固件兼容。
- OpenConnect 参考版本 `70d1e79d1e55849dfc71dcc199b1edb535b547e4`，源码归档 SHA256 `c193f6f0f197bf4cdcc15d74c8803f9073406ddcd2f4d4ce8455ee1f76dcb72f` 已核对。
- Go 移植包保留 LGPL-2.1-or-later 许可和来源说明，见 `engine/univpn/LICENSE` 与 `engine/univpn/NOTICE.md`。上游 Python 管理层的 MIT 许可不能用于替换协议核心许可。

## 后续接入边界

真实网关和客户端版本尚未提供，用户确认当前认证仅需账号密码。GUI 后端选择、配置保存、gVisor 接口适配、SOCKS5/HTTP 代理和 Windows TUN 仍需后续实现及验证。

此核心没有自动重认证、其他协议兜底、MFA、浏览器认证、IPv6、DTLS、压缩或固件协商。地址／路由 TLV 不提供已验证的 DNS、域名资源信息，因此这里不推测或创建 Windows NRPT 规则。

## Trellis

已安装 `@mindfoldhq/trellis` 0.6.17 并初始化项目 Codex 配置、技能、项目规范及任务记录。后端规范来自本项目实际代码，任务执行采用 inline 模式。CLI 和任务上下文命令已验证；安装项目文件不代表全局 Codex 自动 hook 已启用。
