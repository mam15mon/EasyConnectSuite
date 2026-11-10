# Android 使用与构建

仓库包含 EasyConnect 的 Android 原型 App 和 Termux 命令行方案。Android 使用 `zju-connect` 引擎，不等同于桌面端的多协议、多连接工作台。GitHub Actions 构建调试 APK，并使用 Android 13 模拟器和模拟网关验证连接流程；真实设备、真实网关兼容性和电池续航仍需实测。

模拟测试覆盖 VPN 授权和取消、认证拒绝、TCP、UDP 与 ICMP 回显往返、接收通道断线恢复、会话过期重新认证、断开后重新连接，应用进入后台 20 秒后的转发，以及 70 秒空闲期间的心跳和空闲后的 TCP/UDP 转发。Android 原生测试还覆盖 IPv4/IPv6 接收包的拆分与合并，以及接收流在包头或包体中途断开后的处理。IPv6 包边界测试不代表已配置 IPv6 VPN 路由或支持 IPv6 网络访问。短时后台测试和模拟器 CPU 时间不能证明长期待机或实际耗电表现。

## Android App

`android-app/` 是 Kotlin + Android VpnService 工程，最低 Android API 24。界面支持服务器、账号、密码和可选 TOTP 密钥。服务建立 IPv4 默认路由并转发 IPv4 数据包，将本应用自身排除以避免连接回环；隧道断开后会重新登录，临时失败或连接频繁中断时，重试间隔从 3 秒递增到最多 300 秒；隧道持续转发至少 60 秒后才重置退避，明确的认证拒绝不会自动重试。DNS 优先使用登录结果；结果为空时使用代码中的固定地址。

从开始登录到连接和自动重试期间，应用使用前台服务通知显示当前状态，以减少切入后台后服务被系统回收的风险。各设备的后台限制与省电策略不同，长期保持连接仍需在真实设备上验证。

App 会将填写的账号、密码和 TOTP 密钥保存在应用私有的 SharedPreferences 中，当前没有额外加密；使用时请保护设备及其备份。

构建需要 Go、gomobile、Android SDK/NDK、JDK 17 和兼容 Android Gradle Plugin 8.7.3 的 Gradle；工程的 `compileSdk` 为 35。脚本同时构建 arm64 CLI 和 AAR，因此即使只需要 CLI 也需要 gomobile 和 Android 工具链。

```bash
bash build/android/build.sh
mkdir -p android-app/app/libs
cp build/android/zju-mobile.aar android-app/app/libs/zju-mobile.aar
cd android-app
gradle assembleDebug
```

Debug APK 位于 `android-app/app/build/outputs/apk/debug/`；GitHub Actions 的 Android 工作流也会保存 `android-debug-apk` 构建产物。安装后需要在系统提示中授权 VPN；实际连接和访问效果取决于设备、网关及网络环境。

## Termux 命令行方案

`build/android/build.sh` 生成 `build/android/zju-connect-android-arm64`。将其复制到手机后，可在 Termux 中运行，不需要 root：

```bash
termux-setup-storage
cp ~/storage/downloads/zju-connect-android-arm64 ~/zju
chmod +x ~/zju
~/zju --server <服务器地址> --port <端口> \
  --username <账号> --password <密码> \
  --totp-secret <TOTP密钥> --disable-zju-config
```

命令行中的密码和 TOTP 密钥可能留在 shell 历史记录中，使用时注意保护。连接后提供本地 SOCKS5（默认 `127.0.0.1:1080`）和 HTTP（默认 `127.0.0.1:1081`）代理，应用须自行使用代理；Termux 可以使用 `termux-wake-lock` 并按设备需要调整电池优化设置。
