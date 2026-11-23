//go:build linux

package priv

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// bindCap 是授权动作的全部内容：只给这一个能力，绝不 setuid root——
// setuid 会把整个程序变成 root 身份，而绑定低端口只需要这一位。
const bindCap = "cap_net_bind_service=+ep"

// Grant 通过 polkit 的 pkexec 给 exe 加上 CAP_NET_BIND_SERVICE（桌面会弹认证框）。
// 权限只落在文件上：当前进程的 CapEff 不会因此改变，调用方必须提示用户重启程序。
func Grant(exe string) error {
	if strings.TrimSpace(exe) == "" {
		return errors.New("不知道要给哪个文件授权：可执行文件路径为空")
	}
	// pkexec 会重置环境变量（PATH 固定为 /usr/{,s}bin:/sbin），setcap 一律用绝对路径，
	// 否则 Debian 系把 setcap 放在 /usr/sbin 时会在提权进程里找不到命令。
	setcap, err := exec.LookPath("setcap")
	if err != nil {
		return errors.New("系统里没有 setcap（缺 libcap），无法授权；装好 libcap 后重试，或在终端手工执行 setcap 命令")
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return errors.New("系统里没有 pkexec（缺 polkit），请在终端手工执行 setcap 命令")
	}
	out, err := exec.Command(pkexec, setcap, bindCap, exe).CombinedOutput()
	if err == nil {
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
