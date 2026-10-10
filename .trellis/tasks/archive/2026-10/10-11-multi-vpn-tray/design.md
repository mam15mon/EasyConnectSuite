# 改动边界

现有托盘只读取 GetStatus/当前配置，切换并连接会修改工作台选择。行为应位于托盘命令分发：一次读取全体状态快照，按配置 ID 建立各条子菜单，命令记录 ID 和动作。连接/停止调用既有 StartProfileVPN/StopProfileVPN，详情复用 SetActiveProfile 与 profile-switched 事件。断开全部和退出复用既有清理，不改变协议、认证、路由或 DNS。

tray_menu.go 只容纳托盘状态映射、汇总/提示文本和显式 ID 命令分发，供原生菜单及回归测试共用。tray_windows.go 替换单连接菜单，增加连接失败提示、UTF-16 有界文本和图标数据同步；销毁窗口消息交给创建它的托盘线程处理。app.go 的 syncTrayTooltipLocked 改用全部连接快照；停止中只在托盘快照标记，保留公开状态模型和前端 ABI。

菜单顺序：汇总、打开工作台、各配置子菜单（独立生命周期操作/查看详情）、断开全部、退出程序（断开全部）。已连接项勾选；连接中可取消，重连中可断开，断开中禁用重复动作。没有配置时显示空状态。配置名称按字面显示，菜单转义 &。

Win32 依据：AppendMenuW 的 MF_POPUP/MF_CHECKED/MF_GRAYED 与递归 DestroyMenu；NOTIFYICONDATAW.szTip 为 128 UTF-16 单元含终止符。资料：https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-appendmenuw ，https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-destroymenu ，https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-notifyicondataw 。
