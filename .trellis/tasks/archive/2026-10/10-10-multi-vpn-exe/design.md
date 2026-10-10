# Design
App 按 profile ID 持有独立连接记录，保留选中配置的 StartVPN/StopVPN/GetStatus，增加 GetConnections/StopAllVPN。状态和资源事件携带 profile ID。
Engine 持有本地关闭链和 context。代理增加返回独立 closer 的启动接口，既有 CLI 保持原语义；gVisor 增加调用方停止判断；EasyConnect keepalive由自身Close停止，无额外全局注册。
UniVPN实现client.Client适配器，通过Session传输完整IPv4，更新IP后复用网络栈。没有DNS元数据则禁用自动远端DNS。
TUN slot决定独立适配器、/30地址、NRPT标签；清理只匹配本程序标识。同前缀和DNS命名空间冲突明确拒绝。
日志使用进程共享writer，多引擎结束后释放。配置新增protocol/domain和代理端口，首配置旧默认1080/1081，其他配置确定分配不同默认端口。
修改App、engine、TUN、必要的zju-connect接口、前端和生成绑定。生命周期重构通过现有回归与多实例测试验证。
