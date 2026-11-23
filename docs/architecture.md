# 架构与协议实现

## 两个可执行文件

共享同一套 `internal/` 内核，**不可同时运行**（均占用 `udp:67 / 69 / 4011`）：

| | `PxeServer.exe` | `pxed.exe` |
| --- | --- | --- |
| 入口 | `main.go`（mygo 原生界面） | `cmd/pxed/main.go`（无界面 CLI） |
| 日志 | 界面 + `PXE-server.log`（>2 MB 轮转 `.old`） | stdout / stderr |
| 启动参数 | `-capture <png>` `-debug-port <port>` `-firewall-apply` `-firewall-remove` | `-config` `-root` `-no-http` `-selftest` `-version` |
| 服务行为 | 按配置决定是否自启（默认手动点启动） | 总是立刻启动 |

## DHCP / ProxyDHCP 核心实现

- **手写编解码**（`internal/dhcp/packet.go`）：精确控制字节级选项集合。ProxyDHCP 模式下 `yiaddr=0`、不发 Option 1/3/51，仅发 60/66/67/43；仅在 `option 60` 以 `PXEClient` 开头时应答，并同时监听 **UDP 4011**（普通模式与代理模式均监听 4011 并单播回复请求方）。
- **架构分流（Option 93）**：`0` → `undionly.kpxe`（BIOS）、`6/7/9` → `snponly*.efi`（UEFI）、`10/11` → ARM64，未识别回落全局 `boot_file`（默认 `snponly.efi`）。
- **BIOS 与 UEFI 差异**：`arch < 6` 的 BIOS 客户端一律不发 43/60/66/67，只填 BOOTP `file` 字段（避免老 ROM 触发 `PXE-E74`）；UEFI 默认受 `no_option43` 控制不发 43。
- **兼容性与校验**：
  - OFFER/ACK 同时写 `option 67` 和 BOOTP 固定 `file` 字段（Hyper-V UEFI 只读 `file`）。
  - REQUEST 中 `option 54` 指向他机则忽略；地址被占、越界或为保留地址则回 **NAK**（Option 56 附原因）。
  - `DECLINE` 冲突地址隔离 60 秒；内存租约池不落盘、无 ARP 探测。
  - `user-class` 含 `iPXE` 时下发菜单 URL，传统 PXE ROM 下发引导固件名；无 `option 60/93` 的普通 DHCP 客户端只分配 IP，不下发引导文件。
- **TFTP**：每传输分配临时端口，监听 `udp:69` 仅处理 RRQ；启用 `blksize=1468` + `tsize`。

## 启动链路：TFTP → HTTP

TFTP 为停等协议（1~3 MB/s），大文件（如 700+ MB `boot.wim`）耗时极长；HTTP 流式传输可达本机 300+ MB/s：

1. 固件通过 TFTP 拉取 `snponly.efi`（约 1 MB）；
2. iPXE 启动后执行 `autoexec.ipxe`：`chain tftp://${next-server}/httpboot.ipxe`（兜底 `|| chain tftp://${next-server}/menu.ipxe`）；
3. `httpboot.ipxe` 由程序每次启动自动生成，内容为 `chain http://<服务器>/menu.ipxe`；
4. 后续菜单内以 `/` 开头的相对路径均由 iPXE 自动拼接为 HTTP 请求。

## 单网段限制与广播 socket 绑定

同一时间仅服务一个网段。切换网段需在主页「网卡」重新选择并重启服务；服务启动后会提示本机其他有载波的网段。

Windows 对目的地址 `255.255.255.255` 的广播包**按路由表选择出口网卡**。若 socket 绑定 `0.0.0.0:67`，应答广播常从无关虚拟网卡（如 Hyper-V vEthernet）发出，导致源 IP 不匹配而被客户端丢弃。因此服务端将 `udp:67` 绑定到具体 `server_ip`，且开启 `SO_BROADCAST`、**不启用 `SO_REUSEADDR`**（防止多进程静默抢占端口）。`server_ip` 不属于本机网卡时**不退回 `0.0.0.0`**：那样虽然能绑上，但应答里的 Option 54 / Option 66 仍是 `server_ip`，客户端续租与取引导文件都会打空，现场表现为「服务在跑但卡在 `Configuring`」。端口被占用时回退也绑不上，所以两种平台一致：启动直接报错并提示重选「网卡」。

Linux 的 DHCP 服务通过 `0.0.0.0` 接收 `udp:67` 与 `udp:4011` 流量，用 `IP_PKTINFO` 核对入口网卡，并从 `server_ip` 对应的网卡和源 IP 发出应答。`server_ip` 未配置在本机时启动直接报错。
