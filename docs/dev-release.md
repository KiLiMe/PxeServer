# 开发、构建与发版

## 本地构建

```pwsh
pwsh ./Scripts/build.ps1 -Test        # go vet + go test，再 go tool mygo build → build/bin/PxeServer.exe
pwsh ./Scripts/build.ps1              # 同上，不跑测试
pwsh ./Scripts/build.ps1 -Target linux   # Linux 产物（build/bin/linux-<arch>/）；bash 里 pwsh 一样能跑
go build -o build/bin/pxed.exe ./cmd/pxed   # 命令行版（Linux 上去掉 .exe）
```

- `-Target windows|linux`（默认 `windows`）与 `-Arch`（默认跟随 `go env GOARCH`）选择产物。两个平台都能从任意平台交叉构建，不需要目标机器。
- `-Target linux` 一次产出：可执行文件 `pxeserver`、`pxeserver.desktop`、图标、`pxeserver_<版本>_amd64.deb`、`pxeserver-<版本>-1.x86_64.rpm`、`pxeserver-<版本>-linux-amd64.tar.gz`（内含 `install.sh`，解压后装到 `~/.local`）与 `PKGBUILD`，全部留在 `build/bin/linux-<arch>/`。
- 图标（`build/appicon.png`）、exe 版本资源、PerMonitorV2 manifest 均由 `go tool mygo build` 写入；`build.ps1` 会先清理残留的 `rsrc_windows_*.syso`，Windows 传 `-skip-installer -skip-dmg`、Linux 传 `-skip-dmg`。
- 升级 `mygo` 时需同步升级工具链依赖：`go get github.com/egoist/mygo@vX.Y.Z github.com/egoist/mygo/cmd/mygo@vX.Y.Z && go mod tidy`。
- 界面调试截图：`PxeServer.exe -capture screenshot.png`（启动约 2 秒后保存原生窗口并退出）。

## 本地调试接口

显式传入 `-debug-port <端口>` 启动，仅监听 `127.0.0.1`；不传或传 `0` 则不启动。实际地址写入 `config.json` 的 `runtime.debug_addr` 并显示在主页。

| 接口 | 作用 |
| --- | --- |
| `GET /status` | 运行状态 + 当前租约 |
| `GET /logs?n=200` | 最近 n 行日志 |
| `GET /config` | 当前生效配置 |
| `POST /start` `POST /stop` | 启动 / 停止全部服务 |
| `GET /selftest` | DHCP 报文编解码自检（不联网、不占端口） |
| `GET /decode?hex=…` | 解析十六进制 DHCP 报文 |
| `GET /tftp?file=menu.ipxe` | 用内置 TFTP 客户端下载文件，返回大小与 SHA-256 |
| `GET /http?path=/menu.ipxe&range=0-15` | 通过本机 HTTP 取文件，返回状态码与首字节 |
| `GET /interfaces` | 本机网卡列表 |

```pwsh
.\PxeServer.exe -debug-port 18080
$addr = (Get-Content .\config.json -Raw | ConvertFrom-Json).runtime.debug_addr
Invoke-RestMethod "http://$addr/selftest" -NoProxy   # 有系统代理时需加 -NoProxy
```

安全限制：带 `Origin` 头的请求（跨站）或 `Host` 与监听地址不符的请求（DNS 重绑定）一律返回 `403 forbidden`。

## 界面文案（i18n）

中文（`zh-Hans`）为源语言，缺译文时回落至此；设置页切换后存入 `config.json` 的 `language`（留空跟随系统）。

| 路径 | 作用 |
| --- | --- |
| `internal/locales/zh-Hans.json` | 源语言文案；带参消息用 `"@key": {"placeholders": …}` 声明 |
| `internal/locales/en.json` | 英文译文，key 与参数名必须与源语言一致 |
| `internal/locales/catalog_gen.go` | `go tool mygo generate` 生成物，禁止手改 |
| `internal/locales/locales_test.go` | 校验键集合与占位符一致性 |

新增文案流程：`zh-Hans.json` 加 key → `en.json` 补译文 → `go tool mygo generate` → `go tool mygo i18n check` → 代码调用 `locales.Xxx()`。复数使用 ICU `{count, plural, other {# 条}}`（英文需写 `one`/`other`）。

**翻译边界**：
- 界面控件、弹框、校验错误走 `internal/locales`；协议/字段名不译（`DHCP`、`TFTP`、`ProxyDHCP`、`MAC`、`option252`、`server_ip`）。
- 运行日志（`DHCPd：`/`TFTPd：`/`HTTPd：`）、`cmd/pxed` 输出与 `debugapi.go` JSON 字段保持中文。
- 生成的 `httpboot.ipxe` 与菜单条目保持英文，仅模板注释跟随界面语言。

## 语义化版本与发版

版本号遵循 `X.Y.Z`（不兼容升 `X`、兼容新增升 `Y`、修 Bug 升 `Z`，高位升级低位归零；`0.y.z` 为初始开发），支持 `-alpha`/`-beta`/`-rc` 预发布后缀与 `+` 构建信息。

```pwsh
pwsh ./Scripts/release.ps1 0.5.0        # 校验格式与干净工作区，打本地标签 v0.5.0
pwsh ./Scripts/release.ps1 0.5.0 -Push  # 推送标签触发 GitHub Actions 发布
```

### CI / Release 流程

| workflow | 触发条件 | 动作 |
| --- | --- | --- |
| `ci.yml` (`check`) | push `main`/`master`、PR | `gofmt -l`、`go vet ./internal/...`、`mygo generate` 一致性校验、`go test -race ./internal/...`（Linux） |
| `release.yml` (`meta`) | 仅 `v*` 标签 | 校验语义化版本标签，解出版本号与是否预发布 |
| `release.yml` (`build`) | 同上，`windows-latest` + `ubuntu-latest` 矩阵 | `build.ps1 -Target <平台> -Version …` + 构建 `pxed` + `-selftest` 冒烟 + 打包为 artifact（Linux 那次额外跑 `gofmt`/`vet`/`test`） |
| `release.yml` (`publish`) | 同上 | 汇总两平台资产、生成 `SHA256SUMS.txt`、创建 Release（含 `-` 自动标 Pre-release；重跑时覆盖资产） |

发布资产：

| 资产 | 平台 |
| --- | --- |
| `PxeServer-<tag>-windows-amd64.zip`（`PxeServer.exe` + `pxed.exe` + README / LICENSE） | Windows |
| `pxeserver_<版本>_amd64.deb` / `pxeserver-<版本>-1.x86_64.rpm` / `PKGBUILD` | Linux |
| `pxeserver-<版本>-linux-amd64.tar.gz` | Linux（`install.sh` 装到 `~/.local`） |
| `PxeServer-<tag>-linux-amd64-cli.tar.gz`（命令行 `pxed`） | Linux |
| `SHA256SUMS.txt` | — |

发布说明由 workflow 用 `git log` 自动生成，正文里固定提醒「压缩包不含引导二进制」（见 [boot-files.md](boot-files.md)）。

### 版本号注入

- **GUI (`PxeServer`)**：`Scripts/build.ps1 -Version X.Y.Z` 构建前临时改写 `mygo.json` 的 `version`（已自动剔除 `+` 构建信息），构建后还原；`mygo.App.Version()` 读回展示。开发构建为 `0.0.0-dev`。
- **Linux 包**：deb / rpm 的 metadata 与产物文件名同样按 `mygo.json` 的 `version` 生成，所以发版必须先过 `build.ps1 -Version`，不要直接 `go tool mygo build`。
- **CLI (`pxed`)**：通过 `-ldflags "-X main.Version=..."` 注入。
