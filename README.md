# VPN

支持深信服 EasyConnect、华为／联软 UniVPN 及天融信 TopSAP 标准模式协议子集的第三方桌面图形客户端。一个 EXE 可同时连接多个配置，分别管理连接状态、资源、本地代理和 TUN。EasyConnect 引擎基于 [zju-connect](https://github.com/mythologyli/zju-connect)(上游 commit `4031c52`)，UniVPN 和 TopSAP 使用 Go 协议核心；**引擎源码直接编译进 GUI 进程**，无需 OpenConnect 或 Python 子进程。支持范围见 [UniVPN 接入说明](docs/univpn-go.md)及 [TopSAP 测试版说明](docs/topsec-go.md)；TopSAP 的新版 EXE 仍待真实网关验证。

macOS 支持 EasyConnect、UniVPN 的代理模式和按 VPN 资源分流的 IPv4 TCP/UDP TUN。开启 TUN 时会请求管理员授权，主程序无需提权；域名资源使用系统 resolver。GitHub Actions 在 Intel 和 Apple Silicon 测试真实 utun、系统 DNS、多连接与模拟后端转发，并构建 `.app`。真实网关连接和管理员授权弹窗交互尚未验证，CI 成功不代表已验证真实连接。菜单栏支持查看各连接、连接或断开、打开工作台和退出；TopSAP 的 macOS 设备标识依据官方客户端静态分析实现，真实网关仍待验证。配置和主程序日志位于 `~/Library/Application Support/VPN`；TUN Worker 的管理员诊断日志位于 `/private/var/log/EasyConnectSuite/tun-worker-crash.log`。菜单栏成功启动后，关闭窗口会隐藏工作台并保持连接，使用菜单栏的退出操作可断开连接并退出；菜单栏和窗口的实际交互尚未在实体 Mac 验证。Actions 产物为保留可执行权限的 `.tar.gz`，应用尚未签名或公证。

## 文档导航

- [Windows TUN 使用、共存与排查](docs/windows-tun.md)
- [UniVPN 协议范围与验证记录](docs/univpn-go.md)
- [TopSAP 协议范围与验证记录](docs/topsec-go.md)
- [多连接工作台与日志](docs/multi-vpn-workspace.md)
- [Android App 与 Termux 构建](docs/android.md)
- [构建资源说明](build/README.md)

## 目录结构

本地 VPN 核心模块名为 `EasyConnectSuite/vpncore`，代码导入统一使用该路径，例如 `EasyConnectSuite/vpncore/client/easyconnect`。根模块通过 `replace EasyConnectSuite/vpncore => ./zju-connect` 使用本地源码；`zju-connect/` 保留为来源目录名，原有许可证与来源说明继续保留。

```
EasyConnectSuite/
├── main.go            Wails 应用入口、权限及遗留规则清理
├── app.go             GUI 配置/Profile/状态
├── connections.go     按配置隔离的连接管理
├── tray_windows.go    托盘与多连接状态
├── engine/            引擎适配层:把上游 main.go 改写为可启停、带回调的库
│   ├── univpn/        Go 原生 UniVPN TLS/IPv4 协议核心
│   └── topsec/        Go 原生 TopSAP 标准模式 TLS/IPv4 协议核心
├── zju-connect/       上游源码 fork(vendor),含以下修改:
│   ├── stack/gvisor/stack.go        ★ EOF 不再 panic,改为自动重登 + 重建隧道
│   ├── internal/hook_func/          ★ 增加 ResetTerminalFuncs(支持进程内重启)
│   ├── lifecycle/                   ★ 公开 hook_func 给外部模块使用
│   └── service/                     ★ bind 失败由 panic/Fatal 改为日志+返回
└── frontend/          多配置连接列表、协议选择及资源界面
```

## 为什么原来的版本会崩溃

原版内嵌的 zju-connect.exe 在 VPN 服务器断开数据流(登录会话过期/被顶号/网络切换)时:

1. `L3Conn.Read` 返回 `EOF`,内部尝试用旧 token 重建接收流;
2. 服务器拒绝旧 token,重建失败;
3. gVisor 读循环里 `panic(err)` → **整个进程终止**(日志表现为 `panic: EOF`)。

## 本 fork 的修复

- `stack/gvisor/stack.go`:读/写错误不再 `panic`/`os.Exit`,而是走 `rebuildL3Conn`:
  自动重新登录(`Client.Setup()` 重建 token,上游本身支持重复调用)→ 重建 L3 连接 → 继续服务;
  每隔 3 秒重试；网络不可达、连接失败或网络超时会持续等待，网络恢复后自动重新登录并重建隧道；
  密码登录被服务器明确拒绝时立即停止；其他登录或握手错误最多连续重试 5 次，全部失败才通过回调通知上层优雅收尾。
  手动断开会终止重连等待。
- GUI 层:引擎事件直接映射到状态胶囊(`connecting / connected / reconnecting / disconnected / error`),
  日志面板内容由进程内日志管道转发,不再抓取子进程 stdout。
- 进程内常驻服务的 bind 失败(SOCKS5/HTTP/端口转发/Shadowsocks)不再炸进程。

## 构建

桌面构建需要 Go（版本见 `go.mod`）、Node.js 和 Wails v2.12.0 CLI；macOS 还需要 Xcode 命令行工具。Wails 根据 `wails.json` 安装并构建前端。

```powershell
wails build -m -nosyncgomod -trimpath
go build ./...     # 仅编译验证
go test ./...      # 根模块测试
```

Windows 默认产物为 `build/bin/VPN.exe`。在 macOS 上构建得到 `build/bin/VPN.app`；CI 分别构建 Intel 和 Apple Silicon，打包为 `.tar.gz` 以保留可执行权限。各平台的构建和测试结果不等同于真实网关验证。

生成 UPX 高压缩版本（需安装 UPX）：

```powershell
upx --ultra-brute --lzma build/bin/VPN.exe
upx -t build/bin/VPN.exe
```

默认生成 `build/bin/VPN.exe`，当前验收包保持未压缩。可选 UPX 命令直接压缩该文件；`--ultra-brute` 会尝试更多压缩组合，构建耗时更长。

`vpn_profiles.json` 配置与原 EasyConnectGUI 完全兼容,可直接复用。

程序文件、Windows 产品名称和窗口标题均为 `VPN`。默认配置存储在 `%APPDATA%\VPN\vpn_profiles.json`，运行日志在 `%APPDATA%\VPN\logs`，WebView 缓存在 `%APPDATA%\VPN\webview`；已有的 EXE 同目录配置继续兼容。首次启动前会将旧 `%APPDATA%\EasyConnectVPN` 目录整体改名为 `VPN`，保留配置和历史日志；新旧目录同时存在或迁移失败时明确报错，不覆盖任一目录。

窗口 X 只隐藏窗口并保留托盘和连接；托盘“退出程序（断开全部）”交给 Wails 发起真正退出。退出清理只执行一次，完成所有连接和 Worker 的清理后再移除托盘；退出期间禁止新连接和重新唤醒窗口。

## Linux 开发版

Linux x86-64 的图形版和无图形版正在适配，首发目标是 Ubuntu/Debian。图形版使用 Wails、GTK 3 和 WebKitGTK 4.1，配置位于 `${XDG_CONFIG_HOME:-$HOME/.config}/VPN`；Linux 默认使用本地代理。保存配置时会将配置文件权限限制为 `0600`。Linux 系统 TUN、路由与 DNS 接管尚未实现，不能使用桌面端 TUN 功能。

无图形版 `vpn-headless` 在前台运行单个连接，默认仅在 `127.0.0.1` 开放 SOCKS5 `1080` 和 HTTP `1081`；应用需要显式使用这些代理。可通过 `-socks-port`、`-http-port` 调整端口，`-proxy-all` 将代理请求送往 VPN，同时保留服务器资源访问限制。密码从标准输入读取，不接受作为命令行参数；输入第二行 TOTP 密钥时使用 `-totp-stdin`。例如在交互式 Bash 中：

```bash
read -rs -p '密码: ' vpn_password; echo
printf '%s\n' "$vpn_password" | ./vpn-headless -server vpn.example.com -username user -proxy-all
unset vpn_password
```

通过 Ctrl+C 或 SIGTERM 停止。支持的协议选项为 `easyconnect`、`univpn` 和 `topsec`，但 Linux TopSAP 设备身份尚未实现；Linux 真实网关兼容性尚未验证。当前模拟测试已覆盖 EasyConnect 登录、SOCKS5 TCP 数据往返和信号退出，不代表真实网关、完整桌面功能或长期稳定性已验证。正式安装包和各平台发布产物仍在准备中。

## 同时连接多个 VPN

1. 从托盘退出旧客户端，运行新版 EXE。单实例机制会阻止第二个窗口代替正在运行的旧实例。
2. 新建或编辑配置，选择 EasyConnect、UniVPN 或 TopSAP。UniVPN 填写服务器、端口、账号、密码及网关要求的认证域；认证域允许留空。TopSAP 目前支持已验证的标准模式账号密码认证，按官方配置填写实际端口。
3. 每个配置使用独立 SOCKS5/HTTP 端口。新配置自动分配未被其他配置使用的端口对，通常依次为 `1080/1081`、`1082/1083`；端口已被外部程序占用时，该连接明确报错。
4. 连接第一个配置，再选择另一个配置连接。切换配置只切换当前查看的状态和资源。可以逐条断开，也可以点击“断开全部”；退出程序关闭全部连接。

TUN 通过独立后台 Worker 申请管理员权限，主 GUI 保持普通权限且不重启。已连接配置可热开启或关闭 TUN，已有 VPN 和代理流继续运行；只显示系统 UAC，不显示额外 MessageBox。每个 TUN 使用独立网卡、虚拟 DNS 地址和 NRPT 标记；本程序内完全相同的资源路由前缀或重叠 DNS 命名空间会明确报错。不同长度前缀仍按 Windows 最长匹配选择，程序不能让同一个系统目的地址同时路由到两条隧道；有资源重叠时，可用各配置的本地代理明确选择 VPN。

## Windows TUN 模式

GUI 支持按服务器资源分流的 IPv4 TCP/UDP TUN，同时保留本地 SOCKS5/HTTP 代理。与其他客户端的相同前缀比较时使用较低 metric，不改变其他客户端更具体路由的优先级。EasyConnect 域名解析使用限定到 VPN 资源的 Windows NRPT；UniVPN 当前仅使用已验证的 IPv4 地址和路由，不推测 DNS 资源。重登录会更新资源，正常断开只清理所属连接创建的路由和 DNS 规则。GUI 退出或失联后 Worker 会清理所属 TUN；下次启动只探测残留，用户开启 TUN 时由授权 Worker 清理。

配置、共存及验证方法见 [Windows TUN 模式](docs/windows-tun.md)。
