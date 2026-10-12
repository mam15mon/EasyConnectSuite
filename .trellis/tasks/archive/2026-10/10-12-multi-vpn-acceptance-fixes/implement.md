# 执行与验收计划

## 当前状态

- 实施与 AC1–AC7 验收已完成，最终记录见 validation.md；待完成任务归档与本地 main 合并。
- PRD 已收敛为 R1–R3 / AC1–AC7，无阻塞产品问题。
- 当前为 Codex inline 模式，主会话直接实施和检查，不派发子代理；无需填充代理 JSONL。
- 实施前审阅最新 prd.md、design.md、本文与 research/acceptance-evidence.md。已运行 task.py start 并加载 trellis-before-dev。

## 1. 固化可重复回归

- [x] 将验收的三个 Go 问题复现转为仓库测试；t.Setenv(APPDATA, t.TempDir())，只使用模拟配置。
- [x] R1 用 channel 暂停旧启动：先 PhaseError，再完成同 ID 新会话，最后释放旧返回；断言列表/详情/资源一致，ready 等待和 Stop 不阻塞。
- [x] R3 以临时目录中的普通文件阻挡日志目录创建：选中且运行 A、启动 B 失败，断言 A 及资源不变，B 未启动；另测所选配置失败回滚和重试。
- [x] 以前端原始业务函数及受控 Promise 复现 A 的选择排队、切 B、刷新得到 A 资源；记录修复前失败。
- [x] 对 frontend/src/main.js:609–613 补过期返回与托盘式新连接事件交错测试；这项关联风险先验证再修改。
- [x] 每次写入后提交本次测试，提交消息说明这是缺陷复现；不要误报测试通过。

## 2. 修复后端状态归属

- [x] D1：为 start 返回结果发布增加当前实例检查，保持 c.engine/c.ready、错误返回及停止清理语义。
- [x] D3：日志目录错误只回滚本次配置；仅所选配置才更新全局详情并清空资源。
- [x] 对应回归由失败转通过，并复跑现有连接独立、取消启动、编辑已停止配置及托盘测试。
- [x] 若关联前端用例失败，按 D1 重新读取后端状态快照，避免旧字符串生成覆盖状态。

## 3. 资源查询端到端修复

- [x] 新增 GetProfileVPNResources(id)，覆盖：选中 A 查 B、无会话/已断开/停止中、未知 ID、返回切片副本、查询不改选择。
- [x] 前端按捕获的 ID 查询；保留既有请求版本/事件失效机制。
- [x] 同步预览 API；未知 ID 明确报错，不退回 GetVPNResources。
- [x] 扩展前端回归：A→B→A、选择等待刷新、响应逆序、断开事件在先、资源更新事件在先。
- [x] 生成绑定和前端资源后，只提交实际发生变化的产物。

## 4. 自动验证

在根目录执行（Windows PowerShell）：

```powershell
go test ./... -count=1 -timeout=120s
go vet ./...
node --check frontend/src/main.js
node --check frontend/src/preview.mjs
node --test frontend/src/resource-filter.test.mjs frontend/src/connection-state.test.mjs
```

竞态检测使用本机已存在的编译器，仅设置本次进程环境；路径变化时重新发现，不安装其他工具作为默认动作：

```powershell
$env:CGO_ENABLED='1'
$env:CC='C:\Users\admin\scoop\apps\msys2\2026-09-27\mingw64\bin\gcc.exe'
$env:PATH='C:\Users\admin\scoop\apps\msys2\2026-09-27\mingw64\bin;'+$env:PATH
go test -race ./... -count=1 -timeout=120s
```

在 zju-connect 目录执行以下兼容回归（根 ./... 不遍历嵌套模块）：

```powershell
go test -race ./service ./stack/gvisor ./client/easyconnect -count=1 -timeout=120s
go vet ./service ./stack/gvisor ./client/easyconnect
```

- [x] 上述命令退出码全部成功；只对测试失败、后续修改或未解决风险重复相关检查。
- [x] Go 修改经 gofmt；git diff --check 通过，检查 diff 仅覆盖本任务。

## 5. 构建与脱敏交互验收

```powershell
wails build -m -nosyncgomod -trimpath
Get-Item build/bin/VPN.exe | Select-Object Length
Get-FileHash build/bin/VPN.exe -Algorithm SHA256
```

- [x] 构建输出 Compress=false，绑定、前端 dist 同步；不覆盖用户运行中的已有 EXE。
- [x] 显式 ?preview 检查地址/名称/状态/空白选择、键盘焦点及定时刷新。
- [x] 两条模拟运行连接间切换：名称、IP、DNS、端口、资源、日志归属正确；第三条独立连接/取消/断开不影响前两条。
- [x] 运行中筛选、未连接/失败项清空详情、控制台无新增异常；最小窗口 840×660 可操作。
- [x] 异步故障场景以自动受控测试为验收证据；不要用立即完成的预览 API 声称已覆盖慢请求。
- [x] 记录源码提交、构建大小/哈希、模拟测试与未验证实网项目；关闭验收启动的临时服务与标签。

## 6. 收尾

- [x] 按 trellis-check 检查规范和跨层一致性；通过后记录最终验收。
- [x] 将本次已验证的实例身份与显式资源查询规则写入相关 spec。
- [x] 每次项目修改命令后提交准确文件，检查工作区无意外改动；只读或生成内容无差异时不创建空提交。
- [x] 用户要求的修复完成并满足 AC1–AC7 后再归档；保留原 TopSAP 实网任务。
- [x] 不把构建/模拟通过表述为真实网关、UDP、休眠或长期稳定性通过。

用户追加交付要求：默认构建名称已改为 VPN.exe，旧 EXE/预览图已清理；完成归档记录后合入 main，并仅以 git branch -d 删除本次已合并修复分支。
