# Windows TUN 模式

在主界面“建立隧道”按钮上方切换 **TUN 模式**，设置自动保存到当前节点。旧配置默认仍使用代理模式。连接期间开关禁用，断开后才能切换。

## 使用方式

1. 从托盘退出旧的 EasyConnectGUI 实例，再运行新版本。
2. 编辑配置，选择实际联网的出口网卡，例如 `WLAN`。自动选择也可使用，但多 VPN 同时运行时建议明确指定出口。
3. 在主界面开启 TUN 模式。普通权限下会弹出 Windows 管理员授权窗口；同意后程序关闭旧实例并以管理员身份重新打开，保留配置文件路径及当前节点。取消授权则保留原窗口和原模式。已是管理员时不重启。
4. 点击“建立隧道”。保存的 TUN 配置在普通权限下连接时，也会先请求管理员授权重启；重开窗口后再次点击连接。
5. Windows 中会出现 `EasyConnect Suite` 网卡，地址为 `198.18.255.1/30`。SOCKS5 `127.0.0.1:1080` 和 HTTP `127.0.0.1:1081` 继续可用。

Wintun 由现有 sing-tun 依赖内嵌，本实现无需另外放置 `wintun.dll`。

## 与 aTrust、Clash 共存

- 仅导入 VPN 下发的 IPv4 资源范围，不自动加入整个 `10.0.0.0/8`，也不添加全局默认路由。
- EasyConnect 网卡 metric 和资源路由 metric 都设为 `0`。相同前缀与 aTrust 的总 metric `1` 比较时，EasyConnect 优先。
- 不同前缀仍遵循最长匹配。例如 EasyConnect 的 `/8` 不会覆盖 aTrust 的 `/16`。
- 不修改 aTrust 或 Clash 的网卡 metric、路由、DNS 配置。Clash 原有的 `198.19` 地址范围和 `10.0.0.0/8` 绕过保持原设置。
- 如果另一个客户端在相同前缀上也使用总 metric `0`，两者可能打平；本版本不会自动修改其他客户端。

## 域名与 DNS

为 VPN 下发的域名、固定解析名称以及节点配置中的“补充 TUN DNS 域名”添加带有 `EasyConnectSuite resource TUN` 标记的 Windows NRPT 规则，指向虚拟 DNS `198.18.255.2`。DNS 通过 TUN 内部处理，无需占用本机的 `53` 端口。

如果内网 IP 可访问，但域名报 `ERR_NAME_NOT_RESOLVED`，先比较系统解析和虚拟 DNS：

```powershell
Resolve-DnsName one.console.res.zb-chinatower.com -Type A -DnsOnly
Resolve-DnsName one.console.res.zb-chinatower.com -Type A -Server 198.18.255.2 -DnsOnly
```

虚拟 DNS 成功而系统解析失败，说明该域名没有命中现有 DNS 规则。编辑当前节点，在“补充 TUN DNS 域名”中填入该完整域名（多个域名用逗号分隔），保存后断开并重新连接。仅填写域名，不带 `https://` 或路径；该域名及其子域名会使用 VPN DNS。补充 DNS 不会增加域名访问权限或扩大 IP 资源范围，目标 IP 和端口仍需在服务器授权列表中。配置随节点保存，重登录时重新应用，断开时一起清理。

域名解析结果已经属于服务器 IP 资源范围时，保留原资源路由的前缀；仅域名资源、且没有相应 IP 资源路由时，才添加解析地址的 `/32` 路由。端口和 TCP/UDP 权限继续由 VPN 引擎校验。匹配失败不会再次直连同一个 TUN 路由。

重登录后重新获取资源、替换路由与域名解析规则。断开时关闭 TUN 和活动连接，撤销本程序创建的路由与标记规则。

本模式目前支持 IPv4 TCP/UDP。ICMP/ping 不作为连通性验证依据；IPv6 资源不经此 TUN。浏览器或应用自带的 DNS-over-HTTPS 可能绕过 Windows 域名策略，应使用系统 DNS 或显式 SOCKS5 访问内网域名。原有更具体的 NRPT 策略也可能影响 DNS 选择。

## 验证与排查

连接后在 PowerShell 中查看：

```powershell
Get-NetIPInterface -InterfaceAlias 'EasyConnect Suite' -AddressFamily IPv4
Get-NetRoute -InterfaceAlias 'EasyConnect Suite' -PolicyStore ActiveStore
Get-DnsClientNrptRule | Where-Object Comment -eq 'EasyConnectSuite resource TUN'
```

使用实际开放的业务端口验证，而不是 ping，例如 `Test-NetConnection <内网IP> -Port <业务端口>`。同时确认 aTrust 的独立资源和 Clash 的普通代理流量仍可使用。断开后再查看，确认本程序的路由和标记规则已经清理。

日志位于 `%APPDATA%\EasyConnectVPN\logs\engine.log`。初始化失败或资源路由失败会作为连接错误返回；代理端口占用会记入日志。

如果强制结束进程后遗留了本程序的 DNS 标记，可在管理员 PowerShell 中清理：

```powershell
Get-DnsClientNrptRule |
  Where-Object Comment -eq 'EasyConnectSuite resource TUN' |
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
