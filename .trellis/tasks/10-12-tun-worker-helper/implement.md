# TUN Worker 执行与验收计划

当前 planning；PRD 已按“本条不断线热附着/分离”收敛，设计完成。用户审阅本次最终摘要后才 start；inline 主会话执行，不派发子代理，不创建子任务。阶段之间不得发布未同步的 API/Worker 版本。

## 1. TUN 后端与会话拆分 → 验证 AC3/AC9

- [ ] 建 codex/tun-worker-helper 分支，读取 prd→design→本文、research 与相关 spec/trellis-before-dev，start。
- [ ] 固化当前 TUN ACL、TCP/UDP/DNS、域名 /32、最长前缀、slot/冲突、关闭测试，先运行基线。
- [ ] Engine 保留原会话/代理，抽出 TUNBackend、TUNSpec/TUNAttachment；局部接口重构保持原 TUN 行为，复跑上述回归证明。
- [ ] 保存扩展前服务端资源快照及解析错误，构建独立 TUN view；测试代理 ZJU/custom/fake-IP 策略与实际资源分离，附着不替换原 resolver/stack。
- [ ] DNS 系统路由动作与 GUI 解析/授权拆分；GUI 管理域名绑定，Worker 不接收认证或自报 ACL。

## 2. 管道与无窗口 Worker → 验证 AC1/AC6

依赖阶段 1 的 typed snapshot/backend 契约。

- [ ] internal/tunipc 编码/帧/请求取消及限额；测试未知版本/方向/命令、超长/截断、半帧读写、背压、TCP EOF、UDP 边界。
- [ ] Windows 真实普通权限命名管道测试：唯一实例、禁止远端/错误 PID、Unicode、关闭唤醒读写、有效身份、不同会话和过期附着拒绝。
- [ ] main 最早分流 --tun-worker；不 Wails、不托盘、不读 profile；manifest asInvoker，Worker 要求提升状态。
- [ ] GUI ShellExecuteExW runas 只启动 Worker，移除两处 MessageBox 和旧重启路径；测试参数不含凭据、取消 UAC、launch/hello 失败和句柄释放。
- [ ] 单 Worker 多 ingress，使用原 slot/routeManager/tunPolicies；后台网络命令无窗口，Worker 原 system stack 权限边界保持。

## 3. Engine 热附着与数据桥 → 验证 AC3/AC4/AC9/AC10

依赖阶段 1–2；注入 Worker/controller、模拟栈，不触及真实网关。

- [ ] Engine.SetTUN 独立取消/互斥/状态，Stop 撤销附着后清理；测试 attach/detach 不调用 client.Setup、Engine.Stop、resolver.Close 或代理 listener.Close。
- [ ] 真实管道+本地 mock 实际 TCP 双向传输/半关闭、UDP 多包长度及内容、DNS/fake-IP/端口拒绝；不能只测消息 echo。
- [ ] attach/detach 时已建立 SOCKS/HTTP 会话连续传输，身份/代理端口不变，TUN 流停止但代理流保留。
- [ ] 重连更新版本，旧 DNS/旧 flow/旧更新无效；TUN 更新失败不令 VPN 重新登录，移除只关闭本条 attachment。
- [ ] attach 与 Stop/cancel/同 ID 重试、反向开关、workerlost、资源更新交错受控测试，无 sleep 判定顺序、泄漏或死锁。

## 4. App/配置/UI 集成 → 验证 AC2/AC5/AC7/AC10

依赖阶段 3 完整生命周期；不改变 UI 外观。

- [ ] Wails mode setter 改 error 返回、运行 TUN 字段/事件，保留 VPN State；删除 restarting 状态和所有“先断开全部”限制。
- [ ] 提权/IPC 不持有 a.mu；模式操作 per-profile 预约，取消/保存/回滚/清理失败准确反馈，不误写其他配置。
- [ ] 未连接偏好/准备预约，连接/托盘共用 backend，取消/错误/删除释放，最后引用后 Worker 退出；关闭到托盘保持心跳。
- [ ] 前端允许 connected 热切换，connecting/stopping 禁用，reconnecting 仅分离，单独显示 tunError；控制请求结果需捕获 ID/version。
- [ ] GUI 单写 engine.log，Worker 脱敏事件完整阶段/清理原因，无凭据/包内容，无重复广播或跨进程轮转。
- [ ] startup probe 不重启/UAC，授权 Worker 清理本程序残留；原 TopSAP 实网任务保持独立。

## 5. 失联与系统验收 → 验证 AC5/AC8

- [ ] 注入时钟的心跳/超时/parent death/EOF/数据管道失联，关闭每条流和 ingress，确认正常分离清理幂等、reservation 无泄漏。
- [ ] 使用本机模拟栈分别验实际 UAC 同意与取消、GUI/Worker PID/权限、无窗口/无 MessageBox、两条 attachment 的系统路由/DNS/网卡独立创建和清理。
- [ ] 关闭 TUN、退出、强制结束 GUI 实测 Worker 自清理；记录系统命令失败残留，不把模拟成功表述为实测。
- [ ] 真实 UAC 必须用户操作确认框；不代用户批准。实网测试如未执行，明确留下验证边界，不要求真实凭据入任务/Git。

## 6. 全量质量与交付

```text
gofmt（仅改动 Go 文件）
go test ./... -count=1 -timeout=180s
go vet ./...
go test -race ./... -count=1 -timeout=180s
node --check frontend/src/main.js
node --check frontend/src/preview.mjs
node --test frontend/src/*.test.mjs
wails build -m -nosyncgomod -trimpath
git diff --check
```

race 使用已有 MinGW GCC+CGO_ENABLED=1，不安装替代工具。PowerShell 的 Node 文件列表需显式展开。嵌套 zju-connect 若修改，单独执行相关 test/vet/race；现有 service/stack/gvisor/client/easyconnect 兼容回归一并覆盖。

- [ ] trellis-check 全范围审查 Engine/IPC/Windows/API/UI/日志及规范；异常不扩大到无关代码。
- [ ] 840×660 脱敏 preview 的开启/关闭、等待、取消、TUN 单独错误、不同配置切换和托盘状态一致；不以预览证明 UAC/系统操作。
- [ ] Wails 绑定/models/dist、preview 与签名同步；交付 build/bin/VPN.exe，Compress=false，记录基线/字节/SHA256。
- [ ] 按 trellis-update-spec 写 7 段可执行 TUN生命周期/IPC/安全/失败契约；质量通过后提交、finish-work 归档/记录，结束临时测试 Worker/标签/服务。

## 改动文件与风险检查

现有 main.go/app.go/connections.go/tun_mode_windows.go、相关测试、engine/engine.go/policy.go/tun_windows.go/tun_identity_windows.go/dns_zones.go、frontend/src/main.js/preview.mjs、生成绑定/models/dist、manifest、README/Windows TUN 说明和相关 spec。新增 engine/tun_session*.go、根 Worker 管理、internal/tunipc 与测试是跨进程契约所需，不引入通用框架。profile_logs.go 仅确有查询语义变化时改；固定 GUI日志路径保持则不改。

风险最高：热附着与 Stop/重连交错，DNS/fake-IP/ACL不越界，数据管道关闭能唤醒等待，Worker死亡后实际清理确认。每阶段有可复现断言；设计明显变化回到规划审阅，不能默默改成引擎迁移或强制重登。
