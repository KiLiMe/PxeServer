#Requires -Version 7.0
# 按语义化版本生成并推送标签，形如 v0.2.0、v0.3.0-rc.1；推送后 GitHub Actions 自动构建并发布 Release。
# 规则见 docs/versioning.md：不兼容改动升主版本，兼容性新增功能升次版本，修 Bug 升修订号，升级高位时低位归零。
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)][string]$Version,
    [switch]$Push
)
$ErrorActionPreference = 'Stop'
# 脚本放在 Scripts/ 下，git 命令要在仓库根目录执行。
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    $version = $Version.TrimStart('v')
    # 预发布可加 -alpha/-beta/-rc（可再带 .序号）；构建信息用 +，不参与版本比较。
    $semver = '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-(alpha|beta|rc)(\.\d+)?)?(\+[0-9A-Za-z.-]+)?$'
    if ($version -notmatch $semver) {
        throw "版本号应为 X.Y.Z，可带 -alpha/-beta/-rc 或 +构建信息，实际：$Version"
    }
    if (git status --porcelain) { throw '工作区不干净，先提交再发版' }
    $tag = 'v' + $version
    if (git tag --list $tag) { throw "标签已存在：$tag" }
    # 把最近的标签列出来，升哪一位由人决定（版本线从 v0.1.0 重新起算，旧的 v1.x 与时间戳标签已清理）。
    $recent = @(git tag --list 'v*' --sort=-creatordate | Select-Object -First 3)
    if ($recent) { Write-Host ("最近的标签：" + ($recent -join '、')) }
    git tag -a $tag -m $tag
    if ($LASTEXITCODE -ne 0) { throw "打标签失败：$tag" }
    Write-Host "已创建标签 $tag"
    if ($Push) {
        git push origin $tag
        if ($LASTEXITCODE -ne 0) { throw "推送标签失败：$tag" }
        Write-Host "已推送，等待 GitHub Actions 构建：https://github.com/KiLiMe/PxeServer/actions"
    } else {
        Write-Host "确认无误后执行：git push origin $tag   或 pwsh ./Scripts/release.ps1 $version -Push"
    }
} finally {
    Pop-Location
}
