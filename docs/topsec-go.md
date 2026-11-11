# TopSAP Go 接入与测试

TopSAP 标准模式已接入 Windows、macOS 和 Linux GUI/headless。协议核心在 `engine/topsec`，适配器在 `engine/topsec_client.go`；认证、资源获取和数据通道均由 Go 实现，不调用官方客户端、DLL、Python、Docker 或 MITM。每条配置独立管理连接、代理端口、TUN 槽位和停止流程。

本轮依据是本机 TopSAP 3.6.6.18.64.1 的成功登录和内网访问样本。已确认 HTTPS 认证字段、cookie、资源缓存 HASH、Windows PCI 网卡设备标识及 IPv4 数据分帧。Go 的设备字段与本机官方服务当前计算结果一致，Go 解析器能读取实际 auth_param 和资源缓存。私有样本、凭据和设备标识均在仓库外。

## 使用测试版

1. 在官方 TopSAP 中断开连接，并从托盘退出旧版 EasyConnect Suite，避免账号会话和程序单实例冲突。
2. 启动 `build/bin/VPN.exe`。当前双栏界面、托盘与稳定性日志说明见 [multi-vpn-workspace.md](multi-vpn-workspace.md)。
3. 新建配置，选择「天融信 TopSAP（标准模式）」，填写原网关地址、实际端口、账号和密码。本轮网关使用自定义端口，请按官方配置填写。
4. 如果网关证书名称或证书链不受信任，可按用户要求手动开启「忽略服务器证书校验」。默认关闭，仅作用于当前配置；开启后无法验证网关身份。认证与数据通道仍须使用相同公钥。
5. 连接后检查资源列表和内网访问。TUN 使用服务器返回的 IPv4 前缀，并保留该配置的本地 SOCKS5/HTTP 代理；其他配置可继续单独连接、断开。

## 支持边界

- 支持账号密码、standard TLS、非压缩 IPv4 L3、`na`/`ip` 网络资源和连续 IPv4 掩码。
- Windows 使用已验证的 PCI 网卡设备标识路径；未实现原生硬盘标识路径或无 PCI 网卡的其他标识方式。macOS 和 Linux 设备标识依据各自官方客户端实现，真实网关尚待验证；Linux 证据及权限要求见下文。
- 未实现验证码、短信、浏览器认证、国密、客户端证书、端点安全检查、其他网络资源或非空域名/DNS 配置格式。遇到这些模式明确报错，避免猜参数或扩大路由。
- 网关未返回完整资源列表时停止启动；不从虚拟地址推导资源，也不添加默认路由。
- 认证被明确拒绝时保留服务器 Ret 数字并停止重试，不将任意错误解释成密码错误。
- command 7 只按成功样本发送一次八零字节；未确认它是心跳，不添加猜测的定时命令。

## 验证情况

| 验证 | 结果与范围 |
| --- | --- |
| 独立脱敏向量 | 认证编码、单/双网卡标识、ACK、IPv4 UDP、损坏和截断报文通过 |
| 本机私有样本对照 | 设备字段与当前官方计算一致；真实认证参数、两条资源及 CONNECT 常量解析通过 |
| 本地 TLS 模拟网关 | 成功、一次拒绝、缺失资源、取消、损坏帧、幂等注销和会话隔离通过 |
| 改密权限回归 | 成功响应含 `change_pswd=on` 时继续获取资源和建立隧道；此字段是允许改密，不是强制改密 |
| 完整资源请求回归 | 对照原生代码改用 `TYPE=all`；模拟 `TYPE=phone` 只返回联系信息，先复现缺失资源再修复 |
| 数据虚拟 IP 回归 | 数据 IP/DNS 取自完整配置 net_param；认证与配置返回不同 IP 时，gVisor TCP/UDP 模拟往返仍通过；缺失/无效 IP 明确停止 |
| 持久化诊断 | 成功、认证拒绝及缺失资源失败的阶段、HTTP、Ret、资源数量和注销结果落盘通过；日志不含凭据、令牌或报文 |
| gVisor 集成 | 实际 TCP 握手及 TCP/UDP 应用数据往返通过；没有安装系统 TUN |
| 证书策略 | 默认不发送凭据给不受信任证书；手动忽略可连接；公钥不一致仍拒绝 |
| 三协议管理 | 模拟 EasyConnect/UniVPN/TopSAP 并发，停止 TopSAP 保留另外两条会话及独立端口/槽位 |
| Go 检查 | 根模块 `go test ./...`、`go vet ./...`、`go test -race ./...` 通过 |
| 历史 Windows 交付包 | workspace-v4 前端与 windows/amd64 生产构建通过，Compress=false；文件 20,695,552 字节 |
| 实际 Go EXE | 用户确认 fix4 登录和内网访问成功，日志出现双向数据；新版长期稳定性、真实 UDP 及系统 TUN 共存仍待验证 |

当前构建命令：`wails build -m -nosyncgomod -trimpath`，输出 `build/bin/VPN.exe`，不使用 UPX。历史交付大小、哈希及 UI 验收见 [multi-vpn-workspace.md](multi-vpn-workspace.md)。

历史 workspace-v4 交付文件 SHA-256：`8F0FE78754989EC427853D6804B0DECDC0E2A06C8583E3AEFF6609CCF74AABB5`。

