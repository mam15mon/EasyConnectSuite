# Windows TUN 模式

在主界面“建立隧道”按钮上方切换 **TUN 模式**，设置自动保存到当前节点。旧配置默认仍使用代理模式。已连接时可以直接开启或关闭 TUN；原 VPN 会话、网络栈和代理保持运行。正在初次连接或停止时不可切换。

## 使用方式

1. 从托盘退出旧的 EasyConnectGUI 实例，再运行新版本。
2. 编辑配置，选择实际联网的出口网卡，例如 `WLAN`。自动选择也可使用，但多 VPN 同时运行时建议明确指定出口。
3. 开启 TUN 时由系统 UAC 为独立无窗口 `VPN.exe --tun-worker` 授权。主 GUI 不退出、不重新登录，取消授权会还原该配置的开关。Worker 存活期间由多条 TUN 共用；最后一条 TUN 和准备预约释放后退出，下次启动仍可能弹 UAC。
4. 未连接时开启只保存偏好并准备 Worker，点击连接后附着真实网卡；已连接时直接附着或分离。TUN 错误在连接设置中单独显示，VPN 及本地代理继续运行。
5. 第一槽位的网卡为 `EasyConnect Suite`，地址为 `198.19.128.1/30`；后续槽位使用 `EasyConnect Suite #N`、`198.19.128.(4N+1)/30`，对应 DNS 为 `198.19.128.(4N+2)`，最多 64 个独立槽位。各配置的 SOCKS5/HTTP 代理继续可用，端口以配置和界面显示为准。

可以逐条连接多个配置，切换当前节点不会断开其他连接。其他配置已连接时也可为当前配置附着 TUN。程序内两条 TUN 的资源前缀完全相同，或 DNS 命名空间存在相同/父子关系时会明确报错；不同长度前缀保持最长匹配。需要访问重叠地址时，可通过各连接的本地代理明确选择 VPN。

Wintun 由现有 sing-tun 依赖内嵌，本实现无需另外放置 `wintun.dll`。

VPN 状态显示认证及代理启动阶段；TUN 分别显示附着、已附着、分离或错误。Worker 进度和日志通过命名管道转入 GUI 日志；不会把 TUN 错误变成整个 VPN 登录失败。

初次连接先识别完整 DNS 区域，再一次应用分域规则；资源路由通过一次查询当前 TUN 网卡的路由表判断已有项，避免为每条资源路由重复查询。

## 与 aTrust、Clash 共存

- 仅导入 VPN 下发的 IPv4 资源范围，不自动加入整个 `10.0.0.0/8`，也不添加全局默认路由。
- EasyConnect 网卡 metric 和资源路由 metric 都设为 `0`。相同前缀与 aTrust 的总 metric `1` 比较时，EasyConnect 优先。
- 不同前缀仍遵循最长匹配。例如 EasyConnect 的 `/8` 不会覆盖 aTrust 的 `/16`。
- 本程序虚拟地址放在 `198.19.128.0/17` 后半段：多网卡及 DNS 使用 `198.19.128.0/24` 内各自的 `/30`，内部 Fake-IP 映射池使用 `198.19.192.0/18`，两者不重叠。Clash 的 Fake-IP 池应由用户设为前半段 `198.19.0.0/17`；aTrust 的 `198.18.0.0/16` 域名映射范围与本程序分开。
- 不修改 aTrust 或 Clash 的网卡 metric、路由、DNS 配置。Clash 的 `10.0.0.0/8` 绕过保持原设置。
- `198.19.128.0/17` 是本程序的地址分配范围，不会因此向 TUN 添加整段 `/17` 路由；实际业务仍按服务器资源分流。更换程序后需从托盘退出旧实例再运行新版、重新连接，原 DNS 规则会在正常断开时清理。
- 如果另一个客户端在相同前缀上也使用总 metric `0`，两者可能打平；本版本不会自动修改其他客户端。

## 域名与 DNS

EasyConnect 连接时自动为 VPN 下发的所有域名与固定解析名称添加带有 `EasyConnectSuite resource TUN` 或 `EasyConnectSuite resource TUN #N` 标记的 Windows NRPT 规则，指向该槽位的虚拟 DNS。DNS 通过 TUN 内部处理，无需占用本机的 `53` 端口。UniVPN 当前只解析已验证的 IPv4 地址和路由，不推测其 DNS/域名资源，也不创建 UniVPN NRPT 规则。

