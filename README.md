# pxed — PXE 服务内核（Go）

手写 DHCP / ProxyDHCP + TFTP + HTTP 的 PXE 服务端，自带 mygo 原生桌面界面，也有无界面命令行入口。界面支持简体中文与英文（设置 → 界面）。

## 快速开始

```pwsh
go run ./cmd/pxed -selftest   # 1. 自检（无需管理员、无需网络）
go run .                      # 2. 启动桌面界面
pwsh ./Scripts/build.ps1 -Test  # 3. vet + test + 构建 → build/bin/PxeServer.exe
```

## 依赖环境

| 项 | 版本 |
| --- | --- |
| Go | 1.27.3 |
| github.com/egoist/mygo | v0.9.2 |
| github.com/kdomanski/iso9660 | v0.4.0 |
| github.com/pin/tftp/v3 | v3.2.0 |
| golang.org/x/sys | v0.50.0 |
| PowerShell | 7+（构建脚本） |

## 文档

全部说明在 [docs/](docs/README.md)，按顺序读：[getting-started](docs/getting-started.md) → [boot-files](docs/boot-files.md) → [architecture](docs/architecture.md)；界面、调试接口与多语言见 [gui](docs/gui.md)；排错见 [troubleshooting](docs/troubleshooting.md)；能力边界见 [scope](docs/scope.md)；版本号与发版见 [versioning](docs/versioning.md)。

版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)：`X.Y.Z`，不兼容改动升主版本、新增功能升次版本、修 Bug 升修订号；`0.y.z` 为初始开发阶段。

## 许可

MIT License，见 [LICENSE](LICENSE)。需保留版权声明与许可声明；软件按「现状」提供，不附带任何担保。`boot/` 下的 iPXE、wimboot 等二进制遵循各自许可证。