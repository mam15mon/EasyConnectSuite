# EasyConnect Suite

支持深信服 EasyConnect 和华为／联软 UniVPN 协议子集的第三方 Windows 图形客户端。一个 EXE 可同时连接多个配置，分别管理连接状态、资源、本地代理和 TUN。EasyConnect 引擎基于 [zju-connect](https://github.com/mythologyli/zju-connect)(上游 commit `4031c52`)，UniVPN 使用 Go 协议移植；**引擎源码直接编译进 GUI 进程**，无需 OpenConnect 或 Python 子进程。UniVPN 的真实网关兼容性尚未验证，支持范围见 [UniVPN 接入说明](docs/univpn-go.md)。

## 目录结构

```
EasyConnectSuite/
├── main.go            Wails 应用入口、权限及遗留规则清理
├── app.go             GUI 配置/Profile/状态
├── connections.go     按配置隔离的连接管理
├── tray_windows.go    托盘与多连接状态
├── engine/            引擎适配层:把上游 main.go 改写为可启停、带回调的库
│   └── univpn/        Go 原生 UniVPN TLS/IPv4 协议核心
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

```
wails build -m -nosyncgomod -trimpath -o EasyConnectGUI-multi-vpn.exe
go build ./...     # 仅编译验证
```

生成 UPX 高压缩版本（需安装 UPX）：

```powershell
upx --ultra-brute --lzma -o build/bin/EasyConnectGUI-multi-vpn-upx.exe build/bin/EasyConnectGUI-multi-vpn.exe
upx -t build/bin/EasyConnectGUI-multi-vpn-upx.exe
```

压缩版可直接运行，功能与未压缩版本相同；`--ultra-brute` 会尝试更多压缩组合，构建耗时更长。

`vpn_profiles.json` 配置与原 EasyConnectGUI 完全兼容,可直接复用。

## 同时连接多个 VPN

1. 从托盘退出旧客户端，运行新版 EXE。单实例机制会阻止第二个窗口代替正在运行的旧实例。
2. 新建或编辑配置，选择 EasyConnect 或 UniVPN。UniVPN 填写服务器、端口、账号、密码及网关要求的认证域；认证域允许留空。
3. 每个配置使用独立 SOCKS5/HTTP 端口。新配置自动分配未被其他配置使用的端口对，通常依次为 `1080/1081`、`1082/1083`；端口已被外部程序占用时，该连接明确报错。
4. 连接第一个配置，再选择另一个配置连接。切换配置只切换当前查看的状态和资源。可以逐条断开，也可以点击“断开全部”；退出程序关闭全部连接。

TUN 需要管理员权限。已有其他连接时不自动重启切换权限，应先断开全部再授权，随后逐条连接。每个 TUN 使用独立网卡、虚拟 DNS 地址和 NRPT 标记；本程序内完全相同的资源路由前缀或重叠 DNS 命名空间会明确报错。不同长度前缀仍按 Windows 最长匹配选择，程序不能让同一个系统目的地址同时路由到两条隧道；有资源重叠时，可用各配置的本地代理明确选择 VPN。

## Windows TUN 模式

GUI 支持按服务器资源分流的 IPv4 TCP/UDP TUN，同时保留本地 SOCKS5/HTTP 代理。与其他客户端的相同前缀比较时使用较低 metric，不改变其他客户端更具体路由的优先级。EasyConnect 域名解析使用限定到 VPN 资源的 Windows NRPT；UniVPN 当前仅使用已验证的 IPv4 地址和路由，不推测 DNS 资源。重登录会更新资源，正常断开只清理所属连接创建的路由和 DNS 规则。强制结束后的残留会在下次启动时检查和清理，发现残留且权限不足时请求管理员授权。

配置、共存及验证方法见 [Windows TUN 模式](docs/windows-tun.md)。
