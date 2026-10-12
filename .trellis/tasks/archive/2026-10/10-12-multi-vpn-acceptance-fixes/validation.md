# 最终验收记录（2026-10-12）

任务 R1–R3 与 AC1–AC7 全部通过。用户已授权完成、合回本地 main、安全删除本次修复分支，并统一交付文件名为 VPN.exe。远程推送不在本次范围内。

## 修复与回归证据

| 标准 | 已验证结果 |
| --- | --- |
| AC1 | channel 控制旧启动先报错、新同 ID 会话连接成功、旧错误晚返回；列表/详情/IP/资源保持新会话，旧 ready 关闭，新会话可停止 |
| AC2 | 原始 toggleConnection/receiveStatus 函数与受控 Promise 复现托盘式新成功后旧字符串覆盖；修复后读取当前快照，新成功保留，实际当前失败仍可见 |
| AC3 | 选择队列仍在 A 时刷新 B、A→B→A、响应逆序、断开事件及资源更新事件抢先，均不串资源或覆盖较新数据 |
| AC4 | 显式 ID 查询不改变选择，返回切片副本；无会话/断开/错误/停止中为空，未知 ID 明确失败且前端不调用旧查询兜底；预览契约一致 |
| AC5 | 临时 APPDATA 普通文件阻挡目录创建；非所选 B 失败保留运行 A 的状态/资源，B 无残留且可重试；所选配置失败清空旧资源 |
| AC6 | 首批确定性回归修复前 3 项 Go、2 项 Node 失败；修复后新回归及现有连接、取消、托盘、相关嵌套模块检查全部通过 |
| AC7 | 生成 Wails 绑定/dist 同步；Windows amd64 未压缩构建通过，840×660 脱敏浏览器验收通过，交付大小/哈希见下文 |

先固化复现提交 1657c85，再修复 e1f6481；扩展资源及事件交错断言提交 9c3973e，绑定/dist 提交 1e7c548，契约规范提交 9577354，默认交付文件名提交 f0ce8e7。新测试位于 connections_test.go 和 frontend/src/connection-state.test.mjs；不依赖仓库外临时 overlay。

## 自动检查

以下命令退出成功：

```text
go test . -count=1 -timeout=120s
go test -race ./... -count=1 -timeout=120s
go vet ./...
node --check frontend/src/main.js
node --check frontend/src/preview.mjs
node --test frontend/src/connection-state.test.mjs frontend/src/resource-filter.test.mjs
git diff --check
```

Node 合计 13 项通过（状态归属 9 项、已有过滤 4 项）。根 race 覆盖 App、engine、TopSAP、UniVPN 四个包。竞态环境仅使用已有 MinGW GCC，CGO_ENABLED=1；没有安装新工具。

嵌套 zju-connect 模块单独执行并通过：

```text
go test -race ./service ./stack/gvisor ./client/easyconnect -count=1 -timeout=120s
go vet ./service ./stack/gvisor ./client/easyconnect
```

trellis-check 复核：只修改会话结果发布、目录失败回滚、显式资源 API 与调用、回归及交付命名；未改协议核心、认证、路由、配置格式或 UI 外观。无新依赖、兜底、自动重试或通用框架。后端/绑定/生产 UI/预览的参数、空结果、未知 ID 错误一致，旧 GetVPNResources 保持兼容。相关规范已补齐可执行跨层契约。

## 脱敏界面验收

显式 ?preview 模式，840×660：地址点击、名称/键盘 Enter、卡片空白选择正确；两条模拟运行配置的名称/IP/DNS/代理端口/资源分别一致，运行记录含对应配置 ID。第三条模拟配置启动完成、断开、再次启动并取消，前两条始终运行；未连接/错误配置显示空 IP/DNS/资源。运行中筛选仅含运行项，资源刷新正确，计时持续更新，资源区可滚动操作。浏览器 console warn/error 为空。

慢请求及事件时序由自动受控测试证明，不以立即完成的预览 API 替代。验收标签已关闭、窗口尺寸已恢复、临时 Vite 服务已停止。预览截图已移出交付目录，仅留在本机临时目录。

## 交付与清理

源码/默认命名基线 f0ce8e7。执行 `wails build -m -nosyncgomod -trimpath` 成功，production windows/amd64，Compress=false；自动生成内容与 1e7c548 无差异。

- 文件：build/bin/VPN.exe
- 大小：20,698,624 字节
- SHA-256：3C759BAC1AFE0FDB59524773DE564CE82DC0B680D3C124FD14A01A524B99A63A
- 原 acceptance-fixes EXE 和最终 VPN.exe 内容哈希相同。
- 清理 18 个旧 EasyConnectGUI*.exe 与 3 张交付目录预览图；保留 univpn-notices、univpn-connect-source。未删除用户配置、凭据或日志。
- README/default outputfilename 统一到 VPN.exe；当前交付保持未压缩。

## 验证边界

本次完成 App/UI 业务归属、模拟协议兼容回归与 Windows 构建。未执行真实网关、真实 UDP、长期运行、实际休眠恢复或多厂商系统 TUN 共存测试；这些继续保留在 10-10-topsec-go-core，不因本任务通过而归档。测试/任务资料仅含临时目录与脱敏示例。
