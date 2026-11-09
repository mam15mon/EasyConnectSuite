# Android 使用与构建

仓库包含 EasyConnect 的 Android 原型 App 和 Termux 命令行方案。Android 使用 `zju-connect` 引擎，不等同于桌面端的多协议、多连接工作台。GitHub Actions 构建调试 APK，并使用 Android 13 模拟器和模拟网关验证连接流程；真实设备、真实网关兼容性和电池续航仍需实测。

## Android App

`android-app/` 是 Kotlin + Android VpnService 工程，最低 Android API 24。界面支持服务器、账号、密码和可选 TOTP 密钥。服务建立 IPv4 默认路由，将本应用自身排除以避免连接回环；隧道断开后会重新登录，临时失败重试间隔从 3 秒递增到最多 300 秒，明确的认证拒绝不会自动重试。DNS 优先使用登录结果；结果为空时使用代码中的固定地址。

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
