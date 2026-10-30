# 验证

- node --test frontend/src/connection-state.test.mjs：13 项通过，覆盖并行 TUN 准备、重复点击、跳过运行/连接中/处理中配置、单项 UAC 拒绝以及空列表。
- node --check frontend/src/main.js、npm run build、git diff --check 通过。
- Windows wails build -m -nosyncgomod -trimpath -s -skipbindings 通过。Go/协议/IPC/绑定未变；复用本会话已验证的重连修复。
- 840x660 浏览器 ?preview 模拟界面：按钮完整可见；点击连接全部后原 2 条在线保留，另 2 条同时认证，最终显示 4 条在线，连接全部禁用。这不是实际网关验证。
- 交付桌面 VPN-一键连接版-1030.exe，20922880 bytes，SHA256 22756B4A488BBBCEB2B370E48B078AB57973F1E12E9D4D9C730A7491098FFDDB。包含前一轮 DNS 重连死锁修复。未启动或中断真实 VPN。
- 模拟截图保存桌面，不纳入 Git。真实同时连接仍需用户运行新版验证。
