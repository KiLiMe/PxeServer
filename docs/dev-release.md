# 开发、构建与发版

## 本地构建

```pwsh
pwsh ./Scripts/build.ps1 -Test   # go vet + go test，再 go tool mygo build → build/bin/PxeServer.exe
pwsh ./Scripts/build.ps1         # 同上，不跑测试
go build -o build/bin/pxed.exe ./cmd/pxed   # 命令行版
```

- 图标（`build/appicon.png`）、exe 版本资源、PerMonitorV2 manifest 均由 `go tool mygo build` 写入；`build.ps1` 会先清理残留的 `rsrc_windows_*.syso`，并传入 `-skip-installer -skip-dmg` 跳过安装包生成。
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
| `ci.yml` (`check`) | push `main`/`master`、PR、`v*` 标签 | `gofmt -l`、`go vet ./internal/...`、`mygo generate` 一致性校验、`go test -race ./internal/...`（Linux） |
| `ci.yml` (`build`) | 仅 `v*` 标签 | Windows 上 `vet` / `test` / 构建双 exe / `pxed.exe -selftest` 冒烟 |
| `release.yml` | 仅 `v*` 标签 | 同上构建并打包 `PxeServer-<tag>-windows-amd64.zip` + `SHA256SUMS.txt` 创建 Release（含 `-` 自动标 Pre-release） |

### 版本号注入

- **GUI (`PxeServer.exe`)**：`Scripts/build.ps1 -Version X.Y.Z` 构建前临时改写 `mygo.json` 的 `version`（已自动剔除 `+` 构建信息），构建后还原；`mygo.App.Version()` 读回展示。开发构建为 `0.0.0-dev`。
- **CLI (`pxed.exe`)**：通过 `-ldflags "-X main.Version=..."` 注入。
