# AGENTS.md

本文件只写**约定**与**路径**。原理、背景、排错、方案对比一律放 `docs/`，不要写进这里。

## 路径

| 路径 | 作用 |
| --- | --- |
| `main.go` | 唯一的主包文件：mygo 启动、窗口装配、提权分支（`-capture` 截图） |
| `deps_test.go` | 依赖方向的机械校验（`go list` + 源码扫描） |
| `cmd/pxed/main.go` | 无界面入口；`cmd/pxed/helpers.go` 为 `-selftest` 自检 |
| `internal/app/app.go` | `App`：生命周期、`Status`、关于信息；同目录 `services.go` 起停服务、`config.go` 配置持久化、`menu.go` 菜单读写、`logs.go` 日志、`dialogs.go` 文件与目录选择、`firewall.go` 防火墙包装、`debugapi.go` 本地调试接口（仅 `-debug-port` 开启，只监听 `127.0.0.1`） |
| `internal/ui/` | mygo 原生界面：`ui.go` 是状态、调度与通用控件，`home.go`/`settings.go`/`menu.go`/`arch.go`/`logs.go`/`leases.go`/`about.go` 每页一个文件（只调 `App` 的导出方法） |
| `internal/config/config.go` | `Config`（字段 = 界面控件）、IP 探测；`path.go` 为 `DefaultPath()` |
| `internal/config/extra.go` | 额外 DHCP 选项解析 |
| `internal/logbook/logbook.go` | 日志级别（`I`/`W`/`E`）与内核包的 `Logger` 回调签名 |
| `internal/locales/zh-Hans.json` / `en.json` | 界面文案（源语言 / 英文），`catalog_gen.go` 为 `go tool mygo generate` 生成物；见 `docs/dev-release.md` 的「界面文案（i18n）」 |
| `internal/dhcp/packet.go` | 手写 BOOTP/DHCP 编解码（236 头 + cookie + TLV） |
| `internal/dhcp/server.go` | DHCP / ProxyDHCP 应答、租约池、option 43/66/67 |
| `internal/dhcp/broadcast_{windows,unix}.go` | `SO_BROADCAST` |
| `internal/firewall/firewall_windows.go` | 防火墙策略：放行 / 撤销 / 状态 / 提示（`rule_` 规则、`elevate_` 提权、`marker_` 标记） |
| `internal/firewall/firewall_other.go` | 非 Windows 的占位实现（`Supported()` 恒为假，界面因此隐藏防火墙那一段） |
| `internal/single/single_{windows,linux,other}.go` | 单实例保护：Windows 命名互斥体 `Local\PXE Server` + 拉前台；Linux `flock` 锁文件 `runtime_dir/pxe-server.lock`，只提示不激活窗口 |
| `internal/netif/netif.go` | 网卡枚举（界面「网卡」下拉数据源） |
| `internal/shell/shell_{windows,other}.go` | 用系统默认程序打开文件 |
| `internal/tftp/server.go` | 只读 TFTP，blksize 1468 + tsize |
| `internal/httpd/server.go` | HTTP 静态服务（wimboot / iPXE kernel / `/iso/`） |
| `internal/isorom/iso.go` | ISO9660 只读视图（ISO 直连已剥离，仅余后端；见 `docs/scope.md`） |
| `boot/menu.ipxe` | 示例 iPXE 菜单（**唯一入库**的 boot 文件） |
| `Scripts/build.ps1` / `Scripts/release.ps1` | 本地构建（`-Test` 跑 vet+test，`go tool mygo build` 出产物，`-Version X.Y.Z` 写版本资源，`-Target windows|linux` 选平台）/ 打标签发版（`Scripts/release.ps1 X.Y.Z [-Push]`）；脚本一律放 `Scripts/` |
| `mygo.json` | 打包配置：名称 / 标识 / 版本占位（`0.0.0-dev`，`-Version` 构建时临时改写再还原，deb/rpm 文件名也按它生成）/ 图标 / 产物目录 |
| `.github/workflows/ci.yml` / `release.yml` | `ci.yml` 跑分支 / PR 的格式与测试；`release.yml` 仅语义化版本标签（`vX.Y.Z`，可带 `-alpha/-beta/-rc`）触发，Windows + Linux 双平台矩阵构建后统一发布 |
| `config.json` | 本地运行态，已 gitignore，不入库 |
| `docs/` | 全部说明文档，索引见根 `README.md` |

## 约定

