#!/usr/bin/env bash
# 打包期授权：给包管理器装好的二进制加上 CAP_NET_BIND_SERVICE，普通用户开箱就能绑
# 67/69，不必再手工 setcap。只做「这台机器上做得到」的那几种包：
#   - PKGBUILD：往 makepkg 的 PKGBUILD 里追加 post_install / post_upgrade，零依赖；
#   - deb：需要 dpkg-deb，给 control 加 DEBIAN/postinst；
#   - rpm：需要 rpm2cpio + cpio + rpmbuild，按原包的元数据重建并追加 %post。
# 缺工具就跳过并说明——界面里的「一键授权」（pkexec）仍能兜底。
# 用法：Scripts/patch-packages.sh <产物目录>（即 build/linux-<arch>）
set -euo pipefail

app=pxeserver
exe=/opt/$app/$app
cap='cap_net_bind_service=+ep'

dir=${1:-build/linux-amd64}
[[ -d $dir ]] || { printf '产物目录不存在：%s\n' "$dir" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

# scriptlet 是 deb 与 rpm 共用的授权片段：装/升级完各跑一次。
# setcap 缺失或文件系统不支持 xattr 都不许让安装失败，因此一律 || true。
scriptlet() {
    cat <<EOF
if command -v setcap >/dev/null 2>&1; then
	setcap '$cap' $exe || true
fi
EOF
}

patch_pkgbuild() {
    local file=$dir/PKGBUILD
    [[ -f $file ]] || return 0
    if grep -q '^install=' "$file"; then
        printf '  PKGBUILD  已含 install=，跳过\n'
        return 0
    fi
    # makepkg 不会把 PKGBUILD 里的函数搬进 .INSTALL：只有在 PKGBUILD 里写了
    # install=<文件名>，它才会把那个文件（相对 PKGBUILD 所在目录）随包装上。
    local inst=$app.install
    {
        printf '# 装完给 %s 加特权端口能力：普通用户就能绑 67/69。\n' "$exe"
        printf '# || true：文件系统不支持 xattr、或没装 libcap 时不能让安装失败，界面的「一键授权」还能兜底。\n'
        printf 'post_install() {\n'
        scriptlet | sed 's/^/\t/'
        printf '}\n'
        printf 'post_upgrade() {\n\tpost_install\n}\n'
    } >"$dir/$inst"
    printf '\n# 让 makepkg 把 %s 随包带上（见上方文件）。\ninstall=%s\n' "$inst" "$inst" >>"$file"
    printf '  PKGBUILD  已追加 install=%s（post_install / post_upgrade）\n' "$inst"
}

patch_deb() {
    local pkg=$1 work
    work=$(mktemp -d)
    dpkg-deb -R "$pkg" "$work"
    {
        printf '#!/bin/sh\n# 由 Scripts/patch-packages.sh 注入：装/升级后授权绑定特权端口。\nset -e\n'
        scriptlet
        printf 'exit 0\n'
    } >"$work/DEBIAN/postinst"
    chmod 755 "$work/DEBIAN/postinst"
    dpkg-deb -b --root-owner-group "$work" "$pkg" >/dev/null
    # 重建后自己验一遍：包里的 postinst 真带上 setcap 才算注入成功，
    # 否则让 patch_one 还原原包，别发一个假装授权的包出去。
    if ! dpkg-deb -e "$pkg" "$work/check" || ! grep -q setcap "$work/check/postinst"; then
        printf 'deb 重建后找不到带 setcap 的 postinst\n' >&2
        rm -rf "$work"
        return 1
    fi
    rm -rf "$work"
    printf '  %-9s 已注入 DEBIAN/postinst\n' "$(basename "$pkg")"
}

patch_rpm() {
    local pkg=$1 work ver rel lic sum arch files
    work=$(mktemp -d)
    mkdir -p "$work/payload" "$work/rpmbuild"
    (cd "$work/payload" && rpm2cpio "$pkg" | cpio -idm --quiet)

    ver=$(rpm -qp --qf '%{VERSION}' "$pkg")
    rel=$(rpm -qp --qf '%{RELEASE}' "$pkg")
    lic=$(rpm -qp --qf '%{LICENSE}' "$pkg")
    arch=$(rpm -qp --qf '%{ARCH}' "$pkg")
    # Summary 可能是多行的，压成一行；%files 用原包的包内路径，避免和实际安装布局不一致。
    sum=$(rpm -qp --qf '%{SUMMARY}' "$pkg" | tr '\n' ' ' | sed 's/ *$//')
    files=$(rpm -qpl "$pkg")

    {
        printf 'Name: %s\n' "$app"
        printf 'Version: %s\nRelease: %s\n' "$ver" "$rel"
        printf 'Summary: %s\nLicense: %s\n' "$sum" "$lic"
        printf 'BuildArch: %s\nAutoReqProv: no\n' "$arch"
        # 原包的依赖原样带上（含 soname），装的时候不会比正式包松。
        rpm -qpR "$pkg" | grep -v '^rpmlib(' | sed 's/^/Requires: /' || true
        printf '%%description\n%s\n' "$sum"
        printf '%%prep\n%%build\n'
        printf '%%install\nmkdir -p "%%{buildroot}"\ncp -a %s/payload/. "%%{buildroot}/"\n' "$work"
        printf '%%files\n%s\n' "$files"
        printf '%%post\n'
        scriptlet
    } >"$work/$app.spec"

    rpmbuild -bb --quiet \
        --define "_topdir $work/rpmbuild" \
        --define "_sourcedir $work" \
        "$work/$app.spec" >/dev/null
    local out
    out=$(find "$work/rpmbuild/RPMS" -name '*.rpm' -print -quit)
    [[ -n $out ]] || { printf 'rpmbuild 没有产出包\n' >&2; rm -rf "$work"; return 1; }
    if ! rpm -qp --scripts "$out" | grep -q setcap; then
        printf '重建出的 rpm 里没有 %%post 或它不带 setcap\n' >&2
        rm -rf "$work"
        return 1
    fi
    cp -f "$out" "$pkg"
    rm -rf "$work"
    printf '  %-9s 已注入 %%post\n' "$(basename "$pkg")"
}

# patch_one 先用副本换掉原包，注入失败就还原：发版不能因为这一步把一个本来好好的包弄坏。
patch_one() {
    local fn=$1 pkg=$2 bak=$2.bak
    cp -f "$pkg" "$bak"
    if ("$fn" "$pkg"); then
        rm -f "$bak"
    else
        mv -f "$bak" "$pkg"
        printf '  %-9s 注入失败，已还原原包（界面「一键授权」仍可兑底）\n' "$(basename "$pkg")"
    fi
}

printf '打包期授权：%s\n' "$dir"
patch_pkgbuild

shopt -s nullglob
debs=("$dir"/*.deb)
rpms=("$dir"/*.rpm)
shopt -u nullglob

if ((${#debs[@]})); then
    if have dpkg-deb; then
        for pkg in "${debs[@]}"; do patch_one patch_deb "$pkg"; done
    else
        printf '  *deb      缺 dpkg-deb，跳过（装 dpkg 后才能注入）\n'
    fi
fi

if ((${#rpms[@]})); then
    if have rpm2cpio && have rpmbuild && have cpio; then
        for pkg in "${rpms[@]}"; do patch_one patch_rpm "$pkg"; done
    else
        printf '  *rpm      缺 rpm2cpio / rpmbuild / cpio，跳过（CI 上装 rpm 才能注入）\n'
    fi
fi
