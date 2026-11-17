# 引导文件与菜单约定

## 准备引导文件

仓库的 `.gitignore` 排除了 `*.efi` `*.kpxe` `*.iso` `*.wim` `*.sdi` `*.img` 等二进制文件，**clone 后需自备引导文件**。最小可用集：`snponly.efi`（UEFI）或 `undionly.kpxe`（BIOS）任一 + `menu.ipxe`（仓库含示例，缺失时程序自动生成模板）。

### 1. iPXE 引导程序

下载地址：<https://boot.ipxe.org/>（可从源码自编 `make bin-x86_64-efi/snponly.efi`）。

| 文件 | 用途 |
| --- | --- |
| `snponly.efi` | **UEFI x64 首选**（默认配置）：仅走固件 SNP 协议，不解绑网卡驱动（原因见 [troubleshooting.md](troubleshooting.md)） |
| `ipxe.efi` | UEFI x64 全驱动版，仅作备选（部分实机文件下完会卡死） |
| `undionly.kpxe` | **BIOS/Legacy 首选**（基于 UNDI 驱动） |
| `ipxe32.efi` / `ipxe-arm64.efi` | UEFI x86 / UEFI ARM64 |
| `shimx64.efi` / `shimaa64.efi` | Secure Boot 下配合 `x86_64-efi-sb/` 签名版 iPXE 使用 |

### 2. Windows PE 引导文件

| 文件 | 用途 | 获取方式 |
| --- | --- | --- |
| `wimboot` | 让 iPXE 直接加载 `boot.wim` | <https://github.com/ipxe/wimboot/releases> |
| `boot.wim` | WinPE 镜像 | Windows 安装 ISO 的 `sources\boot.wim` 或 ADK `copype` |
| `bcd` `boot.sdi` `bootmgr` `bootx64.efi` | Windows 引导链文件 | 从 `boot.wim` 解出或取自本机 `C:\Windows\Boot\` |

> ISO 直连支持边界见 [scope.md](scope.md)。

## `menu.ipxe` 编写约定

- **不用写 IP 或端口**：二次 DHCP 下发的菜单路径为完整 URL（`http://<服务器>:<端口>/menu.ipxe`），iPXE 以脚本自身 URI 为基准解析以 `/` 开头的相对路径：
  ```ipxe
  kernel /wimboot || goto failed
  initrd /wim-boot/bcd          bcd      || goto failed
  initrd /WIM_BOOT_X/boot.wim   boot.wim || goto failed
  ```
- **菜单标签只用 ASCII**：iPXE 的 `menu`/`item` 按字节数算列宽，UTF-8 中文会导致高亮条错位与分隔线断裂；中文提示仅放 `echo` 中。
- **HTTP 默认端口 80**：可在主页随时修改，修改后无需改动菜单里的相对路径。
