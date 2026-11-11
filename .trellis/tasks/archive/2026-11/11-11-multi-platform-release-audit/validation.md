# 四端发布验收（2026-11-11）

发布源码提交：`240e94156fc6235a1ebc108e415bfc8eace66d20`。

## 已完成检查

- 本地根模块：228 个 Go 测试/子测试通过，0 失败，4 个系统/交互夹具测试跳过；`go vet ./...` 通过。
- 本地独立 `zju-connect` 模块：修复前 471 通过、2 个 Windows POSIX 权限断言失败；修正后 473 通过、0 失败、0 测试跳过；vet 通过。
- 前端：33 个测试通过，无失败、取消或跳过。
- 独立只读审查与 `git diff --check` 通过。未改产品协议、认证、连接或 TUN 行为；未添加兜底。
- GitHub Actions 同提交全部成功：
  - Windows：<https://github.com/mam15mon/EasyConnectSuite/actions/runs/42952233776>
  - macOS Intel / Apple Silicon：<https://github.com/mam15mon/EasyConnectSuite/actions/runs/42952233881>
  - Linux：<https://github.com/mam15mon/EasyConnectSuite/actions/runs/42952233799>
  - Android：<https://github.com/mam15mon/EasyConnectSuite/actions/runs/42952233782>
- 核对六个 Actions artifact 的 `workflow_run.head_sha` 均为发布提交且未过期。下载后重命名时哈希保持一致。
- 检查 Windows PE x64、macOS Mach-O Intel/ARM64、Linux ELF x64；macOS/Linux tar 可执行权限保留；APK ZIP 完整且含四种 ABI。

## 校验清单

| 文件 | SHA256 |
| --- | --- |
| VPN-windows-amd64.exe | 815a31d96a56c882effc30ad40d7c4ac7d02a855f0f80d1424ce112c202f8fad |
| VPN-macos-amd64.tar.gz | dcafb189f9d414e18ad33d481e46583430f60a01b97c5da89e257a04768d9f79 |
| VPN-macos-arm64.tar.gz | d798bbfb8e4fc7e32797f25680dfb22964a4957b93f9fbcd897974ef382eb097 |
| VPN-linux-amd64.tar.gz | 969b1873930145feb825c2f5af789d778cf263f9dc01eef1ed018fcc28efa832 |
| vpn-headless-linux-amd64.tar.gz | 6724264375a5e5c780d49e57be7867930ccbb10f76b2bf18e1aa3173f0277696 |
| VPN-android-debug.apk | 0bb4849dd35d2cb67fc0c29f11009f4badf2d8be57b5d01c3630d17bc2364d40 |

## 验证边界

- Windows 本地未启用 `TestTUNSystemUACAndTwoAttachments`、`TestTUNSystemUACCancellation`、`TestTUNSystemCrashFixture`、`TestTUNSystemParentCrashCleanup`；它们分别需要交互授权、拒绝授权、子进程夹具或真实系统 TUN。CI 的 TopSAP 适配器遇到缺少物理 PCI 身份的精确错误会跳过，不能称作通过。
- Linux 的内部 TUN/路由/resolved/Worker 隔离测试成功，但用户入口仍仅代理；TopSAP Linux 设备身份未实现。
- Android 是 EasyConnect 原型及调试 APK；macOS 尚未签名公证。
- 本轮未连接真实网关、未测试实体 Mac/Android，未证明长期稳定性、全部发行版、全部协议认证方式或网关固件兼容。
- Windows 历史 TopSAP fix4 实网成功记录仍保留，不能替代本次发布包验收；UniVPN Go 实际登录成功尚未确认。

## Release

已公开发布：[preview-2026.11.11](https://github.com/mam15mon/EasyConnectSuite/releases/tag/preview-2026.11.11)，北京时间 2026-11-11 11:55。

- Release tag 解析到发布提交 `240e94156fc6235a1ebc108e415bfc8eace66d20`，非草稿、预览版。
- GitHub 返回的全部七项资源 SHA256/大小与本地 Actions 下载文件一致。
- 六个二进制与校验清单的公开匿名下载链接最终均返回 HTTP 200；一次并发链接检查发生 TLS EOF，单独重试相关链接成功。
- 初次并发上传部分资源停留在 starter；保留已上传文件，停止该上传进程并逐项重传未完成资源后全部成功。发布前始终保持草稿。
- 本文及后续归档/会话日志属于发布后的验收记录；产物来源固定为上述已通过 CI 的提交，不将记录提交称为重新构建后的二进制来源。
