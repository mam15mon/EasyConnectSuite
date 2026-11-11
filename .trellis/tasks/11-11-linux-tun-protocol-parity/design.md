# Linux TUN 与三协议对齐设计

## 边界与复用

沿用现有 engine、代理、资源策略和 IPC。GUI 主程序普通权限，启用现有 Linux pkexec Worker 管理器和共享热切换流程。headless 使用显式 `-tun` 与 root 权限的本地 TUNController，复用 Linux ingress，不迁移整个跨平台 Worker 包。支持目标 Ubuntu 24.04/x86-64、systemd-resolved；其他 DNS 管理器不自动改配置或降级。

## Linux 系统资源

每个附件拥有独占非持久 TUN、已有 IPv4 资源路由及虚拟 DNS 地址。策略更新关闭旧接口并重建，避免逐项删除后误删其他所有者的同属性路由。现有前后冲突检查和进程内资源声明继续有效；不覆盖已有同前缀路由。

DNS 仅由 root 配置。专用 D-Bus 连接固定到 resolved 的 unique owner；发送前可取消，发送后保持请求和接口直到真实服务回复，取消不能冒充请求完成。先设置 DefaultRoute=false，再 DNS 和 route-only domains。服务端回复须匹配发送者/请求，不能把本地超时或总线断开当作服务完成。正常关闭删除本程序独占接口，由 resolved 随 RTM_DELLINK 清除链路；不调用 RevertLink，不修改全局 resolv.conf。未知结果保留明确错误、清理失败声明并阻止自动重接。

接口为本程序独占：不支持外部特权程序强行替换接口或强制复用索引。官方 v255 root 授权为同步，Linux 正常 ifindex 分配递增；人为强制索引复用测试不作为任意管理员干预下的安全承诺。崩溃/服务重启及断连场景分别测试和说明，不声称所有并发情形都已证明。

## headless 生命周期

保持现有代理默认行为，新增 `-tun` 与关闭代理的显式选项。权限条件在读密码前检查；TUN 的状态/错误单独输出，不将 TUN 失败误报为 VPN 登录失败。停止时报告系统资源清理错误。

本地控制器的每代 backend 在 Dial/DNS 前后验证上下文，外部调用期间不持锁，丢弃旧代结果。仅 backend 返回的 VPN 连接异步 Close，并保留 TCP CloseWrite；系统接口/路由清理仍同步确认。这样同步重登录回调内的策略更新不会等待同一 gVisor 端点锁。异步退休保留操作身份与阻塞证据。

## TopSAP Linux 身份

证据来自官方 TopSAP 3.6.3.32.2.22375 amd64：按原生顺序读取根挂载设备的 HDIO_GET_IDENTITY 串行号，或 DMI product_uuid；使用原生固定后缀计算 MD5/Base64。原始身份与摘要都不写日志或配置。不实现官方随机注册文件支路，无可用已验证硬件身份时明确失败。

普通 GUI 通过同一个可执行程序的窄范围 `--topsec-device-code` 提权辅助入口读取身份，仅通过子进程管道返回摘要，不接受用户提供的身份或任意文件路径。root/headless 直接读取。辅助入口在 GUI 初始化和普通日志初始化之前处理，调用尊重取消并记录无身份内容的阶段/耗时。不存在协议/端口/身份替代兜底。

## 验证和发布

三协议核心/适配器模拟集成、GUI 保存/热切换/失败回滚、headless 配置及状态测试；隔离网络/挂载空间和私有 D-Bus 中验证真实 TUN TCP/UDP/DNS、策略替换、代理保留、退出与崩溃清理。官方设备算法以固定哈希原生库和纯合成输入独立对照，不读取或发布真实机器身份。Windows/macOS/Android 现有检查保持通过，发布相同提交的六个产物。

研究引用：systemd v255 `src/shared/bus-polkit.c`、`src/libsystemd/sd-bus/bus-convenience.c`、`src/resolve/resolved-link-bus.c`；官方 Linux 下载 https://app.topsec.com.cn/linux/general/x86_64/deb/TopSAP-3.6.3.32.2.22375_amd64.deb 。
