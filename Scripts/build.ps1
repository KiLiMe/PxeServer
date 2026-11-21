#Requires -Version 7.0
[CmdletBinding()]
param(
    [switch]$Test,
    [string]$Version,
    # 目标平台：windows 出单文件 PxeServer.exe；linux 出 App、.deb、.rpm、PKGBUILD 与 install.sh 打包的 tar.gz。
    [ValidateSet('windows', 'linux')][string]$Target = 'windows',
    # 目标架构：默认跟随当前 go env GOARCH（Windows 上仍是 amd64）。
    [string]$Arch
)
$ErrorActionPreference = 'Stop'
# 脚本放在 Scripts/ 下，仓库根目录是它的上一级：所有相对路径都从 $root 出发。
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
$config = Join-Path $root 'mygo.json'
$original = $null
try {
    # 旧版用 rsrc 生成本地图标资源；mygo 自己写图标 / 版本 / manifest，留着旧 syso 会把它们盖掉。
    Get-ChildItem $root -Filter 'rsrc_windows_*.syso' -File | ForEach-Object {
        Remove-Item $_.FullName -Force
        Write-Host "已删除旧的图标资源：$($_.Name)"
    }
    # 界面文案的访问器（internal/locales/catalog_gen.go）是生成物：改完 *.json 忘了 generate
    # 就会拿到旧文案，所以构建前总是重新生成一遍。
    & go tool mygo generate
    if ($LASTEXITCODE -ne 0) { throw '生成界面文案失败' }
    if ($Test) {
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'go vet 失败' }
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'go test 失败' }
    }
    $bin = [IO.Path]::GetFullPath((Join-Path $root 'build/bin'))
    New-Item -ItemType Directory -Force $bin | Out-Null
    $arch = if ($Arch) { $Arch } else { (& go env GOARCH).Trim() }
    if ($Target -eq 'windows') {
        $output = Join-Path $bin 'PxeServer.exe'
        # 正在跑的界面占着输出文件，Windows 会拒绝覆盖（或被截成半截）：先说清楚是哪个 PID。
        $busy = @(Get-Process -Name 'PxeServer' -ErrorAction SilentlyContinue | Where-Object {
            try { $_.Path -eq $output } catch { $false }
        })
        if ($busy.Count -gt 0) {
            throw "正在运行的 PxeServer.exe（PID $(($busy.Id) -join ', ')）占着 $output：先关掉它的窗口再构建"
        }
    }
    # exe 的文件属性版本只来自 mygo.json（编译期写进资源）：构建时临时改写，结束后还原，工作区不留改动。
    # 参数可以是版本号，也可以直接给发版标签（v0.2.0）：Windows 版本资源只认数字段，
    # 所以去掉 v 前缀与 +构建信息（预发布后缀 -rc.1 由 mygo 自己按 "-" 切掉）。
    # Linux 包的版本号同样来自 mygo.json：deb/rpm 的 metadata 与产物文件名都按它生成。
    if ($Version) {
        $version = ($Version.TrimStart('v') -split '\+')[0]
        if ($version -notmatch '^\d+\.\d+\.\d+') { throw "版本号应以 X.Y.Z 开头，实际：$Version" }
        $original = Get-Content $config -Raw
        if ($original -notmatch '"version"\s*:\s*"[^"]*"') { throw 'mygo.json 里没有 version 字段' }
        Set-Content $config -Value ($original -replace '"version"\s*:\s*"[^"]*"', "`"version`": `"$version`"") -Encoding utf8NoBOM -NoNewline
    }
    if ($Target -eq 'windows') {
        & go tool mygo build -platform "windows/$arch" -skip-installer -skip-dmg -o $bin
        if ($LASTEXITCODE -ne 0) { throw 'GUI 构建失败' }
        $built = Join-Path $bin "windows-$arch/PxeServer.exe"
        if (-not (Test-Path $built)) { throw "构建产物缺失：$built" }
        Move-Item $built $output -Force
        # mygo 把产物放进 <out>/<平台>/，旁边还有升级清单 .mygo-files；这里只留 exe。
        Remove-Item (Join-Path $bin "windows-$arch") -Recurse -Force -ErrorAction SilentlyContinue
        # exe 文件属性版本只有 Windows 读得出来；在 Linux 上交叉构建 windows/amd64 时留空。
        $fileVersion = (Get-Item $output).VersionInfo.FileVersion
        $suffix = if ($fileVersion) { "（exe 版本 $fileVersion）" } else { '' }
        Write-Host "构建完成：$output$suffix"
    } else {
        # Linux：mygo 一次产出可执行文件 + .desktop + 图标 + deb / rpm / PKGBUILD / install.sh，
        # 以及用 install.sh 装到 ~/.local 的 tar.gz，全部留在 build/bin/linux-<arch>/，发版脚本直接取用。
        & go tool mygo build -platform "linux/$arch" -skip-dmg -o $bin
        if ($LASTEXITCODE -ne 0) { throw 'GUI 构建失败' }
        $dir = Join-Path $bin "linux-$arch"
        if (-not (Test-Path (Join-Path $dir 'pxeserver'))) { throw "构建产物缺失：$dir/pxeserver" }
        Write-Host "构建完成：$dir"
        Get-ChildItem $dir -File | Where-Object Name -ne '.mygo-files' | ForEach-Object {
            Write-Host ('  {0,-44} {1,8:N0} KB' -f $_.Name, ($_.Length / 1KB))
        }
    }
} finally {
    if ($null -ne $original) { Set-Content $config -Value $original -Encoding utf8NoBOM -NoNewline }
    Pop-Location
}