程序还通过该 VPN 的 DNS 查询资源域名的 SOA 记录，自动识别其所属 DNS 区域，将区域及其子域名交给当前 VPN 解析。例如服务器只下发 `asc.res.zb-chinatower.com`，但 VPN DNS 返回所属区域 `res.zb-chinatower.com`，则 `one.console.res.zb-chinatower.com` 也会自动使用同一 VPN DNS，无需手动填写或修改 hosts。区域信息仅用于选择 DNS，不会增加资源访问权限。未返回有效 SOA 时不扩展对应区域；查询错误记录到日志。

程序不通过截断域名来猜测区域，也不会添加根域、公共后缀或全局 DNS 规则。只修改本程序标记的规则；aTrust 的更具体域名策略仍按 Windows 的匹配规则选择，Clash 和其他域名的原有 DNS 设置保持原样。两个 VPN 若配置了完全相同的域名，则需要明确归属，单凭域名不能同时分配给两个 DNS。

域名解析结果已经属于服务器 IP 资源范围时，保留原资源路由的前缀；仅域名资源、且没有相应 IP 资源路由时，才添加解析地址的 `/32` 路由。端口和 TCP/UDP 权限继续由 VPN 引擎校验。匹配失败不会再次直连同一个 TUN 路由。

重登录后重新获取资源、替换路由与域名解析规则，在 L3 隧道重建完成后重新查询 DNS 区域。断开时关闭 TUN 和活动连接，撤销本程序创建的路由与标记规则。

GUI 真正退出会通知 Worker 清理；关闭窗口到托盘仍保留会话和心跳。GUI 崩溃、父进程结束、管道失联或心跳超时后，Worker 清理所有所属网卡、路由和 NRPT 并退出。Worker 故障仅令 TUN 出错，不自动重新启动或重新认证 VPN。

程序启动只探测遗留配置，不弹 UAC、不重启。首次授权 Worker 清理本程序的精确 NRPT 标记及自有网卡上匹配预期地址、metric 为 0、无网关、NetMgmt 的 IPv4 非默认路由。清理失败明确返回；名称匹配而地址不符的网卡不处理。

本模式目前支持 IPv4 TCP/UDP。ICMP/ping 不作为连通性验证依据；IPv6 资源不经此 TUN。浏览器或应用自带的 DNS-over-HTTPS 可能绕过 Windows 域名策略，应使用系统 DNS 或显式 SOCKS5 访问内网域名。原有更具体的 NRPT 策略也可能影响 DNS 选择。

## 验证与排查

连接后在 PowerShell 中查看：

```powershell
Get-NetIPInterface -InterfaceAlias 'EasyConnect Suite*' -AddressFamily IPv4
Get-NetRoute -InterfaceAlias 'EasyConnect Suite*' -PolicyStore ActiveStore
Get-DnsClientNrptRule | Where-Object Comment -match '^EasyConnectSuite resource TUN(?: #(?:[1-9]|[1-5][0-9]|6[0-3]))?$'
```

使用实际开放的业务端口验证，而不是 ping，例如 `Test-NetConnection <内网IP> -Port <业务端口>`。同时确认 aTrust 的独立资源和 Clash 的普通代理流量仍可使用。断开后再查看，确认本程序的路由和标记规则已经清理。

日志位于 `%APPDATA%\EasyConnectVPN\logs\engine.log`，多连接共享此进程日志。代理端口失败作为所属 VPN 错误返回；TUN 附着和系统清理失败保留独立 TUN 错误。Worker 不直接写 engine.log，崩溃输出单独位于 tun-worker-crash.log。

如果强制结束进程后遗留了本程序的 DNS 标记，可在管理员 PowerShell 中清理：

```powershell
Get-DnsClientNrptRule |
  Where-Object Comment -match '^EasyConnectSuite resource TUN(?: #(?:[1-9]|[1-5][0-9]|6[0-3]))?$' |
  ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force }
Clear-DnsClientCache
```

## 开发验证

```powershell
go test ./...
go vet ./...
Set-Location zju-connect
go test ./service ./dial ./resolve ./stack/gvisor ./client/easyconnect
```

常规测试使用模拟栈或本地真实命名管道，不修改系统网络。显式 ECS_TUN_SYSTEM_TEST=accept/cancel/crash 才运行真实 TUN／UAC 验收；使用保留槽位 60、61 和文档地址，必须先退出正在运行的 VPN。2026-10-12 已验证普通父进程／提升 Worker、两条 TUN 的 TCP/DNS、逐条清理、拒绝 UAC 和强制结束父进程后的自清理。真实网关和三客户端共存仍是独立验证范围。
