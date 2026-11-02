# 引导文件与网卡准备

## 仓库里没有任何二进制

`.gitignore` 排除 `boot/` 下的 `*.efi` `*.kpxe` `*.iso` `*.wim` `*.sdi` `*.img` 等。
原因：`boot.wim` 一张 700 MB+，ISO 动辄 1~8 GB（GitHub 单文件上限 100 MB），且镜像分发权在厂商手里。
**clone 下来是跑不起来的**，下面这些文件请自备。

### 一、iPXE 引导程序（公开下载，按固件挑 1~2 个）

| 文件 | 用途 |
| --- | --- |
| `snponly.efi` | **UEFI x64 首选**：只走固件 SNP 协议，不解绑网卡驱动（默认配置就指向它；理由见 [troubleshooting.md](troubleshooting.md)） |
| `ipxe.efi` | UEFI x64 全驱动版；部分实机会「文件下完就卡死」，仅作备选 |
| `undionly.kpxe` | **BIOS/Legacy 第一棒**（基于网卡 UNDI 驱动，通用性最好） |
| `ipxe32.efi` / `ipxe-arm64.efi` | UEFI x86（老平板）/ UEFI ARM64（树莓派 4/5 等） |
| `shimx64.efi` / `shimaa64.efi` | Secure Boot 下先过 shim；签名版 iPXE 在 `x86_64-efi-sb/` |

下载根目录 <https://boot.ipxe.org/>（`x86_64-efi/snponly.efi`、`x86_64-efi/ipxe.efi`、`arm64-efi/ipxe.efi`、`i386-efi/ipxe.efi`、`undionly.kpxe`）。
想自己编：`git clone https://github.com/ipxe/ipxe && make bin-x86_64-efi/ipxe.efi`。

### 二、Windows PE 那套

| 文件 | 用途 | 获取方式 |
| --- | --- | --- |
| `wimboot` | 让 iPXE 直接喂 `boot.wim`（约 60 KB） | <https://github.com/ipxe/wimboot/releases> |
| `boot.wim` | **你自己的** WinPE 镜像，几百 MB | Windows 安装 ISO 的 `sources\boot.wim`；或 ADK `copype amd64 C:\winpe` |
| `bcd` `boot.sdi` `bootmgr` `bootx64.efi` | wimboot 需要的 Windows 引导链 | 同一个 ISO 的 `sources\boot.wim` 里取（7-Zip 解开）；或本机 `C:\Windows\Boot\` |
| `memdisk` | 把 ISO 整包载入内存再启动 | syslinux 发布包 `bios/memdisk` |
| `menu.ipxe` / `autoexec.ipxe` | 菜单脚本、二次引导脚本 | 仓库里有示例 `boot/menu.ipxe`；缺失时程序自动生成模板 |

### 三、ISO / WIM 是你自己的东西

本程序只负责把文件「通过 HTTP 暴露出去」或「喂给 wimboot」，不下载、不自带、不上传：

- WIM：放进 `boot/`，在 `menu.ipxe` 里用 `wimboot` + `initrd` 引用（示例见 `boot/menu.ipxe` 的 `:pe` 段）。
- ISO：本版本不提供 ISO 直连启动，能起哪些、为什么起不来见 [scope.md](scope.md)。

最小可用集：`snponly.efi`（UEFI）**或** `undionly.kpxe`（BIOS）一个 + `menu.ipxe`。

> 省事做法：TinyPXE 自带的 `files\` 目录里有一整套现成的（`wimboot`、`wim-boot\`、`BOOT3\boot.wim`），
> 整份拷进 `boot\` 后 `menu.ipxe` 里的 `/wimboot`、`/wim-boot/...`、`/BOOT3/boot.wim` 正好对得上。

## 网卡选择（不需要手填 IP）

设置页有「网卡（自动填 IP）」下拉，列出本机已启用的 IPv4 网卡。选中后自动填好服务器 IP（Option 54）、
子网掩码、广播地址，Next-Server 为空时跟随，IP 池起始若不在同一网段也一并改到该网段 `.2`。
选择记在 `localStorage`，下次打开自动套用；后端是 `App.ListInterfaces() []netif.Iface`（`internal/netif`）。

- 「重新扫描」：插拔网线、连 VPN、Hyper-V 交换机增删后点一下，不用重启程序。
- 下拉里的「（手动填写 IP）」是保留项：没有任何网卡能对上目标网段时选它，下方 IP 字段照旧可以手改，
  程序不会把已填的值冲掉。
