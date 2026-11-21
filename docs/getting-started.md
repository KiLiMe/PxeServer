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
go run ./cmd/pxed -selftest        # 同一套自检，无需 root
go run .                           # 桌面界面（需 GTK3 + WebKitGTK，发行版包名如 webkit2gtk-4.1）
sudo setcap 'cap_net_bind_service=+ep' ./build/bin/pxeserver   # 然后就能普通用户起服务
```

- 67 / 69 / 80 是特权端口：不 `setcap` 就得 `sudo` 跑，否则日志报 `bind: permission denied`，详见 [troubleshooting.md](troubleshooting.md) 第 6 节。
- 界面「设置」里没有防火墙那一段：Linux 防火墙按端口管理，自己开 `dhcp` 服务与 `69/udp` 即可。
- 单实例保护照旧，只是第二次启动只提示、不把窗口拉前台。

## 虚拟机实测

1. 按 [boot-files.md](boot-files.md) 准备 `snponly.efi`（UEFI）或 `undionly.kpxe`（BIOS）与 `menu.ipxe`，放进 `boot/`。
2. 新建虚拟机，确保网卡支持 PXE（Hyper-V Gen1 需选旧版网络适配器，Gen2 直接支持；VMware 直接支持）。
3. Windows 入站流量需过防火墙：在界面「设置 → 放行程序」一键添加规则（详见 [gui.md](gui.md)）；Linux 按端口自己开，见 [troubleshooting.md](troubleshooting.md) 第 6 节。
4. 启动服务端并开机虚拟机，观察日志：`DHCPd：MAC=… DHCPDISCOVER → OFFER …` → `TFTPd：GET …` → 进入 iPXE 菜单。
5. 若卡在 `Configuring` 或 `0.0.0.0`，见 [troubleshooting.md](troubleshooting.md)。
