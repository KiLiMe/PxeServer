# 架构与协议实现

## 关键实现点

- **手写 DHCP**（不用库）：ProxyDHCP 必须字节级控制选项集合 —— `yiaddr=0`、不给 Option 1/3/51，只给 60/66/67 + 43。
- **ProxyDHCP**：只在 `option 60` 以 `PXEClient` 开头时才应答，且额外监听 **UDP 4011**（PXE boot service）；
  与路由器 DHCP 共存，不需要关掉别人的 DHCP。
- **Option 43 子选项**：`6`（discovery control）、`8`（boot server 列表，`{type2}{count1}{ip4}`）、`10`/`11`（Boot Menu / Prompt）。
  **BIOS/Legacy（arch<6）一律不发 43/60/66/67**，只给 BOOTP 固定字段 `file` —— 老 ROM 看到 43 会去校验引导菜单，
  稍不合规就 `PXE-E74` 然后放弃。UEFI 侧由 `no_option43` 开关控制（默认不发，理由同上：简化固件会被 43 带偏）。
- **架构分流**（Option 93）：`0`→`undionly.kpxe`（BIOS）、`6/7/9`→`snponly*.efi`（UEFI），`10/11`→ARM64，
  未识别回落全局 `boot_file`（默认 `snponly.efi`）。
- **应答地址**：giaddr 非 0 时回复给中继 `giaddr:67`，否则广播 `255.255.255.255:68`；
  Windows 下发广播必须先设 `SO_BROADCAST`，否则 `WSAEACCES`。
- **TFTP** 每个传输用新的临时端口（pin/tftp 负责），监听 69 只解析 RRQ；`blksize=1468` + `tsize`，
  否则 UEFI 拉十几 MB 的 `.efi` 会慢到超时。
- **依赖方向由测试守住**：`deps_test.go` 用 `go list` 列出 `./internal/...` 的 import，禁止主包与 `log`/`log/slog`，
  再按目录扫全部 `.go` 禁止 `fmt.Print*` / `print` / `println`。规则改了改它，别只改文档。
- **日志只走一条路**：内核包（dhcp / tftp / httpd）只调用 `logbook.Logger`（`func(level, format, args)`），
  不碰文件也不打控制台；`internal/app` 的 `logAt` 是唯一落点（内存环形 500 行给界面，`logQ` 交给写线程落盘）。

## 两个可执行文件

同一个内核，两个入口，`go.mod` 里只有一份 `internal/`：

| | `PxeServer.exe` | `pxed.exe` |
| --- | --- | --- |
| 源码入口 | `main.go`（mygo） | `cmd/pxed/main.go` |
| 界面 | 原生窗口，可改配置、看日志、看租约 | 无 |
| 日志 | 界面 + `PXE-server.log`（>2 MB 轮转 `.old`，每行带 `I`/`W`/`E`） | 只打 stdout/stderr |
| 启动参数 | `-capture <png>`、`-firewall-apply` | `-config` `-root` `-no-http` `-selftest` `-version` |
| 启动服务 | 看配置（默认不启动） | 总是立刻起服务 |
| 适用场景 | 日常使用、演示 | 排错（实时 tail 日志）、注册成 Windows 服务、无桌面会话 |

两者**不能同时运行**（都占 udp:67 / 69 / 4011）。发版压缩包里两个 exe 都带。

## DHCP 协议行为

- **OFFER/ACK 同时写 option 67 和 BOOTP 固定 `file` 字段**：Hyper-V 的 UEFI 固件只读 `file`，
  读不到就报 "A boot image was not found"。
- **REQUEST 校验**：option 54 指向别的 DHCP 服务器 → 不回应；请求的地址被别的 MAC 占用、不在池内、
  或是本机/网关保留地址 → 回 **NAK**（option 56 附原因），客户端重新走 DISCOVER。
- **DECLINE 隔离**：客户端报「地址冲突」后，该地址隔离 60 秒不再分配。
- **iPXE/gPXE 二次请求**：`user-class` 含 iPXE 时下发菜单 URL（默认 `menu.ipxe`），传统 PXE ROM 才下发引导固件名。
- **普通 DHCP 客户端**（无 option 60/93，例如 WinPE 自己的网络栈）只给地址，不下发引导文件名。
- 租约池是内存表（重启清空），不落盘、无 ARP 冲突探测。

## 启动链路：小文件走 TFTP，大文件走 HTTP

