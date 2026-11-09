# TopSAP 网卡识别修复

每张网卡独立读取完整 PnPInstanceId，避免上一张虚拟网卡的短注册表值限制下一张 PCI 网卡的读取长度。保留 GetAdaptersInfo 顺序、PCI 和六字节 MAC 筛选及原摘要算法，不增加其他设备标识方案。

engine.log 记录 PID、profile、网卡序号/类型、PNP 是否存在、PCI 判断、MAC 字节数、是否选中、注册表读取错误和最终数量。不记录原始 MAC、PNPInstanceId 或设备摘要。测试电脑直接连接一次后，回传 engine.log 即可继续定位，无需额外执行网卡信息采集命令。

原来选中集合不变时设备标识不变；原来被错误跳过的 PCI 网卡被识别后，设备标识可能改变。问题电脑真实网关认证及内网访问仍待验证，不能把模拟测试或构建成功当作实际登录成功。

验证：根模块 go test ./... -count=1、go vet ./...、Windows amd64 Wails 生产构建通过。新增 Windows 回归覆盖短虚拟标识后长 PCI 标识、多网卡顺序、读取失败及诊断脱敏。当前环境 CGO 未启用，race 检查未运行成功。

历史交付 build/bin/VPN.exe（大小和哈希不代表当前构建），20,908,544 字节，Compress=false。SHA256：383C8ABA19D72B776D21015D5DCF352D7FCB9E6E903DEE2AF6DD91FC2E980014。