日志位置：`%APPDATA%/VPN/logs/engine.log`，8 MiB 轮转并保留 `.old`。TopSAP 行包含 PID、配置 ID、进程内会话编号、认证/完整配置/数据 TLS/隧道 ACK 阶段、HTTP 状态和耗时、Ret、资源数量；首次收发、每分钟收发累计/增量与最后活动时间、关闭字节统计可确认隧道是否有实际数据。失败阶段及注销结果同样落盘。密码、用户名、S_CODE、cookie、会话令牌和数据包内容不写入日志。

## Linux 设备身份与验证边界

当前 Linux 目标是 Ubuntu 24.04/x86-64。普通用户启动 GUI 时，TopSAP 身份读取通过 `pkexec` 启动本程序的 `--topsec-device-code` 专用入口，需要可用的 polkit 授权代理；headless 的 TopSAP 模式要求通过 `sudo` 以 root 运行，包括仅代理模式。此入口不接受额外参数或任意硬件路径。权限只用于读取设备身份，摘要从匿名管道返回，不保存到配置或日志。root 进程直接读取。

研究来源是[官方 TopSAP 3.6.3.32.2.22375 amd64 包](https://app.topsec.com.cn/linux/general/x86_64/deb/TopSAP-3.6.3.32.2.22375_amd64.deb)，没有安装或重分发其程序。可核对标识：

| 对象 | SHA-256 |
| --- | --- |
| `TopSAP-3.6.3.32.2.22375_amd64.deb` | `b80632d99c9ae1a1283a03f0c6379285e652e1aec016788661d4d270eb705ba1` |
| `opt/apps/cn.com.topvpn/files/libtopsec_vpn.so` | `4bad4c46933551d9e8e132821779b3b77230ea5306f4ee713dd40849d83dd981` |

该库的 `FeatureCodeHelper::getDiskId`（ELF 地址 `0xb061c`）读取 `/etc/mtab` 的根挂载，删除设备名尾部数字后执行 `HDIO_GET_IDENTITY`。从 512 字节结果的偏移 20 取 NUL 结束字符串并移除 ASCII 空格；不进行 MAC 或 machine-id 替代。磁盘读取失败时，`createOrGetUUid`（`0xb116e`）先读取 `/sys/class/dmi/id/product_uuid` 并去除末尾换行。官方随后存在随机注册文件支路，本项目没有实现：两个已验证硬件来源均不可用时明确失败。NVMe 分区名等仍保留官方尾数处理的局限，磁盘路径无法读取时进入已有 DMI 顺序，不猜测额外设备映射。

`getFeatureCodes`（`0xb0f06`）追加固定后缀 `lUoYuAncHeNtIaNkAijWn` 并计算 MD5。原生 `GetParamForAuthRequest` 在 `0xfa870` 起将 32 位十六进制摘要转换为 16 字节，再 Base64 和 URL 编码，写入 S_CODE。Go 直接对同一 16 字节摘要执行 Base64，再由表单编码处理转义。

`build/linux/topsec-device-reference-test.sh` 下载并校验上述哈希，只提取该库到临时目录。独立 C++ 测试通过原生完整身份函数及原生摘要转换函数处理两种合成输入，检查磁盘优先顺序、DMI 分支和以下固定输出；拦截硬件调用，并在原生计算期间用 seccomp 禁止真实文件打开、命令执行和网络连接。随后运行 Go 同向量测试。该脚本不读取真实机器身份，也不将官方二进制放入交付产物。

| 合成输入 | S_CODE |
| --- | --- |
| 磁盘字符串 ` a b c `，有效 DMI 同时存在 | `9ZgAH2EHpqCdTiEgY662wA==` |
| 磁盘 ioctl 失败，DMI `00000000-1111-2222-3333-444444444444` | `0kHjlf0il4PC1hgCAOqddA==` |

取消时同时关闭私有管道并尝试结束辅助进程。等待 polkit 授权期间，pkexec 尚未读取 stdin，必须终止仍保留调用者真实 UID 的进程；完成 UID 切换后，辅助入口通过 EOF 退出。已经运行的辅助入口也在父进程退出后响应 EOF，避免硬件读取阻塞牵住调用者。该 UID 行为可对照 [polkit 125 pkexec 源码](https://github.com/polkit-org/polkit/blob/125/src/programs/pkexec.c)和 [Linux kill 权限规则](https://man7.org/linux/man-pages/man2/kill.2.html)。日志中的 `wait_privileged_device_identity` 包含授权与身份读取等待，不代表授权已获批准。

Linux Go 回归覆盖读取顺序、字面向量、未知硬件明确失败、日志脱敏、辅助进程输出校验、授权等待取消、读取等待取消和已启动辅助入口的父进程死亡清理。跨平台适配器继续运行模拟网关 TCP/UDP 与三协议隔离检查；它们不证明 Linux/macOS 已通过真实网关认证，也不证明所有实体硬件都可读取上述身份。

独立原生对照需在 Linux x86-64 准备 C++ 编译器、Go 和上文官方 Debian 包，然后运行 `bash build/linux/topsec-device-reference-test.sh /path/to/TopSAP-3.6.3.32.2.22375_amd64.deb`。脚本先验证包与库的固定哈希，再以纯合成输入执行原生函数；不安装官方客户端，也不读取真实硬件身份。GitHub 托管 Runner 连接官方下载站曾超时，因此发布 CI 保留 Go 向量与 race，原生对照在具备该包的独立 Linux 环境执行，具体结果和环境记录在发布验收中。
