# PxeServer

手写 DHCP / ProxyDHCP + TFTP + HTTP 的 PXE 服务端，提供 mygo 原生桌面端与 CLI 入口，Windows / Linux 同源。

## 快速启动

```pwsh
go run ./cmd/pxed -selftest      # 1. 协议自检
go run .                         # 2. 启动桌面端
pwsh ./Scripts/build.ps1 -Test   # 3. 测试并构建 PxeServer.exe
pwsh ./Scripts/build.ps1 -Target linux   # 4. 出 Linux 包（deb / rpm / PKGBUILD / tar.gz）
```

## 依赖环境

| 依赖 | 版本 |
| --- | --- |
| 系统 | Windows 10+ / Linux（amd64，GTK3 + WebKitGTK） |
| Go | 1.27.3 |
| github.com/egoist/mygo | v0.9.2 |
| github.com/kdomanski/iso9660 | v0.4.0 |
| github.com/pin/tftp/v3 | v3.2.0 |
| golang.org/x/sys | v0.50.0 |
| PowerShell（构建脚本） | 7+（Linux 也能装） |

## 许可

MIT License，见 [LICENSE](LICENSE)。