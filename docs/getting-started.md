# 运行与实测

## 运行方式

```pwsh
go run ./cmd/pxed -selftest   # 编解码自检：不联网、不占端口、无需管理员
go run .                      # 桌面界面；默认不自动启动服务（「主页 → 自动启动服务」可改）
go run ./cmd/pxed             # 命令行版：立刻起服务，首次在当前目录生成 config.json
```

桌面版（`PxeServer.exe` / `pxeserver`）与命令行版（`pxed.exe` / `pxed`）**不能同时运行**，均需占用 `udp:67 / 69 / 4011`。构建、调试接口与发版说明见 [dev-release.md](dev-release.md)。

## 在 Linux 上运行

```bash
./Scripts/build.sh                 # 构建：App + deb / rpm / PKGBUILD + tar.gz，出到 build/bin/linux-<arch>/（不需要 PowerShell）
go run ./cmd/pxed -selftest        # 同一套自检，无需 root
go run .                           # 桌面界面（需 GTK3 + WebKitGTK，发行版包名如 webkit2gtk-4.1）
sudo setcap 'cap_net_bind_service=+ep' ./build/bin/linux-amd64/pxeserver   # 然后就能普通用户起服务
```

- 构建脚本的完整参数（`-t` 跑测试、`-v` 写版本号、`-a` 换架构）与交叉构建见 [dev-release.md](dev-release.md) 的「本地构建」。
- 67 / 69 / 80 是特权端口：不 `setcap` 就得 `sudo` 跑，否则日志报 `bind: permission denied`，详见 [troubleshooting.md](troubleshooting.md) 第 6 节。界面顶部那块黄条右侧有「一键授权」，点一下走 pkexec（弹系统认证框）把能力打到当前可执行文件上，重启程序即生效。
- 用 `makepkg -si` 装的 Arch 包由 PKGBUILD 里的 `post_install` 自动授权，deb / rpm 由 `postinst` / `%post` 自动授权（注入逻辑在 `Scripts/patch-packages.sh`，构建时执行），装完直接启动即可。免安装包（`~/.local`）不能预先授权，用界面按钮或下面的 `setcap`。
- 界面「设置」里没有防火墙那一段：Linux 防火墙按端口管理，自己开 `dhcp` 服务与 `69/udp` 即可。
- 单实例保护照旧，只是第二次启动只提示、不把窗口拉前台。
- KDE Plasma 下，桌面端默认通过桌面门户使用 Qt 目录选择器。Linux 需有可用的 `xdg-desktop-portal-kde`，已有的 `GTK_USE_PORTAL` 设置优先。旧版可以用 `GTK_USE_PORTAL=1 ./build/bin/linux-amd64/pxeserver` 从终端启动。
- 启动文件可以放在已挂载的数据盘上。在主页选择目录并保存，或在桌面版 `config.json` 中把 `tftp_root` 设为绝对路径。TFTP 和 HTTP 共用该目录。命令行版也可以用 `pxed -root /mnt/data/pxe/boot` 指定。
- `server_ip` 必须是本机用于 PXE 的网卡 IPv4。Linux 按该网卡接收 DHCP 广播并发送应答，地址尚未配置时启动会报错。
- `pxed` 在 TFTP 或已启用的 HTTP 端口绑定失败时以非零状态退出。启用 HTTP 时，桌面端和 `pxed` 都会在启动目录生成 `httpboot.ipxe` 供 `autoexec.ipxe` 跳转到 HTTP 菜单。使用 `pxed -no-http` 时，DHCP 下发以 TFTP 为基准的菜单文件名。

## 虚拟机实测

1. 按 [boot-files.md](boot-files.md) 准备 `snponly.efi`（UEFI）或 `undionly.kpxe`（BIOS）与 `menu.ipxe`，放进 `boot/`。
2. 新建虚拟机，确保网卡支持 PXE（Hyper-V Gen1 需选旧版网络适配器，Gen2 直接支持；VMware 直接支持）。
3. Windows 入站流量需过防火墙：在界面「设置 → 放行程序」一键添加规则（详见 [gui.md](gui.md)）；Linux 按端口自己开，见 [troubleshooting.md](troubleshooting.md) 第 6 节。
4. 启动服务端并开机虚拟机，观察日志：`DHCPd：MAC=… DHCPDISCOVER → OFFER …` → `TFTPd：GET …` → 进入 iPXE 菜单。
5. 若卡在 `Configuring` 或 `0.0.0.0`，见 [troubleshooting.md](troubleshooting.md)。
