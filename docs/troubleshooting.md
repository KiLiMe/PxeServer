# 排错手册

## 1. 客户端反复 `DHCPDISCOVER`、始终 `0.0.0.0`

- **现象**：日志不断输出 `DHCPd：MAC=… DHCPDISCOVER → OFFER …`，但始终没有 `REQUEST`/`ACK` 和 `TFTPd：` 请求。
- **原因**：开启了 `ProxyDHCP`，但当前网段没有其他 DHCP 服务器；代理模式只下发 PXE 启动信息，不分配 IP。
- **解法**：取消勾选 `ProxyDHCP`（切换为普通 DHCP），保存后重启服务。

## 2. UEFI 文件下完后卡死在厂商 Logo / 转圈

- **现象**：iPXE 菜单正常，`.wim` 等文件全部下载完成后屏幕卡死，服务端不再收到 WinPE 的后续 DHCP 请求。
- **原因**：使用了全驱动版 `ipxe.efi`。它在启动时调用 `DisconnectController()` 接管网卡，退出时调用 `ExitBootServices()` 后部分固件无法恢复网卡状态导致挂死。
- **解法**：改用 **`snponly.efi`**（只通过固件 `EFI_SIMPLE_NETWORK_PROTOCOL` 收发包，退出时网卡状态不变）。
- **辨别手里的文件**：按 Latin-1 搜索字符串，全驱动版含 `i219`/`net80211`（约 1.1 MB），`snponly` 版含 `SNPONLY`（约 200~300 KB）。

## 3. Legacy BIOS 报 `PXE-E74` 后退出

- **现象**：老式 PXE ROM 屏幕报 `PXE-E74: Bad or missing PXE menu and/or prompt information` 后 `PXE-M0F` 退出，或不取文件直接去问 `udp:4011`。
- **原因**：BIOS/Legacy 固件看到 Option 43 或 Option 60/66/67 时会进入严格「PXE 发现」流程，对菜单子选项格式极度挑剔。
- **内置机制**：服务端对 `arch < 6` 的 BIOS 客户端已强制走最小应答——**不发 Option 43/60/66/67**，仅写 BOOTP 固定字段 `file` 与 `siaddr`（回归测试见 `internal/dhcp/option43_test.go` 的 `TestLegacyGetsMinimalReply`）。若仍异常，检查是否通过「额外 DHCP 选项」手工注入了冲突选项。
