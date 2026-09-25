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
  最多连续重试 5 次(间隔 3 秒),全部失败才通过回调通知上层优雅收尾。
- GUI 层:引擎事件直接映射到状态胶囊(`connecting / connected / reconnecting / disconnected / error`),
  日志面板内容由进程内日志管道转发,不再抓取子进程 stdout。
- 进程内常驻服务的 bind 失败(SOCKS5/HTTP/端口转发/Shadowsocks)不再炸进程。

## 构建

```
wails build        # 产物: build/bin/EasyConnectGUI.exe(单文件)
go build ./...     # 仅编译验证
```

`vpn_profiles.json` 配置与原 EasyConnectGUI 完全兼容,可直接复用。
