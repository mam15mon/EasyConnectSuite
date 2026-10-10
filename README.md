# EasyConnect Suite

深信服 EasyConnect VPN 的第三方图形客户端。本项目由 [EasyConnectGUI](../EasyConnectGUI)(Wails 壳 + 内嵌 zju-connect.exe 子进程)与 [zju-connect](https://github.com/mythologyli/zju-connect)(上游 commit `4031c52`)合并而成:**引擎源码直接编译进 GUI 进程**,不再嵌入、不再启动子进程。

## 目录结构

```
EasyConnectSuite/
├── main.go            Wails 应用入口(不变)
├── app.go             GUI 逻辑:配置/Profile/状态,直接驱动 engine(不再 exec 子进程)
├── tray_windows.go    托盘(不变)
├── engine/            引擎适配层:把上游 main.go 改写为可启停、带回调的库
├── zju-connect/       上游源码 fork(vendor),含以下修改:
│   ├── stack/gvisor/stack.go        ★ EOF 不再 panic,改为自动重登 + 重建隧道
│   ├── internal/hook_func/          ★ 增加 ResetTerminalFuncs(支持进程内重启)
│   ├── lifecycle/                   ★ 公开 hook_func 给外部模块使用
│   └── service/                     ★ bind 失败由 panic/Fatal 改为日志+返回
└── frontend/          前端(与原 GUI 完全相同,未改动)
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
wails build        # 产物: build/bin/EasyConnectGUI.exe(单文件)
go build ./...     # 仅编译验证
```

生成 UPX 高压缩版本（需安装 UPX）：

```powershell
wails build -o EasyConnectGUI-resume.exe
upx --ultra-brute --lzma -o build/bin/EasyConnectGUI-upx.exe build/bin/EasyConnectGUI-resume.exe
upx -t build/bin/EasyConnectGUI-upx.exe
```

压缩版可直接运行，功能与未压缩版本相同；`--ultra-brute` 会尝试更多压缩组合，构建耗时更长。

`vpn_profiles.json` 配置与原 EasyConnectGUI 完全兼容,可直接复用。

## Windows TUN 模式

GUI 支持按服务器资源分流的 IPv4 TCP/UDP TUN，同时保留本地 SOCKS5/HTTP 代理。相同前缀使用较低 metric，不改变其他客户端更具体路由的优先级。域名解析使用限定到 VPN 资源的 Windows NRPT；重登录会更新资源，正常断开会清理本程序创建的路由和 DNS 规则。强制结束后的残留会在下次启动时检查和清理，发现残留且权限不足时请求管理员授权。

配置、共存及验证方法见 [Windows TUN 模式](docs/windows-tun.md)。
