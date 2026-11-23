//go:build linux

package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// pickerTimeout 是外部选择器的存活上限：用户不点它就一直挂着，超时按「取消」收场，
// 不让协程永久滞留在 exec 上。比任何一次真实选择都宽裕。
const pickerTimeout = 10 * time.Minute

// desktopPickers 把桌面映射到它原生的选择器程序。
// Qt 桌面（KDE、LXQt）用 kdialog，画出来就是 Qt 原生框；GTK 桌面（GNOME、XFCE、
// Cinnamon…）用 zenity，是同一套 GTK widget。都不认识就不用外部程序，走内建对话框。
// 注意**不能**用桌面门户（GTK_USE_PORTAL）：带 cap_net_bind_service 的进程会被
// xdg-desktop-portal 拒绝服务，弹不出框还会 SIGABRT（见 troubleshooting 第 8 节）。
var desktopPickers = []struct {
	bin    string
	tokens []string
}{
	{"kdialog", []string{"KDE", "LXQT", "DDE", "DEEPIN", "UKUI"}},
	{"zenity", []string{"GNOME", "UNITY", "XFCE", "CINNAMON", "MATE", "BUDGIE", "PANTHEON", "LXDE", "ELEMENTARY"}},
}

// desktopPicker 返回这块桌面该用的外部选择器；不认识就返回空串。
// desktop 一般是 XDG_CURRENT_DESKTOP，形如 "KDE" 或 "ubuntu:GNOME"。
func desktopPicker(desktop string) string {
	for _, token := range splitDesktop(desktop) {
		for _, entry := range desktopPickers {
			for _, name := range entry.tokens {
				if token == name {
					return entry.bin
				}
			}
		}
	}
	return ""
}

// splitDesktop 把 "ubuntu:GNOME" 拆成 ["UBUNTU" "GNOME"]，统一大写后比对。
// Cinnamon 一类桌面报的是 "X-Cinnamon"，去掉 "X-" 前缀才认得出来。
func splitDesktop(desktop string) []string {
	tokens := strings.Split(desktop, ":")
	out := tokens[:0]
	for _, token := range tokens {
		token = strings.ToUpper(strings.TrimSpace(token))
		token = strings.TrimPrefix(token, "X-")
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

// pickerArgs 拼出选择器命令行：kdialog 的目录参数是位置参数，zenity 是 --option=值。
func pickerArgs(bin, title, start string) []string {
	if bin == "kdialog" {
		args := []string{"--title", title}
		if start != "" {
			args = append(args, "--getexistingdirectory", start)
		} else {
			args = append(args, "--getexistingdirectory")
		}
		return args
	}
	args := []string{"--file-selection", "--directory", "--title=" + title}
	if start != "" {
		args = append(args, "--filename="+start)
	}
	return args
}

// pickFolderExternal 用桌面对应的外部选择器选目录。
// handled=false 表示这台机器上没有能用的外部选择器（桌面不认识、或程序没装），
// 调用方应回落到内建的 GTK 对话框。handled=true 时 path 为空即「用户取消」。
func pickFolderExternal(title, start string) (path string, handled bool) {
	bin := desktopPicker(os.Getenv("XDG_CURRENT_DESKTOP"))
	if bin == "" && os.Getenv("KDE_FULL_SESSION") != "" {
		// KDE 的兜底标记：某些会话里 XDG_CURRENT_DESKTOP 是空的。
		bin = "kdialog"
	}
	if bin == "" {
		return "", false
	}
	full, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), pickerTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, full, pickerArgs(bin, title, start)...).Output()
	if err != nil {
		// 非零退出＝用户按了取消，超时＝同样当取消：都别让内建对话框再弹一次。
		return "", true
	}
	if picked := strings.TrimSpace(string(out)); picked != "" {
		return picked, true
	}
	return "", true
}
