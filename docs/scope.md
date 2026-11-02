# 能力边界与未实现

## 能起什么

| 映像 | 状态 | 依赖 |
| --- | --- | --- |
| WinPE / 任意 WIM | **支持** | `wimboot` + `bcd`/`boot.sdi`/`bootmgr`/`bootx64.efi` + 目标 `.wim` |
| ArchISO 系 Linux ISO（SystemRescue、Arch、CachyOS…） | **后端保留、入口已剥离** | `/iso/` 只读文件视图 + 内核参数 `archiso_http_srv=` |
| Ubuntu/Debian、RHEL/Fedora 原版 ISO | 未支持 | 需要各自安装器参数模板（`iso-url=` / `inst.stage2=`） |
| Windows 安装 ISO | **不支持** | 见下节 |
| NBD / iSCSI / SMB 后端 | 未实现 | — |

引导文件怎么准备见 [boot-files.md](boot-files.md)。

## 为什么 Windows 安装 ISO 起不来

1. **读不出内容**：真正的 `bootmgr`、`sources\boot.wim`、`sources\install.wim` 全在 UDF 里，
   而 `internal/isorom` 基于 `kdomanski/iso9660`，只解析 ISO9660。实测样本（7.87 GB，
   卷标 `CPRA_X64FRE_ZH-CN_DV9`）的 ISO9660 根目录**只有 1 个条目 `README.TXT`**。
   ArchISO 系能读，是因为它们是纯 ISO9660/Rock Ridge。
2. **就算能读也装不了**：那套依赖「内核自带 HTTP 客户端」。WinPE 里没有组件能把 `install.wim` 从 HTTP
   当系统源，`setup.exe` 只认**盘符**与 **UNC 路径**，而 `install.wim` 有 4~5 GB，不可能走 memdisk。

绕过去的路线（按代价排序）：

| 路线 | 做法 | 代价 | 结论 |
| --- | --- | --- | --- |
| A. wimboot + SMB | 从 ISO 取出引导链用 wimboot 起安装 PE；`install.wim` 由只读 SMB 共享提供 | 中（约 300 行） | 推荐 |
| B. 只做「进 PE」 | 能进安装界面，选完语言卡在找 `install.wim` | 低 | 半成品，不单独交付 |
| C. 给 isorom 加 UDF | 解锁 `/iso/` 读任意 ISO 内文件 | 中 | 只解锁「读」，安装流程依旧卡住 |
| D. 自制 WinPE + 注入虚拟盘驱动 | 等于复刻 iVentoy | 极高（驱动 + 签名 + 兼容性） | 不做 |

## ISO 直连：为什么剥离、怎么还原

原理：不用解包、也不用 memdisk，把 ISO 当只读文件系统按 HTTP Range 暴露出来，内核自己流式读 squashfs
（`/iso/<ISO 段名>/<ISO 内路径>`，`http.ServeContent` 带 Range 支持）。iVentoy 走的是 NBD + 自带 initramfs，
复杂度在客户端；这条路复杂度在服务端，客户端零依赖，但只覆盖原生支持 HTTP 的发行版。

剥离理由：目标机器绝大多数是 WinPE/WIM；能直连的只有 ArchISO 系少数发行版，其余各要一套参数模板；
入口一旦暴露就得跟着界面、配置、菜单生成、命令行一起维护。

**保留**：`internal/isorom/iso.go`（ISO9660 只读视图）与 `internal/httpd/server.go` 的 `/iso/` 路由。
没有任何代码调用 `httpd.SetISOs`，所以 `/iso/*` 恒返回 404。

**还原**：整段代码在 git 历史里，直接取出来即可 —— `git show 5ffa9d9^:internal/bootmenu/menu.go`
（同提交 `5ffa9d9` 剥离了 `internal/bootmenu`、`config.ISOList/ISOPaths`、`app.mountISO/InsertISOMenu/PickISO`、
`ui` 的 ISO 控件与 `cmd/pxed` 的挂载段，`git show 5ffa9d9` 就是完整清单）。

## 明确不做的三样：BINL / DNSd / SMB

原版 TinyPXE 面板上的三个勾选框，本项目不做，工具栏里也已去掉（不占位、不误导）。

- **BINL**（微软 RIS/WDS 的 Boot Information Negotiation Layer，走 udp 4011 问「谁是 WDS、启动文件叫什么」）：
  我们走 iPXE 链路 —— 固件拿 `snponly.efi` → 菜单 → `wimboot` 载入 `boot.wim`，没有一步会问 BINL。
  做它等于重写一遍 WDS 握手，收益为零。
- **DNSd**（只回 A 记录的迷你 DNS）：我们下发的 Option 66/67、`siaddr`、菜单 URL 全是 IP，客户端不需要解析域名；
  再塞一个服务进 53 端口还会和系统 DNS、路由器 DNS 打架。想用域名就在上游路由器/域控里解析到本机。
- **SMB**（只读共享，方便 PE 里 `net use`）：它实现的是 SMB1，Windows 10/11 默认已禁用；SMB2/3 是另一个大工程；
  我们已有 HTTPd。替代做法：PE 里 `curl.exe -o D:\install.wim http://<服务器>/BOOT3/install.wim`。

## 其它方案记录（避免重复调研）

| 方案 | 解决什么 | 客户端要求 | 本项目状态 |
| --- | --- | --- | --- |
| netboot.xyz | 现成的多发行版菜单（kernel/initrd over HTTP） | 无（iPXE 即可） | 一条 `chain --autofree https://boot.netboot.xyz/ipxe/netboot.xyz.efi` 即可接入，未默认启用 |
| NBD | 任意 ISO 挂成块设备 | 需 `nbd-client` / 内核驱动 | 未实现（服务端约 400~600 行，难在客户端自带 initramfs） |
| sanboot over iSCSI/HTTP | 远端块设备当本地盘 | iPXE 自带 | 仅用 `sanboot --no-describe --drive 0x80` 回本地硬盘；iSCSI target 未实现 |
| wimboot + WIM | Windows PE | 无 | **已实现** |

`sanboot http://…/x.iso` 已知不可用：无缓存（每次随机读都重新发请求，安装器读大文件极慢）、只读、
UEFI 下只能引导本身是 EFI 可引导的映像，Secure Boot 再叠一层签名问题。

## 其它未实现

- DHCP 池派址是内存表（重启清空），无 ARP 冲突探测，租约不落盘。
- 引导二进制一律不入库（`.gitignore` 已排除），见 [boot-files.md](boot-files.md)。
