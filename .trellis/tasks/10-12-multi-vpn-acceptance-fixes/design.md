# 修复设计

## 基线与范围

以 c5f630e 为缺陷基线。三个问题共用“配置 ID + 会话实例 + 请求版本”的现有归属约束，作为一个任务实施，不拆父子任务。仅修改 App 会话结果发布、资源查询及相关前端调用/测试；不改 engine 协议核心。

## D1：启动完成路径遵守会话身份

在 startConnectionID 中，start(opts) 返回并重新持有 a.mu 后，继续把返回的引擎交给原 c，并保证 c.ready 在全部正常返回路径上关闭。发布状态和资源前同时检查 a.connections[p.ID] == c 与 !c.stopping。过期实例不能写 a.status/a.resources，也不能发送覆盖当前会话的状态事件。

保持原错误返回语义和当前取消清理链；不要在锁内调用可能回调 App 的 Stop。验收需覆盖旧失败返回、新会话成功及 Stop 等待完成，防止加入身份检查后出现 ready 未关闭。

前端 toggleConnection 的 actionVersions 仅识别经本页面发起的新动作，不能证明托盘发起的重试与旧 Promise 属于同一会话。实施时以受控用例核实；对后端启动返回错误，不再凭一个字符串合成可能覆盖新状态的完整 VPNStatus，应重新取得后端按 ID 的连接快照并按当前选择呈现，保留当前错误提示。避免为此新增会话版本协议或通用状态管理器。

## D2：资源查询显式绑定配置 ID

新增 Wails 方法 GetProfileVPNResources(id string) ([]engine.ResourceEntry, error)：

- 持有 a.mu，先确认 id 对应已保存配置；未知 ID 返回明确错误。
- 只读取 a.connections[id]。无会话、停止中或非运行状态返回空资源；其余返回资源切片副本。
- 不依赖也不修改 ActiveProfileID，不返回其他配置资源。
- 保留原有 GetVPNResources 的接口以兼容既有调用；新界面不再调用它。保留原接口不是失败兜底，新接口失败不退回旧接口。

refreshResources 在发起时捕获 id，调用 GetProfileVPNResources(id)，继续保留 selectionVersion、resourceRequest 与事件驱动的失效逻辑，防止返回期间发生切换/断开或有更新事件。同步 preview.mjs 和生成的 Wails 绑定，生产包必须包含新接口。

选择显式 ID 而非仅禁用刷新按钮：选择队列和托盘切换都能改变后端选中项，查询本身应具备完整归属。无需让资源读取等待保存活动选择，也不新增配置字段。

## D3：启动前失败回滚限定归属

MkdirAll 失败时删除本次登记的连接并取消其 context；以 p 构造该配置的失败回滚状态用于事件发布。只有 p.ID 等于 ActiveProfileID 时，才同步更新 a.status 并清空 a.resources。其他所选配置的详情完全保留。继续返回实际文件系统错误，不忽略或替换日志目录。

此处失败发生在 a.mu 持有期间、engine.Start 调用之前；测试必须验证引擎未启动、无虚假连接名额占用且后续停止不阻塞。

## 预计影响文件

- connections.go、connections_test.go：D1/D3 及确定性会话生命周期测试。
- app.go、app_test.go 或 connections_test.go：显式资源查询及跨配置、未知 ID、断开和副本语义测试。
- frontend/src/main.js、frontend/src/preview.mjs：查询 API 与启动结果呈现。
- frontend/src/connection-state.test.mjs（拟新增）：通过现有 Node 工具测试真实业务函数，受控延迟，不复制实现作为测试对象；必要时仅抽取被测试的这段逻辑，禁止新增通用框架。
- frontend/wailsjs/go/main/App.js、App.d.ts，以及构建确实改变的 frontend/dist 文件；models.ts 仅在生成器实际产生差异时提交。
- .trellis/spec/backend/configuration-guidelines.md、frontend/index.md：完成后记录已验证的身份/查询契约。

## 风险与兼容

- GetVPNResources 旧接口继续存在，旧配置无需迁移；新前端与绑定必须同时构建交付。
- 新增方法返回 error 时 Wails Promise 会拒绝；预览应体现相同的未知 ID 行为。
- 只加后端检查不足以证明前端不会覆盖，需覆盖返回字符串这条独立路径。
- 暂停旧启动的测试必须用 channel / Promise 控制顺序并保证释放，不能用不稳定的 sleep 判断先后。
- 先补可失败回归，再修复；若出现超出三项需求的设计问题，记录证据并另行定范围。

## 回滚

按最小逻辑批次提交。回滚 D2 时同步回滚新前端调用、绑定及构建资源，避免运行期找不到方法；D1/D3 可独立回滚，但相应验收随之失效。只使用有明确范围的 revert，不重置或覆盖他人修改。
