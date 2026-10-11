# Windows TUN 模式

在主界面“建立隧道”按钮上方切换 **TUN 模式**，设置自动保存到当前节点。旧配置默认仍使用代理模式。连接期间开关禁用，断开后才能切换。

## 使用方式

1. 从托盘退出旧的 EasyConnectGUI 实例，再运行新版本。
2. 编辑配置，选择实际联网的出口网卡，例如 `WLAN`。自动选择也可使用，但多 VPN 同时运行时建议明确指定出口。
3. 在主界面开启 TUN 模式。普通权限下先显示“即将切换管理员权限”提示，说明授权后窗口会关闭并自动重新打开、配置会保留，再弹出 Windows 管理员授权窗口；同意后程序关闭旧实例并以管理员身份重新打开，保留配置文件路径及当前节点。取消授权则保留原窗口和原模式。已是管理员时不重启。
4. 点击“建立隧道”。保存的 TUN 配置在普通权限下连接时，也会先请求管理员授权重启；重开窗口后再次点击连接。
5. 第一槽位的网卡为 `EasyConnect Suite`，地址为 `198.19.128.1/30`；后续槽位使用 `EasyConnect Suite #N`、`198.19.128.(4N+1)/30`，对应 DNS 为 `198.19.128.(4N+2)`，最多 64 个独立槽位。各配置的 SOCKS5/HTTP 代理继续可用，端口以配置和界面显示为准。

可以逐条连接多个配置，切换当前节点不会断开其他连接。已有连接时应先断开全部再切换管理员权限。程序内两条 TUN 的资源前缀完全相同，或 DNS 命名空间存在相同/父子关系时会明确报错；不同长度前缀保持最长匹配。需要访问重叠地址时，可通过各连接的本地代理明确选择 VPN。

Wintun 由现有 sing-tun 依赖内嵌，本实现无需另外放置 `wintun.dll`。

连接时主界面及托盘显示实际阶段：认证、建立隧道、加载资源、创建 TUN、配置网卡、识别 DNS、配置路由、配置 DNS、启动代理。认证完成后的 TUN 配置时间不会继续显示为账号验证。启动日志以 `VPN startup [阶段]` 标记每一步，可据时间戳定位耗时。

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

任务管理器强制结束进程时无法执行退出清理，NRPT 规则可能残留。程序下次启动时会先检查并清理全部自有槽位的残留，无需登录成功或开启 TUN。清理只删除本程序的精确 NRPT 标记，以及自有网卡仍具有对应槽位预期地址时，该网卡上 metric 为 `0`、无网关、协议为 `NetMgmt` 的 IPv4 非默认路由。名称符合但地址不符的网卡不会清理。

启动检查在单实例检查之后执行；检查期间不能建立 VPN 连接。普通权限下发现残留才显示切换提示、请求管理员授权并重新打开程序，保留配置路径和当前节点；已有管理员权限时直接清理。授权取消或清理失败会在界面显示错误。程序未重新启动前，残留规则不会由已结束的进程自动清理。

本模式目前支持 IPv4 TCP/UDP。ICMP/ping 不作为连通性验证依据；IPv6 资源不经此 TUN。浏览器或应用自带的 DNS-over-HTTPS 可能绕过 Windows 域名策略，应使用系统 DNS 或显式 SOCKS5 访问内网域名。原有更具体的 NRPT 策略也可能影响 DNS 选择。

## 验证与排查

连接后在 PowerShell 中查看：

```powershell
Get-NetIPInterface -InterfaceAlias 'EasyConnect Suite*' -AddressFamily IPv4
Get-NetRoute -InterfaceAlias 'EasyConnect Suite*' -PolicyStore ActiveStore
Get-DnsClientNrptRule | Where-Object Comment -match '^EasyConnectSuite resource TUN(?: #(?:[1-9]|[1-5][0-9]|6[0-3]))?$'
```

使用实际开放的业务端口验证，而不是 ping，例如 `Test-NetConnection <内网IP> -Port <业务端口>`。同时确认 aTrust 的独立资源和 Clash 的普通代理流量仍可使用。断开后再查看，确认本程序的路由和标记规则已经清理。

日志位于 `%APPDATA%\EasyConnectVPN\logs\engine.log`，多连接共享此进程日志。初始化、资源路由失败和代理端口占用都会作为所属连接的错误返回。

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

路由、DNS 测试使用内存替身，不创建真实网卡或修改本机网络。真实连接与三客户端共存仍需在管理员权限下验证。
