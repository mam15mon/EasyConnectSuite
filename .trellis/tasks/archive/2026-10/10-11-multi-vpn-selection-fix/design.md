# 最小实现

列表点击委托给固定的 connection-list 容器。独立 toggle 按钮走 toggleConnection(id)，其他行内点击走既有 selectProfile(id)；保留名称按钮的键盘可访问性。继续复用序列化选择队列、selectionVersion 和当前 ID 状态/资源/日志守卫。

renderConnections 按 profile ID 重用行及按钮，仅更新文本、状态、禁用属性与选中样式。只有新增/删除/筛选/排序时变更行节点，不在每次状态 tick 中 replaceChildren 或重写 innerHTML。新增行的模板为静态结构，用户值使用 textContent/属性。

不改变协议连接或后台接口，不添加另一路选择状态/猜测其他配置。CSS 仅让整行显示可点击鼠标光标。
