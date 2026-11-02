# 安装与运行

## 环境

| 项 | 版本 |
| --- | --- |
| Go | 1.27.3 |
| github.com/egoist/mygo | v0.9.2（原生界面 + `go tool mygo build` 打包） |
| github.com/kdomanski/iso9660 | v0.4.0 |
| github.com/pin/tftp/v3 | v3.2.0 |
| golang.org/x/sys | v0.50.0 |
| PowerShell | 7+（构建脚本） |

## 目录

| 路径 | 作用 |
| --- | --- |
| `main.go` | 桌面版唯一主包文件：mygo 启动、窗口装配、`-capture` 截图分支 |
| `cmd/pxed/` | 命令行入口；`-selftest` 自检 |
| `internal/app/` | 服务起停、配置持久化、日志、菜单、对话框、防火墙、调试接口 |
| `internal/ui/` | mygo 原生界面（六个选项卡） |
| `internal/config/` | `Config`（字段 = 界面控件）、IP 探测、额外 DHCP 选项解析 |
| `internal/dhcp/` `internal/tftp/` `internal/httpd/` | 三个服务的实现 |
| `internal/isorom/` | ISO9660 只读视图（ISO 直连已剥离，仅余后端，见 [scope.md](scope.md)） |
| `internal/netif/` `internal/firewall/` `internal/shell/` `internal/logbook/` `internal/locales/` | 网卡枚举、防火墙、用默认程序打开文件、日志级别、界面文案 |
| `boot/menu.ipxe` | 示例 iPXE 菜单（唯一入库的 boot 文件） |
| `Scripts/` | `build.ps1` 本地构建、`release.ps1` 打标签发版 |
| `docs/` | 全部说明，索引见 [README.md](README.md) |

## 运行

```pwsh
go run ./cmd/pxed -selftest   # 编解码自检：不联网、不占端口、不要管理员
go run .                      # 桌面界面；默认不启动服务（「更多 → 启动行为」可改）
go run ./cmd/pxed             # 命令行版：立刻起服务，首次在当前目录生成 config.json
```

桌面版与命令行版**不能同时运行**：都要占 udp:67 / 69 / 4011，后启动的会报「udp:67 可能已被其他 DHCP 服务占用」。

## 构建

```pwsh
pwsh ./Scripts/build.ps1 -Test   # go vet + go test，再 go tool mygo build → build/bin/PxeServer.exe
pwsh ./Scripts/build.ps1         # 同上，不跑测试
go build -o build/bin/pxed.exe ./cmd/pxed   # 命令行版（普通 go build）
```

图标（`build/appicon.png`）、exe 文件属性版本、PerMonitorV2 manifest 都由 `go tool mygo build` 写入；
`build.ps1` 会先删掉残留的 `rsrc_windows_*.syso`，否则它们会静默盖掉这些资源。
版本号只来自 `mygo.json` 的 `version`（开发构建是 `0.0.0-dev`），规则见 [versioning.md](versioning.md)。

`build.ps1` 传了 `-skip-installer`：mygo 0.9 起 `mygo build` 默认还会下载 NSIS 产一个
`pxeserver-setup-<版本>-amd64.exe`，本项目只发 exe + zip，用不上就去掉这个 flag。

升级 mygo 时两个都要升，否则 `go tool mygo generate` 会报 `missing go.sum entry … go.digitalxero.dev/rpm`：

```pwsh
go get github.com/egoist/mygo@vX.Y.Z
go get github.com/egoist/mygo/cmd/mygo@vX.Y.Z   # 工具链的依赖（rpm/NSIS 等）也要进 go.sum
go mod tidy
```

调试截图：`PxeServer.exe -capture screenshot.png`，启动约两秒后保存原生窗口并退出。

## 防火墙

Windows 上绑定 67/69 **不需要管理员**（低端口无特权限制），但入站流量要过防火墙。
界面「防火墙 → 放行程序」按**程序路径**放行、不限端口/协议：一条入站 Allow 规则
（`-Program <exe 路径>`、`-Protocol Any`，规则名 `PXE Server（程序路径放行）`）。
换端口、换协议都不用重新加规则；不是管理员时会 UAC 提权重跑一次（`PxeServer.exe -firewall-apply`，跑完即退出）。

- 状态显示：管理员下实时查规则；查不到时用 `config.json` 的 `runtime.firewall_exe`（放行成功时写入）兜底。
- 「撤销放行」删掉规则并清掉 `runtime.firewall_exe`。

## 实测（虚拟机里跑一遍）

1. 按 [boot-files.md](boot-files.md) 准备 `snponly.efi`（UEFI）/ `undionly.kpxe`（BIOS）与 `menu.ipxe`，放进 `boot/`。
2. 新建虚拟机，网卡选**旧版/legacy 适配器**才能触发 PXE（Hyper-V 的 legacy adapter；VMware 直接可 PXE）。
3. 看日志：`DHCPd：MAC=… DHCPDISCOVER → OFFER 192.168.5.2，启动文件 snponly.efi` → `TFTPd：GET snponly.efi` → 客户端进 iPXE 菜单。
4. 卡在 "Configuring" 多半是 43/4011 的问题；进了 iPXE 但传输慢，查 blksize 是否协商成功。见 [troubleshooting.md](troubleshooting.md)。
