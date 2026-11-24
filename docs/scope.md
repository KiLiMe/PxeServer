# 能力边界与未实现

## 平台支持

Windows 与 Linux 共用同一套 `internal/` 内核，行为一致（同一 DHCP/TFTP/HTTP 代码、同一种配置格式），差异全在边界上：

| | Windows | Linux |
| --- | --- | --- |
| 桌面界面 | `PxeServer.exe`（mygo） | `pxeserver`（同一份 mygo 代码，系统 GTK3 + WebKitGTK） |
| 命令行 | `pxed.exe` | `pxed` |
| 67/69/80 特权端口 | 无需管理员 | 需 `setcap` / `sudo` / systemd `AmbientCapabilities`；界面会提前检测并拦住「启动」 |
| 防火墙 | 界面一键「放行程序」（按程序路径） | 不提供（块级隐藏）；自己开 `dhcp` 服务与 `69/udp` |
| 单实例 | 命名互斥体 + 把窗口拉到前台 | `flock` 锁文件 + 一句话提示退出 |
| 发行方式 | 单文件 exe / zip | `.deb`、`.rpm`、`PKGBUILD`、`install.sh` tar.gz |

本仓库只发 Windows 与 Linux 的 amd64 构建。macOS / 其它架构（arm64 等）理论上是同一个 mygo 后端、能自己 `go tool mygo build -target` 出来，但目前未验证、也不提供安装包。

## 支持矩阵

| 映像 | 状态 | 依赖 |
| --- | --- | --- |
| WinPE / 任意 WIM | **支持** | `wimboot` + `bcd`/`boot.sdi`/`bootmgr`/`bootx64.efi` + `.wim` |
| ArchISO 系 Linux ISO | **后端保留、入口已剥离** | `/iso/` 只读视图 + `archiso_http_srv=` 参数 |
| Ubuntu/Debian、RHEL/Fedora 原版 ISO | 未支持 | 需各自安装器参数模板（`iso-url=` / `inst.stage2=`） |
| Windows 安装 ISO | **不支持** | 见下节 |
| NBD / iSCSI / SMB 后端 | 未实现 | — |

## 为什么 Windows 安装 ISO 起不来

1. **只读解析限制**：Windows 安装 ISO 的核心文件位于 UDF 分区，而 `internal/isorom` 基于 `kdomanski/iso9660` 仅解析 ISO9660（ArchISO 系为纯 ISO9660/Rock Ridge）。
2. **安装器协议限制**：WinPE 下 `setup.exe` 只认盘符与 UNC 路径，无法直接通过 HTTP 挂载 4~5 GB 的 `install.wim`。

| 绕行路线 | 做法 | 代价 | 结论 |
| --- | --- | --- | --- |
| A. wimboot + SMB | `wimboot` 起安装 PE，只读 SMB 共享提供 `install.wim` | 中（约 300 行） | 推荐 |
| B. 只进 PE | 进安装界面后卡在找 `install.wim` | 低 | 半成品，不单独交付 |
| C. `isorom` 加 UDF | 解锁读取 ISO 内文件 | 中 | 仅解锁读，安装仍卡住 |
| D. 自制 WinPE + 虚拟盘驱动 | 复刻 iVentoy | 极高（驱动 + 签名） | 不做 |

## ISO 直连剥离现状

为聚焦 WinPE/WIM 场景并减少界面/配置维护面，提交 `5ffa9d9` 剥离了 `internal/bootmenu`、ISO 挂载配置与 UI/CLI 入口；目前仅保留 `internal/isorom/iso.go` 与 `internal/httpd/iso.go` 的 `/iso/` 路由（因无调用 `httpd.SetISOs`，`/iso/*` 恒返回 404）。如需还原，直接检出 `5ffa9d9^` 相关文件即可。

## 明确不做的三项

- **BINL**（WDS 协议）：本项目走 iPXE → `wimboot` 链路，无需 WDS 握手。
- **DNSd**：下发地址与 URL 全为 IP，无域名解析需求，避免抢占 `53` 端口。
- **SMB**：老式 SMB1 已被 Win10/11 默认禁用，SMB2/3 工程量过大；PE 中可用 `curl.exe` 走 HTTP 拉取大文件。

## 其他方案备忘

| 方案 | 客户端要求 | 本项目状态 |
| --- | --- | --- |
| netboot.xyz | iPXE 即可 | 一条 `chain --autofree https://boot.netboot.xyz/ipxe/netboot.xyz.efi` 即可接入 |
| NBD | 需 `nbd-client` / 内核驱动 | 未实现（难在客户端 initramfs） |
| sanboot over HTTP/iSCSI | iPXE 自带 | 仅用 `sanboot --no-describe --drive 0x80` 回本地盘；HTTP 挂载无缓存且受限于镜像格式/签名，iSCSI target 未实现 |

## 其他已知限制

- DHCP 租约池为内存表（重启清空），无 ARP 冲突探测。
- 仓库不包含任何引导二进制，见 [boot-files.md](boot-files.md)。
- Linux 不做窗口激活（无通用 X11/Wayland 接口），第二次启动只提示后退出。
