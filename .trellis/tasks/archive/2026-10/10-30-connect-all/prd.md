# 一键连接全部 VPN

用户希望点击一次即可启动多条 VPN。左侧增加“连接全部”，面向全部已保存配置，不受列表筛选影响。跳过已运行、连接中与处理中项，失败独立显示，重复点击不重复启动或断开其他连接。

范围：frontend/src/main.js 操作及按钮、connection-state.test.mjs 并发测试、前端规范。复用 StartProfileVPN/SetProfileTUNMode，不改认证、IPC、后端协议或配置格式。启动前 TUN 准备改用已有每配置 pendingActions 防重入，编辑与模式操作同时遵循该状态。准备期间暂禁断开全部，避免未注册的连接随后启动。

验收：多个 TUN 配置可同时发起；重复点击、现有连接、单项失败不干扰其他配置；Node 测试与前端构建通过；Windows 打包；840x660 演示预览验证。
