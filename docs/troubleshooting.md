# 排错手册

## 客户端反复 DHCPDISCOVER、始终 `0.0.0.0`

**现象**：日志每隔几秒一条 `DHCPd：MAC=… DHCPDISCOVER → OFFER 192.168.5.2…`（之后合并成 `（同 MAC 第 5 次）`），
然后**永远没有** REQUEST/ACK，也看不到 `TFTPd：` 请求，客户端一遍遍重试。

**原因**：勾着 `ProxyDHCP`，但当前网段根本没有别的 DHCP 服务器。代理模式按设计**只补 PXE 启动信息、不发 IP**。

**解法**：取消勾选 `ProxyDHCP`（改成普通 DHCP，自己发 IP），保存后服务重启，日志应从
`已在 …:67 上监听（ProxyDHCP…` 变成 `（普通 DHCP，地址池 …）`，并出现 `→ OFFER`。
启动日志里那条 `ProxyDHCP 只补 PXE 启动信息、不分配 IP…` 就是给这个场景留的线索。

## UEFI 一定要用 snponly 构建

2026-11 在一台联想实机（UEFI x64，arch=7）踩到，表现极具欺骗性：iPXE 起来、菜单出来、`wimboot` / `bcd` /
`boot.sdi` / `bootx64.efi` / `boot.wim` 全部下载成功，然后屏幕**一直停在厂商 logo + 转圈**，服务端此后收不到任何请求
（PE 若起来，`wpeinit` 会再发一次 DHCP）。

**原因**：`ipxe.efi` 是全驱动构建，启动时用 `DisconnectController()` 把固件绑在网卡上的驱动解绑、改用自己的驱动接管；
交出控制权时调 `ExitBootServices()`，固件再也无法把网卡恢复原状（iPXE 留下的 DMA 描述符/中断配置还在），
部分固件在收尾阶段碰网卡就挂死 —— 所以现象必然是「**文件全下完了才卡**」。
`snponly.efi` 只通过 `EFI_SIMPLE_NETWORK_PROTOCOL` 收发包，退出时网卡状态原封不动，换上去一次成功。

**分辨手里的二进制**：按 Latin-1 搜字符串，全驱动版有 `i219`、`net80211`，snponly 版有 `SNPONLY`；体积 1.1 MB vs 200~300 KB。

**排除掉的假线索**（别再走一遍）：

| 猜测 | 实测 |
| --- | --- |
| `wimboot` 版本太旧 | 62,440B（Tiny 自带）与 76,064B（官方 v2.9.0）**卡在同一位置** |
| `boot.wim` 损坏 | 结构完整，LZX、单索引 WinPE 10.0.19041.1，展开 1,816 MB，能正常挂载 |
| 内存不够 | wimboot 峰值 1.5 GB，换 snponly 后同样内存照样进 PE |
| `bootmgr` 与 PE 版本错配 | 换官方 snponly 后同一套 `bcd`/`bootmgr`/`boot.wim` 正常启动 |
| 让固件直接加载 `bootmgfw.efi` | 同样在 `boot.wim` 传完后卡住，原因未查明，不能反推 iPXE 的结论 |

## Hyper-V Gen2 / UEFI 的四个坑（已在代码里修掉）

| 现象 | 原因 | 修法 |
| --- | --- | --- |
| DHCP 全流程正常，客户端却不发 TFTP/ARP，报 `A boot image was not found` | 只写了 option 67，BOOTP 固定字段 `file` 没填 | `reply()` 里同时写 `r.File` 与 option 67 |
| 同上，抓包连 ARP 都没有 | Option 43 子项 6 = 0x03 把简化版客户端带偏 | `no_option43` 开关，默认不给 EFI 发 43 |
| 客户端换端口问 4011，我们没应答 | 4011 只在 ProxyDHCP 时绑定，且应答一律广播到 :68 | 两种模式都绑 4011；应答单播回请求方 |
| iPXE 起来了却停在 `Press Ctrl-B`，还去 ipxe.org 找脚本 | iPXE 二次 DHCP 时仍回 `ipxe.efi`，没回菜单脚本 | user-class 含 `iPXE` 时改回 `menu.ipxe` |

`boot\autoexec.ipxe` 是给 iPXE 的第一入口（`chain tftp://${next-server}/httpboot.ipxe`），
有它就不用等第二次 DHCP、也不会去外网找脚本。

## Legacy BIOS / PXE ROM 的坑

同一台物理机（Realtek PCIe GBE v2.66 + Intel UNDI PXE-2.1 build 083，UEFI 已跑通）切到 BIOS 模式反复失败，
最后靠「用正版 TinyPXE 做 A/B + 抓包对比」定位：

| 现象 | 原因 | 修法 |
| --- | --- | --- |
| 屏幕报 `PXE-E74: Bad or missing PXE menu and/or prompt information` 后 `PXE-M0F` 退出 | 给 BIOS 发了 Option 43（子项 6/8/10/11）；该 ROM 看到 43/60 就进入「PXE 发现」流程并要求菜单完全合规 | **BIOS 一律不发 Option 43** |
| 加了子项 10/11 后仍报 E74 | 子项 11 字段是 `{超时 1 字节}{提示文字}`，按 2 字节写导致文字错位 | 改成 1 字节超时 |
| 不发 43 却仍不取文件，反而去问 4011 | 应答里带了 option 60/66/67，ROM 据此进入发现流程 | **BIOS 走最小应答：不发 60/66/67，只写 `file`/`siaddr`** |

判定依据是正版 TinyPXE 对同一台机器、同一个 ROM 的真实应答：

```
op=2  yi=192.168.5.100  si=192.168.5.1  file='ipxe.pxe'  sname='PXE-SRV'
选项：1=255.255.255.0  3 6 13=0289  28 51 53 54  175=010101080101
（没有 43、没有 60、没有 66、没有 67）
随后 TFTP 取 ipxe.pxe 成功
```

回归测试：`internal/dhcp/option43_test.go` 的 `TestLegacyGetsMinimalReply`。实测成功后的 BIOS 链路：

```
15:53:08 TFTPd：GET undionly.kpxe（80942 字节）
15:53:10 第二次 DHCP：下发 http://192.168.5.1:80/menu.ipxe
15:53:13 HTTPd：GET /menu.ipxe -> 200
15:53:28 HTTPd：GET /wimboot /wim-boot/{bcd,boot.sdi,bootx64.efi,bootmgr} -> 200
15:53:34 HTTPd：GET /BOOT3/boot.wim -> 200（6.4 s）
```
