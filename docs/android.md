# 在安卓设备上使用(Termux 方案)

把引擎编译成 android/arm64 二进制,在 [Termux](https://f-droid.org/packages/com.termux/) 里运行,
手机上即获得 SOCKS5(`127.0.0.1:1080`)与 HTTP(`127.0.0.1:1081`)代理,内含桌面版全部修复
(TOTP 自动提交、8 小时会话到期自动重登)。**不需要 root。**

## 构建

```
bash build/android/build.sh        # 或在 Windows 上用 Git Bash 运行
```

## 安装到手机

1. 手机安装 Termux(建议 F-Droid 版,Play 商店版已停更)
2. 把 `zju-connect-android-arm64` 传到手机(微信/网盘/USB 均可,放到 Download 目录)
3. 在 Termux 里执行:

```
termux-setup-storage
cp ~/storage/downloads/zju-connect-android-arm64 ~/zju
chmod +x ~/zju
```

## 运行

```
~/zju --server <服务器地址> --port <端口> \
       --username <账号> --password <密码> \
       --totp-secret <TOTP密钥> \
       --disable-zju-config
```

连接成功后 Termux 会显示日志,保持该会话即可。

### 让手机上的应用走隧道

- **Firefox**:地址栏进 `about:config` → `network.proxy.type=1`、
  `network.proxy.socks=127.0.0.1`、`network.proxy.socks_port=1080`、`network.proxy.socks_version=5`
- **系统级(HTTP)**:WiFi 设置 → 当前网络 → 代理"手动" → `127.0.0.1:1081`
  (对遵循系统代理的应用生效,覆盖大多数 App 的 HTTP(S) 流量)
- 其他支持自定义代理的 App 直接填 SOCKS5 地址即可

### 挂机不中断

```
termux-wake-lock                       # 保持 CPU 唤醒
```

并在系统设置里给 Termux 关闭电池优化(设置 → 应用 → Termux → 电池 → 无限制),
否则息屏后引擎可能被系统冻结。

## 注意

- **多端同账号可能互踢**:手机和电脑同时连同一账号时,服务器可能踢掉其中一个;
  桌面端会自动重登(约 1 秒),手机端 CLI 同样内置该逻辑。
- 服务端会话约 8 小时 20 分到期,引擎会自动重登,日志里可见
  `Re-login success, L3 tunnel rebuilt`,属正常现象。
- 若手机是 32 位 arm(老设备),把构建脚本里 `GOARCH=arm64` 改为 `arm` 重新编译。

## 正式 App(未实现,规划)

上游 `zju-connect/mobile/mobile_android.go` 提供了 gomobile 绑定
(`Login()` + `StartStack(fd)`),可配合 Android VpnService 实现 TUN 全局接管
(所有 App 无感走 VPN,无需配代理)。当前缺失:

1. mobile 绑定不支持 TOTP(需把 `AuthOptions.TOTPSecret` 接入)
2. mobile 走 tun 栈,未包含本 fork 的掉线自愈逻辑
3. 需要本机安装 Android Studio + NDK 才能产出 AAR/APK

需要时按此路径推进。

---

## 正式 App(android-app/,已完成 v1)

全局 TUN 接管:所有 App 的流量无感走 VPN,无需配置代理。构建产物已放在桌面
(`EC隧道.apk`,debug 签名直接安装)。

- 工程结构:`android-app/`(Gradle + Kotlin,两个源文件:
  `MainActivity` 表单 + `TunnelService` VpnService)
- 引擎经 `build/android/zju-mobile.aar` 集成(构建脚本 `build/android/build.sh`)
- 关键设计:`addDisallowedApplication(自身)`——本 App 自己的流量绕过 VPN,
  避免引擎到服务器的连接被回环进隧道;DNS 由引擎登录后下发
- 断线自动重连:会话到期(约 8 小时 20 分)隧道断开后,服务自动重新登录并
  重建 VPN 接口,无需人工干预;失败按 3s/6s/…退避重试

### 重新构建 APK

```
bash build/android/build.sh                # 引擎 AAR(先)
cd android-app && gradle assembleDebug     # APK(后)
```

需要 ANDROID_HOME(已配置)与 Gradle 8.9(已装在 ~/tools/gradle-8.9)。
