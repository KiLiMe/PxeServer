# PxeServer

手写 DHCP / ProxyDHCP + TFTP + HTTP 的 PXE 服务端，提供 mygo 原生桌面端与 CLI 入口。

## 快速启动

```pwsh
go run ./cmd/pxed -selftest      # 1. 协议自检
go run .                         # 2. 启动桌面端
pwsh ./Scripts/build.ps1 -Test   # 3. 测试并构建 PxeServer.exe
```

## 依赖环境

| 依赖 | 版本 |
| --- | --- |
| Go | 1.27.3 |
| github.com/egoist/mygo | v0.9.2 |
| github.com/kdomanski/iso9660 | v0.4.0 |
| github.com/pin/tftp/v3 | v3.2.0 |
| golang.org/x/sys | v0.50.0 |
| PowerShell | 7+ |

## 许可

MIT License，见 [LICENSE](LICENSE)。