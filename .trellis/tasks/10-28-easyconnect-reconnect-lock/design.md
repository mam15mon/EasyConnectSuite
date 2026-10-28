# 设计

最小行为差距位于 tun_worker_windows.go：remoteTUN.Update 在重连调用栈同步关闭流，gVisor 端点锁可能已被该调用栈持有；tunManager.handle 跨 Dial/DNS 持有 attachment 读锁。

仅修改该文件、邻近回归测试和 TUN 规范。Update 在锁内更新版本并取出旧流，锁外由独立 goroutine 关闭旧流，使恢复能够返回并释放端点锁。使用现有 operation observer 保留关闭阶段、身份、耗时及阻塞证据。handle 只在读取身份与发布结果时持锁，网络调用后验证原始 ref、取消及 context；过期连接关闭，DNS 数据/路由不发布。finish 在锁内取消并取出流，锁外关闭。

不更改协议、重试上限、前端及超时，不添加自动降级。真实双机网关行为单独验证。