### 代码质量
- 始终以干净、模块化的方式写代码：架构合理、职责分离，新逻辑放到语义最近的位置，而不是随手塞进现有文件。
- 不为满足单次需求硬塞功能；找不到自然的落点、或认为需要先重构才干净时，**先停下说明理由**，确认后再动手。

### 语言与文本
- 仓库全部文档、注释、日志、提交信息用**简体中文**；iPXE 菜单、协议字段名保持英文。
- 界面文字（按钮、标签、弹框、校验错误）一律走 `internal/locales`：改文案先改 JSON 再跑 `go tool mygo generate`；新增带参消息要在源语言里声明 `@key.placeholders`，并同步 `en.json`。日志保持中文不译（`docs/dev-release.md` 的「界面文案（i18n）」）。
- 换行全仓 LF（`.gitattributes` 已声明 `* text=auto eol=lf`）；二进制扩展名标 `binary`。
- 提交信息用 `type(scope): 摘要`，如 `feat(ui): …`、`docs: …`、`ci: …`、`fix(release): …`。

### Go 代码
- 模块名 `pxeserv`，Go 1.27.3；依赖只有 `egoist/mygo`、`kdomanski/iso9660`、`pin/tftp/v3`、`golang.org/x/sys`。**不要随意加依赖**。
- 内核与 UI 解耦：`internal/*` 不 import 主包，也不直接打印；日志一律走 `internal/logbook.Logger` 回调。级别由写日志的一端标注：`a.Logf`（信息）、`a.Warnf`（警告，客户端侧失败、端口没绑上这类）、`a.Errorf`（本次运行真的坏了）。`deps_test.go` 机械校验“不 import 主包、不直接打印”这两条。
- 布局固定：根目录只放 `main.go`（+ `deps_test.go`）。界面代码放 `internal/ui`（只调 `App` 的导出方法，不 import 内核包）；服务编排、配置持久化、日志、菜单、选文件对话框、调试接口放 `internal/app`（按职责分文件，别堆回一个文件）；能被独立测试的纯逻辑放各自的 `internal/<名字>` 包。新功能不要往根目录加文件。
- 平台差异按文件后缀分：`*_windows.go`、`*_linux.go`，其余平台落实在 `*_other.go`（构建约束 `!windows && !linux`），对外只暴露同一个函数名，调用处不写 `runtime.GOOS` 分支。
- 日志前缀统一 `DHCPd：` / `TFTPd：` / `HTTPd：`（**全角冒号**），其后接句子；术语用中文（`架构`、`地址池`），MAC 保持 `MAC=`。
- `Config` 结构体字段与界面控件一一对应；新增字段同步 `internal/config/config.go` 与界面，并考虑 `normalize` / 运行态（`runtime.*`）回落逻辑。
- 面向用户的错误信息写中文（`errors.New("…")` 亦同）。
- 测试：表驱动、`TestXxx` 命名、断言用标准库 `testing`，**不引测试框架**；DHCP 相关用构造报文字节流的方式验证，不依赖真实网络/管理员权限。新增逻辑要带测试。
- 提交前必须过 `go vet ./...` 与 `go test ./...`（`pwsh ./Scripts/build.ps1 -Test` 一并做）。
- 版本号：遵循语义化版本 `X.Y.Z`，细节与发版流程见 `docs/dev-release.md`。GUI 显示 `mygo.json` 的 `version`（`Scripts/build.ps1 -Version` 构建时写入，界面用 `mygo.App.Version()` 读回）；命令行用 `-X main.Version=`。不要硬编码版本。

### 资源与产物
- 不往仓库放二进制（`*.efi`、`*.kpxe`、`*.wim`、`*.iso` 等已被 `.gitignore` 排除）；仓库里没有引导文件，需要时按 `docs/boot-files.md` 自取。
- 临时文件/脚本放 `tmp_<用途>/`（已 gitignore），用完删除。
- 不提交 `config.json`、`debug-port.txt`、`firewall-ok.txt`、`build/bin/`。
- 不留 `rsrc_windows_*.syso`：图标 / 版本 / manifest 都由 `mygo build` 生成，残留的旧 syso 会静默盖掉它们（`Scripts/build.ps1` 会先删）。

### 文档
- 细节写进 `docs/`：新增或修改行为时同步对应篇章，并在根 `README.md` 的文档表补/改一行。
- 文档间用相对链接互引；排错先指向 `docs/troubleshooting.md`，能力边界指向 `docs/scope.md`。
- 常用命令、环境准备等操作说明写在 `docs/getting-started.md`，本文件不再维护命令清单。