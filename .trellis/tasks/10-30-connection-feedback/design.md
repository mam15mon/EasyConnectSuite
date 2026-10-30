# 设计边界

app.go 状态新增 stageDetail 与 stageStartedAt，engine/engine.go 在现有重连阻塞步骤前上报状态，复用既有 operation 日志。connections.go 初始准备状态补时间戳。保留 Stage=重连中供 TUN/托盘现有判断。

前端 presentation 纯函数计算显示词和耗时；TUN 准备使用按 profile 保存的本地开始时间，不改变实际 running。日志入口捕获 profile，切换后校验仍选中再激活日志；复制再次按 ID 获取日志并校验选择版本。诊断摘要使用允许字段/阶段白名单，完全不输出原始日志、身份、地址、任意 message/cause。

修改对应测试、演示数据、样式和生成绑定/前端产物；不新增重试、超时或自动修复，不扩大到批量摘要与常用连接。
