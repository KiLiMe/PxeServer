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
- **提前提示**：Linux 上程序启动时会读 `/proc/self/status` 的 `CapEff`（第 10 位 = `CAP_NET_BIND_SERVICE`），判断不了就退回按 `euid==0` 粗判（`internal/priv`）。没有该能力时：
  - 主页顶部直接画出这条提示（含可复制的 `setcap` 命令，路径是本进程的可执行文件）；
  - 点「启动」会被立刻拦下并弹提示，不会真去绑端口，配置仍照常保存；
  - `pxed` 在 `pxe.Start` 之前就往 stderr 打一行同样的提示（仍继续启动，后续错误处理不变）。
  - `setcap` 对已在运行的进程不生效，改完必须重启程序；`CapEff` 显示的是 effective 位，systemd 用 `AmbientCapabilities` 才能让这一位保留下来。
- **解法**：任选一种——
  - 点主页黄条右侧的「一键授权」：界面走 `pkexec`（polkit，桌面弹认证框）执行 `setcap`，能力打在当前可执行文件上。密码输完即可，但当前进程的 `CapEff` 不会变——**重启程序**才生效，所以黄条会换成一条绿色的「已授权，重启后生效」。无桌面（SSH / 无 polkit agent）时 `pkexec` 不可用，弹框会直接说清楚，回退到下一种手工方式。
  - `sudo setcap 'cap_net_bind_service=+ep' /opt/pxeserver/pxeserver`（或你解压/编译出来的路径），之后普通用户可直接运行。用 `.deb` / `.rpm` / PKGBUILD 装的包在**安装与升级**时都已由包脚本自动授权（deb 的 `postinst`、rpm 的 `%post`、PKGBUILD 的 `post_install`/`post_upgrade`，由 `Scripts/patch-packages.sh` 注入），不用再手动跑 `setcap`。
  - `sudo ./pxeserver` / `sudo ./pxed`（注意 `config.json` 与 `PXE-server.log` 会落在当前目录，且归 root 所有）。
  - 用 systemd 服务并加 `AmbientCapabilities=CAP_NET_BIND_SERVICE`。
- **附带**：Linux 的防火墙按端口而不是按程序，界面不提供「放行程序」；开着 firewalld 的发行版（Fedora / RHEL / openSUSE）要自己开：`sudo firewall-cmd --add-service=dhcp --add-port=69/udp --add-port=80/tcp`（HTTP 端口按你配的改）。ufw 同理：`sudo ufw allow 67/udp` 等。

## 7. 启动时报 `无法绑定 <IP>:67`（服务器 IP 不是本机网卡地址）

- **现象**：Windows 上点「启动」后弹「DHCP 服务启动失败」，正文含 `无法绑定 192.168.5.1:67（请确认「服务器 IP」是本机某块网卡上的地址）：listen udp4 … bind: …`。
- **原因**：`config.json` 里的 `server_ip` 已不属于本机任何一块网卡——网卡 IP 被路由器重分、网卡被禁用/拔出、配置文件从别的机器拷来，或手工改过。服务端**不会**退回监听 `0.0.0.0:67`：那样虽然能绑上，但应答里的 Option 54 / Option 66 仍是这个地址，客户端会一直卡在 `Configuring`，比直接报错更难查。
- **解法**：回主页在「网卡」下拉里重新选择本机网卡（会同步 `server_ip` 与广播地址），保存后重启服务；或直接编辑 `config.json` 的 `server_ip`。

## 8. Linux 点「选择目录」没反应，程序随后退出

- **现象**：KDE 等桌面点「选择目录」后目录选择器始终不出现，过一会儿窗口消失；`journalctl --user` 里能看到
  `Gtk:ERROR:../gtk/gtk/gtkfilechoosernativeportal.c:532:gtk_file_chooser_native_portal_hide: assertion failed: (data != NULL)` 与 `Bail out!`。
- **原因**：可执行文件带 `cap_net_bind_service`（「一键授权」或 `setcap` 打上去的），而 `GTK_USE_PORTAL=1` 会让选择器改走桌面门户。`xdg-desktop-portal` 拒绝为带能力的进程服务——日志里是 `Portal operation not allowed: Unable to open /proc/<pid>/root`——门户选择器根本建不出来，GTK 在收尾时又断言失败，进程直接 `SIGABRT`。
- **解法**：不要设 `GTK_USE_PORTAL=1`（程序自己也不再设置它）。`XDG_CURRENT_DESKTOP` 认得出的桌面会用原生选择器：KDE / LXQt / DDE 用 `kdialog`，GNOME / XFCE / Cinnamon 等 GTK 桌面用 `zenity`；两者都没装（或桌面不认识）时用 GTK 内建对话框，功能一样、只是外观不跟随桌面。如果环境里已经设了 `GTK_USE_PORTAL`，`env -u GTK_USE_PORTAL ./pxeserver` 启动即可。带 `cap_net_bind_service` 的文件不要指望门户类对话框。
