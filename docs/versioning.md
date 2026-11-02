# 版本号与发版

## 规则

语义化版本 `X.Y.Z`：

| 改动性质 | 升哪一位 | 例子 |
| --- | --- | --- |
| 不兼容（配置字段改名、命令行语义变了、协议行为变了） | 主版本 `X` | `0.4.1` → `1.0.0` |
| 兼容性新增功能 | 次版本 `Y` | `0.4.1` → `0.5.0` |
| 修 Bug | 修订号 `Z` | `0.4.1` → `0.4.2` |

- 升级高位时低位归零：`1.9.3` 加新功能是 `1.10.0`。
- 预发布在末尾加 `-alpha` / `-beta` / `-rc`，可带序号：`0.5.0-rc.1`。
- 构建信息用 `+`，不参与比较：`0.5.0+9f3c1ab`。
- `0.y.z` 表示初始开发阶段、接口还不稳定。

发版标签是 `v` + 版本号，例如 `v0.5.0`。早期手写的 `v1.0.0`/`v1.1.0` 与时间戳标签（`v2026.11.7.1519`）
已在 `v0.1.0` 发布时连同旧 Release 清理，现在远端只剩语义化标签，别在旧号上继续接。

## 发版

```pwsh
pwsh ./Scripts/release.ps1 0.5.0        # 只打本地标签，先看一遍
pwsh ./Scripts/release.ps1 0.5.0 -Push  # 打标签并推送，Actions 开始构建发布
```

脚本做的事：校验版本格式 → 拒绝脏工作区 → 拒绝已存在的标签 → 打印最近三个标签 → `git tag -a v0.5.0`。
`-Push` 才推送到远端。

## CI 与 Release

| workflow | 触发 | 做什么 |
| --- | --- | --- |
| `ci.yml` 的 `check` | push 到 `main`/`master`、PR、版本标签 | `gofmt -l`、`go vet ./internal/...`、`go tool mygo generate` 后比对 `catalog_gen.go`、`go test -race ./internal/...`（ubuntu） |
| `ci.yml` 的 `build` | **仅** `v*` 标签 | Windows：vet / test / `build.ps1 -Version <tag>` / `go build ./cmd/pxed` / `pxed.exe -selftest`，产物存 artifact（7 天） |
| `release.yml` | **仅** `v*` 标签 | 同上一套构建，再打包 `PxeServer-<tag>-windows-amd64.zip`（两个 exe + README + LICENSE）+ `SHA256SUMS.txt`，`gh release create` 上传（带 `-` 的标签自动 Pre-release） |

标签过滤只能做 glob（`v[0-9]*`），真正的格式校验在两个 workflow 第一步（严格正则，三段 `a.b.c`、
可选 `-alpha/-beta/-rc` 与 `+构建信息`），不符就报错退出。

## 版本号怎么进 exe

| 位置 | 内容 |
| --- | --- |
| 文件属性 `FileVersion` / `ProductVersion` | `mygo.json` 里的原始字符串（`0.5.0-rc.1` 就是 `0.5.0-rc.1`） |
| 固定数值 `dwFileVersion` / `dwProductVersion` | 四段 `X.Y.Z.0`，`-` 之后被切掉 |
| 界面「关于」与启动日志 | GUI 用 `mygo.App.Version()` 读回注入值 |
| `pxed.exe -version` | `-X main.Version=` 注入值 |

`Scripts/build.ps1 -Version X.Y.Z` 在构建前改写 `mygo.json`、构建后还原；开发构建是 `0.0.0-dev`。

## 注意

- `+构建信息` 不能进 `mygo.json`：mygo 会原样写进文件属性，而四段数字解析遇到 `+` 会把那一段算成 0
  （`1.2.1+build` → `1.2.0.0`）。`build.ps1` 已用 `-split '\+'` 去掉，手工传 `-Version` 时别带 `+`。
- 预发布之间不排序：`0.5.0-rc.2` 与 `0.5.0-rc.10` 谁高由人判断。