TFTP 是**停等**协议：一个 block（1468 字节）发出去要等 ACK，实测只有 1~3 MB/s，731 MB 的 `boot.wim` 要好几分钟。
HTTP 是流式的，本机环回 300+ MB/s。所以：

1. 固件只认 TFTP → 用 TFTP 拉 `snponly.efi`（约 1 MB，秒下）；
2. iPXE 起来后跑 `autoexec.ipxe`，它先 `chain tftp://${next-server}/httpboot.ipxe`；
3. `httpboot.ipxe` 由程序**每次启动自动生成**在启动目录里，内容只有一行 `chain http://<服务器>/menu.ipxe`；
4. 菜单脚本从此是「HTTP 资源」，iPXE 以脚本自身 URI 为基准解析相对路径，`initrd /BOOT3/boot.wim` 自动变成
   `http://…/BOOT3/boot.wim` —— 秒级。

`autoexec.ipxe` 里有 `|| chain tftp://${next-server}/menu.ipxe` 兜底，即使 HTTP 没开、`httpboot.ipxe` 没生成也能用。

## 菜单约定

**不用写 IP、也不用写端口。** iPXE 把菜单脚本自身的 URI 当当前工作目录，而脚本是用完整 URL 下发的
（第二次 DHCP 时 option 67 = `http://<服务器>:<端口>/menu.ipxe`），所以以 `/` 开头的路径自动落到同一服务器：

```ipxe
kernel /wimboot || goto failed
initrd /wim-boot/bcd     bcd     || goto failed
initrd /BOOT3/boot.wim   boot.wim || goto failed
```

换 IP、改端口，菜单一个字都不用动。例外是内核参数（如 ArchISO 的 `archiso_http_srv=`）必须给绝对 URL，
那段菜单项由程序生成，不用手写。

**只用英文。** iPXE 的 `menu`/`item` 按**字节数**算列宽：中文在 UTF-8 里 3 字节，高亮条会被算得比文字长、
分隔线断成两截（看起来像「断层」）。所以 `menu`/`item` 标签只用 ASCII，中文只放 `echo` 里。

**HTTP 默认端口 80**（与正版 TinyPXE 一致，浏览器直接 `http://<服务器>/` 就能看首页）。
早期用过 `8080`（太容易被占）和 `18080`（得先知道号码），最终回到 80；端口在设置页随时可改。

## 一次只能服务一个网段

配置里只有一份 `server_ip` / 地址池 / 广播地址，所以**同一时间只能服务一个网段**。
DHCP 收包 socket 绑在 `server_ip:67` 上（原因见下节），客户端接在别的网卡上时它的广播进不了这个 socket ——
日志里**一个包都不会出现**，看起来就像「程序坏了」。

切换网段：设置页「网卡」选对应那条 → 保存配置 → 停/启服务。启动成功后会检查本机还有哪些别的网段并提示：

```
提示：还有别的网段（vEthernet (Default Switch)=172.19.128.1、以太网=192.0.2.10），
     接在那些网卡上的客户端拿不到 IP（可在「网卡」里切换）
```

（没有载波的网卡不会出现在这条提示里。）

## 广播应答为什么必须从「绑定到本机 IP」的 socket 发出

Windows 对目的地址 `255.255.255.255` 的报文**按路由表挑出口网卡**，不是按你指定的。
把 socket 绑在 `0.0.0.0:67` 时，回给客户端的广播经常从另一块网卡（比如 Hyper-V 虚拟网卡）出去，
源 IP 变成 `192.168.100.1`；客户端一看不是自己网段的服务器就直接丢掉 —— 现象是
「客户端一直重发 DISCOVER，日志里有 OFFER，但客户端毫无反应」。

本项目把 udp:67 绑到 `server_ip` 上；只有当地址不属于本机网卡时才退回 `0.0.0.0`（并在日志里警告）。
pktmon 实测：

| 绑定方式 | 255.255.255.255 应答的出口网卡 |
| --- | --- |
| `0.0.0.0:67` | `02:00:00:00:00:01`（vEthernet，源 IP 192.168.100.1）❌ |
| `192.168.5.1:67` | `02:00:00:00:00:02`（以太网，源 IP 192.168.5.1）✓ |

顺带一个反直觉的结论：**不要加 SO_REUSEADDR**。在 Windows 上它会让第二个进程悄悄绑上已在用的
`0.0.0.0:67`，用户再也看不到「端口被占用」的报错。
