#!/usr/bin/env bash
# Linux 构建入口（不需要 PowerShell）：出 App、.desktop、图标、deb / rpm / PKGBUILD 与带 install.sh 的 tar.gz。
# Windows 与 CI 走 Scripts/build.ps1；两边只共用一个约定——产物都留在 build/bin/ 下，发版脚本直接取用。
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
bin="$root/build/bin"
config="$root/mygo.json"
backup=""
run_tests=0
version=""
arch=""

usage() {
    cat <<'EOF'
用法：Scripts/build.sh [选项]

  -t, --test            构建前跑 go vet ./... 与 go test ./...
  -v, --version X.Y.Z   临时改写 mygo.json 的 version（deb / rpm 的文件名与 metadata 按它生成），构建后还原
  -a, --arch ARCH       目标架构，默认跟随 go env GOARCH
  -h, --help            显示本帮助
EOF
}

die() {
    printf '错误：%s\n' "$1" >&2
    exit 1
}

while (($#)); do
    case $1 in
    -t | --test) run_tests=1 ;;
    -v | --version)
        version=${2-}
        [[ -n $version ]] || die "$1 需要一个版本号，如 0.2.0"
        shift
        ;;
    -a | --arch)
        arch=${2-}
        [[ -n $arch ]] || die "$1 需要一个架构，如 amd64"
        shift
        ;;
    -h | --help)
        usage
        exit 0
        ;;
    *)
        usage >&2
        die "未知参数：$1"
        ;;
    esac
    shift
done

cd "$root"

# 参数先校验完再干活：参数写错就别白白 generate 一遍。
# 参数可以是版本号，也可以直接给发版标签（v0.2.0-rc.1）：去掉 v 前缀与 +构建信息，
# 预发布后缀 -rc.1 由 mygo 自己按 "-" 切开。deb / rpm 的版本号同样来自 mygo.json。
if [[ -n $version ]]; then
    label=$version
    version=${version#v}
    version=${version%%+*}
    [[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+ ]] || die "版本号应以 X.Y.Z 开头，实际：$label"
    grep -qE '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$config" || die 'mygo.json 里没有 version 字段'
fi

# 版本号只在构建期间改写：无论成败都还原，工作区不留改动。
restore_config() {
    if [[ -n $backup && -f $backup ]]; then
        mv -f "$backup" "$config"
    fi
}
trap restore_config EXIT

if [[ -n $version ]]; then
    backup=$(mktemp)
    cp -f "$config" "$backup"
    sed -E "s/(\"version\"[[:space:]]*:[[:space:]]*)\"[^\"]*\"/\1\"$version\"/" "$backup" >"$config"
fi

# 旧版用 rsrc 生成本地图标资源；mygo 自己写图标 / 版本 / manifest，留着旧 syso 会把它们盖掉。
for syso in "$root"/rsrc_windows_*.syso; do
    [[ -e $syso ]] || continue
    rm -f "$syso"
    printf '已删除旧的图标资源：%s\n' "$(basename "$syso")"
done

# 界面文案的访问器（internal/locales/catalog_gen.go）是生成物：改完 *.json 忘了 generate 就会拿到旧文案。
go tool mygo generate

if ((run_tests)); then
    go vet ./...
    go test ./...
fi

arch=${arch:-$(go env GOARCH)}

# 构建前改写 mygo.json 的 version，trap 里还原。
if [[ -n $version ]]; then
    backup=$(mktemp)
    cp -f "$config" "$backup"
    sed -E "s/(\"version\"[[:space:]]*:[[:space:]]*)\"[^\"]*\"/\1\"$version\"/" "$backup" >"$config"
fi

mkdir -p "$bin"

# mygo 一次产出可执行文件 + .desktop + 图标 + deb / rpm / PKGBUILD / install.sh，
# 以及用 install.sh 装到 ~/.local 的 tar.gz，全部留在 build/bin/linux-<arch>/。
go tool mygo build -platform "linux/$arch" -skip-dmg -o "$bin"

dir="$bin/linux-$arch"
[[ -f $dir/pxeserver ]] || die "构建产物缺失：$dir/pxeserver"

# 打包期授权：给 PKGBUILD 追加 post_install，装完就带特权端口能力；
# deb/rpm 要 dpkg-deb 与 rpmbuild，本机没有时它会自己跳过（CI 上会注入）。
bash "$root/Scripts/patch-packages.sh" "$dir"

printf '构建完成：%s\n' "$dir"
while IFS= read -r file; do
    [[ $(basename "$file") == .mygo-files ]] && continue
    printf '  %-44s %8d KB\n' "$(basename "$file")" $(($(stat -c %s "$file") / 1024))
done < <(find "$dir" -maxdepth 1 -type f | sort)
