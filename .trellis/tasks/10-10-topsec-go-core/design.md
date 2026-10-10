# TopSAP Go 接入设计

用户已授权开始实现。范围是新增 `engine/topsec` 核心、`engine/topsec_client.go` 适配器、引擎选择以及既有 GUI 的协议选项；不引入官方 DLL、服务、MITM 或新的路由管理层。

控制通道使用独立 HTTPS client/cookie jar，经 underlay 拨号。依次获取 auth_param、一次账号密码认证及完整资源配置。U/P 为 UTF-8 Base64；HASH 是资源缓存校验，首次无缓存发送空值，强制获取完整资源。S_CODE 在本机已验证为 Base64(MD5(物理 PCI 网卡六字节按 `%03X`、连字符格式化))。Windows 按 GetAdaptersInfo 和注册表 PnPInstanceId 选择官方同序设备；缺少可验证设备时明确报错。密码拒绝保留 Ret 数字且映射现有终止重试错误。

auth_param 为带未引号标识符键的对象；只将对象键转换为 JSON，再交给标准 decoder，不执行 JavaScript。响应大小有上限，缺失 Ret 不当作成功。仅实现实际样本的普通密码、standard TLS、非压缩 IPv4 模式；其他认证或客户端安全检查明确拒绝。

网络资源只读取 res_list 中 module=na、protocol=ip 的 IPv4 地址和连续掩码；不从 netacc、com_res 或虚拟地址推导默认路由。该网关的域名/DNS 列表为空；未知的非空 DNS schema 报错，避免猜规则。DNS 地址来自 net_param；无域名资源不添加 NRPT。

数据通道使用同一网关、TLS >=1.2，保留系统证书校验。控制和数据公钥必须一致。发送已验证 CONNECT prelude，随后 16 字节大端帧头、328 字节 command 1（偏移 3=4，72 开始会话 token）、接收 command 2 ACK，再发送一次 command 7 的八零字节。command 15 承载完整 IPv4 包；不将 command 7 命名为心跳或添加未观察到的定时命令。未知帧、超长包、损坏 IPv4 明确失败。

Session 拥有取消、数据连接及 HTTP transport。关闭先中断读写，随后限时注销，回收连接；重登录新建会话和资源，复用现有 gVisor、代理、TUN、路由/DNS 所有权逻辑。

测试使用独立脱敏字面样例和本地 TLS 网关，覆盖身份字段、一次拒绝、缺失资源、分片/合并帧、TCP/UDP IPv4 包、取消/断线及多会话隔离。构建通过只证明构建，模拟通过只证明协议实现；真实 Go 登录/内网访问需用户验证。
