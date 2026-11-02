# 桌面界面

原生界面由 `github.com/egoist/mygo` 绘制（系统标题栏、Win11 风格卡片、可滚动配置页），
不需要 Node / WebView2 / Wails。代码在 `internal/ui/ui.go`（只调用 `App` 的导出方法），
装配在 `main.go`，服务编排与持久化在 `internal/app`。

## 六个选项卡

| 页 | 内容 |
| --- | --- |
| 设置 | 语言、服务开关（ProxyDHCP / HTTPd）、根目录与 HTTP 端口、网卡与 IP、启动文件、更多选项 |
| 租约 | 当前 DHCP 租约 |
| 启动菜单 | 全高编辑器 + 文件名下拉 + 重新载入 / 保存 / 用编辑器打开 |
| 架构 | Option 93 架构 → 启动文件映射 |
| 日志 | 环形 500 行，按来源着色，「只看故障线索」筛 `W`/`E`，可导出 |
| 防火墙·关于 | 放行 / 撤销放行 / 状态、版本与依赖信息 |

顶部有状态徽章与启动 / 停止 / 保存配置按钮；配置改了没保存会显示「有未保存的改动」。

设置页的「更多」折叠区里有：Option 17 根路径、Option 60 ClassID、Option 66/67 覆盖值、Option 252、
额外选项、Option 43 子项 1、EFI 不发 Option 43、自动启动服务、调试接口。

## 额外 DHCP 选项

语法 `code[.sub]=值`，多项用 `;`（或 `,`、换行）分隔；值支持 `ip:1.2.3.4`（也可写裸 IP）、`str:文本`、
`hex:0102`（可带空格）、`u8:3` / `u16:3600` / `u32:7200`。带 `.sub` 时按子选项编码（`子选项号 + 长度 + 值`），
同一个 `code` 的多个子项自动拼接。

例：`175.6=ip:1.8.1.1; 175.8=ip:192.168.5.1` → option 175 = `06 04 01 08 01 01 08 04 C0 A8 05 01`（联想/惠普机型常用）。

- 额外选项在应答里**最后写入**，可覆盖同名标准选项，但拒绝覆盖 53（消息类型）、54（服务器标识）、52（超时）。
- 写法错误在「保存配置」时就报错；启动时日志会列出已生效的选项号。
- **Option 43 子项 1 = 0.0.0.0**：勾上后 Option 43 前面多出 `01 04 00 00 00 00`，少数老式 BIOS PXE 网卡需要它。

实现：`internal/config/extra.go`（`ParseExtraOptions` / `decodeOptionValue` / `bigEndianValue`）、
`internal/dhcp/server.go` 的 `applyExtras` 与 `pxeVendorOptions`。

## 运行日志

日志同时写进 exe 同目录的 `PXE-server.log`（超过 2 MB 轮转成 `.log.old`），每行带完整日期与单字母级别
（`I` 信息 / `W` 警告 / `E` 错误），每次启动先写一行 `======== <时间> 启动 v<版本> ========`，
崩溃或没开界面时也能事后排查。

按「一次请求一行」写：DHCP 把「收到什么 + 怎么应答」拼在一行
（`DHCPd：MAC=… DHCPDISCOVER → OFFER 192.168.5.2，启动文件 snponly.efi（TFTP/HTTP 192.168.5.1）`），
TFTP 传完才打一行（带体积与耗时）。同一客户端 5 秒内反复重发同样的请求会合并成 `（同 MAC 第 5 次）`。

运行态（调试接口地址、防火墙放行标记）存在 `config.json` 的 `runtime` 段，不再单独生成
`debug-port.txt` / `firewall-ok.txt`（旧文件启动时自动清掉）：

```json
"runtime": { "debug_addr": "127.0.0.1:54321", "firewall_exe": "C:\\Program Files\\PxeServer\\PxeServer.exe" }
```

## 本地调试接口

「启用本地调试接口」（`debug_api`，默认开）会让程序另起一个**只监听 `127.0.0.1`** 的 HTTP 服务，端口随机，
实际地址写进 `config.json` 的 `runtime.debug_addr`，界面上也显示、点一下即复制。
它不占任何 DHCP/TFTP 端口，也不对外暴露，用来让脚本（包括 AI 助手）直接观察和驱动正在运行的程序：

| 接口 | 作用 |
| --- | --- |
| `GET /status` | 运行状态 + 当前租约 |
| `GET /logs?n=200` | 最近 n 行日志 |
| `GET /config` | 当前生效配置 |
| `POST /start` `POST /stop` | 启动 / 停止全部服务 |
| `GET /selftest` | DHCP 报文编解码自检（不联网、不占端口） |
| `GET /decode?hex=…` | 把一段十六进制 DHCP 报文解出来 |
| `GET /tftp?file=menu.ipxe` | 用内置 TFTP 客户端从本机服务下载文件，返回大小与 SHA-256 |
| `GET /http?path=/menu.ipxe&range=0-15` | 通过本机 HTTP 取文件，返回状态码与首字节 |
| `GET /interfaces` | 本机网卡列表 |

```powershell
$addr = (Get-Content .\config.json -Raw | ConvertFrom-Json).runtime.debug_addr
Invoke-RestMethod "http://$addr/selftest" -NoProxy
```

本机若配了系统代理，访问 127.0.0.1 要显式 `-NoProxy`，否则请求会被代理吞掉。

接口只接受本机直连：带 `Origin` 头的请求（浏览器跨站）和 `Host` 与监听地址不符的请求（DNS 重绑定）
一律返回 `403 forbidden`。命令行、`curl`、`Invoke-RestMethod` 都不带 `Origin`，不受影响。

## 界面文案（中英文）

中文是源语言，缺译文时永远回落到它。语言在设置页选，存进 `config.json` 的 `language`
（`zh-Hans` / `en` / 留空=跟随系统），切换立即生效。

| 路径 | 作用 |
| --- | --- |
| `internal/locales/zh-Hans.json` | 源语言文案；带参消息用 `"@key": {"placeholders": …}` 声明参数类型 |
| `internal/locales/en.json` | 英文译文，key 与参数名必须一致 |
| `internal/locales/catalog_gen.go` | `go tool mygo generate` 生成物，**不要手改** |
| `internal/locales/locales_test.go` | 键集合、占位符两边一致性 |
| `mygo.json` 的 `i18n` 段 | `{"dir": "internal/locales", "source": "zh-Hans"}` |

加一条文案：源语言加 key（带参就补 `placeholders`）→ `en.json` 加同名 key → `go tool mygo generate`
→ `go tool mygo i18n check` → 代码里用 `locales.Xxx()`。复数用 ICU 的
`{count, plural, other {# 条}}`；英文才写 `one`/`other` 多档。

约定：

- 界面、弹框、按钮、字段标签、校验错误一律走 `internal/locales`；协议名与字段名不译
  （`DHCP`、`TFTP`、`ProxyDHCP`、`MAC`、`option252`、`server_ip`）。
- **运行日志不译**：`DHCPd：` / `TFTPd：` / `HTTPd：` 前缀与句子保持中文；界面「日志」页的故障线索按级别筛，不看文案。
- `cmd/pxed` 的 stderr 与 `-help`、`debugapi.go` 的 JSON 字段保持中文。
- 生成的 `httpboot.ipxe` 与菜单条目保持英文（iPXE 控制台字体），只有模板注释行跟随界面语言。

已知边界：CI 会跑 `go tool mygo generate` 并比对 `catalog_gen.go`，忘了生成会直接失败；
配置校验失败时程序改用默认配置并覆盖 `config.json`，`language` 会跟着丢。
