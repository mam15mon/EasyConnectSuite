# 验证记录

## 根因与防回归

现场线程证据确定 TUN 操作锁跨 DNS Dial 持有，而 Dial 的同步重连反向等待该锁。先前修复覆盖 Worker/数据流锁，但没有覆盖 Engine 的 DNS 域名发现路径。本轮测试显式让 Dial 忽略取消且等待重连/清理操作完成，用于暴露锁循环；不把网关踢线原因推测写为已验证策略。

旧实现回归失败：refresh、detach、reattach、stop 被 DNS 阻塞，cancel 后仍发布旧结果。修复后全部通过。首次附着先下发资源名称，后台发现随后更新区域；策略、backend 与 ref 必须仍相符。未新增兜底或重试，未改变认证和 IPC 协议。

## 已验证

- 根 go test ./... -count=1 -timeout=180s：203 tests，通过。
- 根 go vet ./...：通过。
- 根 go test -race ./... -count=1 -timeout=180s：203 tests，通过。已有 MinGW 与 CGO；初次设置 CC 受到 shell 工具重写影响，但 PATH 中的 GCC 可用。另用 Python 子进程显式配置 CC、CGO、PATH，对新增测试执行 -race -count=10，通过。
- wails build -m -nosyncgomod -trimpath -s -skipbindings：Windows amd64 构建通过。
- git diff --check 通过。嵌套协议模块、UI 与绑定未改动。

## 交付与未验证

桌面 VPN-DNS重连修复-1030.exe，20922368 bytes，SHA256 96FD6C333B3AE2F90DB07F557429A07178890B78DAABF36BDFE166950B899D4D。
源码 fe910c8、6cfbc35。未覆盖在用 EXE，未结束任何 VPN 进程。旧进程必须退出后启动此版本，才能应用修复。
真实网关、系统 TUN 路由与 A/B 同时登录后退出 B 的现场回归尚未执行。模拟测试通过不代表网关允许双端同时在线；现有自动重连策略保持不变。
