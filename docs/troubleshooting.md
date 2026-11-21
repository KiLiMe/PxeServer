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

## 4. 移动了程序目录后，防火墙状态变成「未放行」

- **现象**：把 `PxeServer.exe` 换到别的目录后，设置页状态变成「未放行」，下面提示「程序已移动（原路径 …），请重新放行」；客户端拿不到 IP 或文件。
- **原因**：放行规则绑的是程序的绝对路径（`-Program <exe 路径>`），旧规则仍指向旧位置；`config.json` 也在 exe 同目录，换目录后是全新配置，放行标记为空。
- **解法**：在新位置点一次「设置 → 放行程序」，同名旧规则会先删后建，不会重复；然后按新目录重设启动目录与地址池。

## 5. 双击第二次没反应

- **现象**：已经开着一个界面时再启动一个，新进程一闪而过（Linux 终端里只印一句「已有一个 PXE 启动服务器在运行。」）。
- **原因**：单实例保护生效：第二个进程抢不到单实例锁（Windows 命名互斥体 `Local\PXE Server`，Linux `runtime_dir/pxe-server.lock` 上的 `flock`），退出。
- **解法**：无需处理；Windows 会把已有窗口拉前台（Linux 不做窗口激活，自己切过去）。要同时跑两个服务端，请用 `pxed` 并改端口/启动目录（两份会争 67/69/80，默认配置下第二份必然失败）。

## 6. Linux 上服务起不来：`bind: permission denied`

- **现象**：日志里 `DHCPd：` / `TFTPd：` 报绑定 `:67` / `:69` 失败，服务按钮弹回未运行；`pxed` 直接退出。
- **原因**：低于 1024 的端口是特权端口，普通用户绑不了（Windows 没这个限制，所以界面里不会提示）。
- **解法**：任选一种——
  - `sudo setcap 'cap_net_bind_service=+ep' /usr/bin/pxeserver`（或你解压/编译出来的路径），之后普通用户可直接运行；用 `.deb` / `.rpm` 装的升级后要重设。
  - `sudo ./pxeserver` / `sudo ./pxed`（注意 `config.json` 与 `PXE-server.log` 会落在当前目录，且归 root 所有）。
  - 用 systemd 服务并加 `AmbientCapabilities=CAP_NET_BIND_SERVICE`。
- **附带**：Linux 的防火墙按端口而不是按程序，界面不提供「放行程序」；开着 firewalld 的发行版（Fedora / RHEL / openSUSE）要自己开：`sudo firewall-cmd --add-service=dhcp --add-port=69/udp --add-port=80/tcp`（HTTP 端口按你配的改）。ufw 同理：`sudo ufw allow 67/udp` 等。
