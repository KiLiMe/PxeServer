//go:build linux

package priv

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// bindCap 是授权动作的全部内容：只给这一个能力，绝不 setuid root——
// setuid 会把整个程序变成 root 身份，而绑定低端口只需要这一位。
const bindCap = "cap_net_bind_service=+ep"

// BindCapGranted 报告 exe 这份文件上是否已经带了 CAP_NET_BIND_SERVICE（看 getcap 的输出）。
// 没装 libcap（没有 getcap）时返回 false——那是「查不到」，不是「没有」，调用方别据此拒绝授权。
func BindCapGranted(exe string) bool {
	getcap, err := exec.LookPath("getcap")
	if err != nil {
		return false
	}
	out, err := exec.Command(getcap, exe).Output()
	if err != nil {
		return false
	}
	return parseGetcap(string(out))
}

// parseGetcap 认 getcap 的输出，只看 effective 位（+ep 里的 e 才让非特权进程真正用上）：
//
//	/opt/pxeserver/pxeserver cap_net_bind_service=ep
//	/opt/pxeserver/pxeserver cap_net_admin,cap_net_bind_service=eip
//
// 空输出（文件没有能力）与报错信息（路径不存在等）都算没有。
func parseGetcap(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, " cap_")
		if idx < 0 {
			continue
		}
		names, flags, ok := strings.Cut(line[idx+1:], "=")
		if !ok {
			continue
		}
		if !slices.Contains(strings.Split(names, ","), "cap_net_bind_service") {
			continue
		}
		if strings.Contains(flags, "e") {
			return true
		}
	}
	return false
}

// Grant 通过 polkit 的 pkexec 给 exe 加上 CAP_NET_BIND_SERVICE（桌面会弹认证框）。
// 权限只落在文件上：当前进程的 CapEff 不会因此改变，调用方得让用户重启程序。
func Grant(exe string) error {
	if strings.TrimSpace(exe) == "" {
		return errors.New("不知道要给哪个文件授权：可执行文件路径为空")
	}
	// 文件已经带能力就别再弹一次认证框：重编、升级、拷来拷去之后重复点是常事。
	if BindCapGranted(exe) {
		return nil
	}
	// pkexec 会重置环境变量（PATH 固定为 /usr/{,s}bin:/sbin），setcap 一律用绝对路径，
	// 否则 Debian 系把 setcap 放在 /usr/sbin 时会在提权进程里找不到命令。
	setcap, err := exec.LookPath("setcap")
	if err != nil {
		return errors.New("系统里没有 setcap（缺 libcap），无法授权：Debian 系装 libcap2-bin、Arch 装 libcap，或在终端手工执行 setcap")
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return errors.New("系统里没有 pkexec（缺 polkit），请在终端手工执行 setcap")
	}
	out, err := exec.Command(pkexec, setcap, bindCap, exe).CombinedOutput()
	if err == nil {
		// setcap 在容器、overlayfs、部分网络文件系统上会「成功」但 xattr 没写进去，
		// 回头启动照样 bind 失败。查一下 getcap，把失败挡在这里。
		if _, lookErr := exec.LookPath("getcap"); lookErr == nil && !BindCapGranted(exe) {
			return errors.New("setcap 报告成功，但文件上没有该能力：所在文件系统可能不支持扩展属性，换个安装位置（如 /opt）再试")
		}
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// pkexec 的约定：126 用户取消认证，127 认证失败/策略不允许。
		switch exitErr.ExitCode() {
		case 126:
			return errors.New("授权被取消（没有通过认证）")
		case 127:
			return errors.New("认证失败，或 polkit 策略不允许这次提权")
		}
		return fmt.Errorf("setcap 失败（退出码 %d）：%s", exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("启动 pkexec 失败：%w", err)
}
