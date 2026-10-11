# 2026-10-12 验收证据

## 来源与基线

同一会话的主分支验收，基线 c5f630ee3013c09a8f7afe9668ea5d2607a3f705。以下仅为合成数据，不含真实账户、网关配置、设备标识或日志。临时 overlay 不作为实施依赖；按这里的步骤固化回归即可。

## E1：旧启动返回覆盖新会话

使用 multiTestApp / fakeVPNEngine。启动旧 a，在注入 start 中先调用 OnStatus(PhaseError)，用 channel 暂停返回；再启动同 ID 的新 a，并报告 PhaseConnected，IP=192.0.2.22；释放旧请求返回 old startup returned late。

断言 GetStatus 与 GetConnections[0] 的 State/ClientIP 一致，实际失败输出：

```text
selected state=error ip=, actual connection state=connected ip=192.0.2.22
```

证据强度：App 层确定性测试已复现；不代表真实网关已触发。实施应增加资源和完成等待断言。

## E2：资源选择队列错配

Node vm 加载 frontend/src/main.js 中原始 selectProfile 与 refreshResources 函数；用假 API 和可释放 Promise 延迟 SetActiveProfile(a) 完成。调用 selectProfile(a)，再 selectProfile(b)，此时后端仍选 a；调用 refreshResources(false)。

```text
Before: frontend=b, backend=a, selectionPending=true,
        resourceCacheB=[{target:a-resources}], rendered=[{target:a-resources}]
After releasing selection queue: frontend=b, backend=b, rendered=[{target:b-resources}]
```

断言队列完成前不得缓存/展示 A 资源到 B，结果失败。证据强度：原始业务函数与受控 API 已复现；浏览器常规即时预览未复现延迟窗口。

## E3：日志目录创建失败改变其他选择

t.Setenv(APPDATA, t.TempDir())，选择 a；在临时 APPDATA 下创建名为 EasyConnectVPN 的普通文件。调用 startConnectionID(b)，注入 start 若被调用则直接令测试失败，证明错误发生在引擎启动之前。

```text
selected config=a, GetStatus belongs to b
```

证据强度：App 层确定性文件系统错误已复现。实施增加 A 运行且具有资源、失败配置本身选中等对照用例。

## 基线通过项目

- 根 go test -race ./... -count=1 -timeout=120s；本机 CGO=1 与已有 MinGW GCC。
- 嵌套 service / stack/gvisor / client/easyconnect 的 test -race 与 vet；根 vet。
- main.js 语法及资源过滤 4 项测试。
- Wails 未压缩 Windows 构建及生成资源一致性。
- 脱敏浏览器正常选择、日志、未连接详情清空、第三条模拟会话独立连接与断开；控制台无 warn/error。
- 验收 EXE 大小 20,695,552 字节，SHA-256 14EE2AB53F862BB093420B6889EC1ECCAED8BF21B57EDEA9C8BD0CC17DB7162E。此为修复前构建，不能作为修复完成的产物。

## 尚未证明

前端收到旧 StartProfileVPN Promise 字符串后，是否覆盖托盘发起的新会话状态，是代码中识别的关联风险，实施先补复现。真实网关、真实 UDP、长期运行、休眠恢复和多厂商 TUN 共存未在此次验收执行。
