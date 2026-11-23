//go:build linux

package app

import (
	"os"
	"strings"
	"syscall"
)

// Restart 用同一份可执行文件替换当前进程镜像。setcap 改的是文件上的 xattr，
// 能力要下一次 exec 才进 CapEff，所以「授权成功后重启」是生效的必要一步；
// 让界面一键完成，用户就不必自己关窗口、再去菜单里重新打开。
//
// 成功时不返回（进程已被替换成新镜像），失败时返回原因。
func (a *App) Restart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	a.Logf("重启程序以让授权生效：%s", exe)
	// syscall.Exec 替换的是当前进程：同一 PID、同一环境变量，标准输出与日志照旧。
	args := withoutCapture(os.Args)
	// argv[0] 换成绝对路径：程序可能是用 ./pxeserver 起的，
	// 而 exec 之后 ps 里显示的仍是 argv[0]。
	args[0] = exe
	return syscall.Exec(exe, args, os.Environ())
}

// withoutCapture 去掉 -capture <文件>：那是截图自检的一次性参数，
// 原样复制给新进程会让它再截一张图然后退出，用户看到的就是「点重启，程序没了」。
// argv[0] 永远保留，否则 exec 会因为没有程序名而失败。
func withoutCapture(args []string) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, 0, len(args))
	out = append(out, args[0])
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-capture" || arg == "--capture":
			i++ // 连同后面的路径一起丢掉
		case strings.HasPrefix(arg, "-capture=") || strings.HasPrefix(arg, "--capture="):
			// 内联写法，本身就该丢掉
		default:
			out = append(out, arg)
		}
	}
	return out
}
