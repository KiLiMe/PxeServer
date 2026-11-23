package app

import (
	"os"

	"pxeserv/internal/priv"
)

// CanBindPrivilegedPorts 报告本进程能否绑定 1024 以下的端口。
// 67/69（以及默认的 HTTP 80）都是特权端口，界面据此提前提示，
// 免得用户点完「启动」才在日志里看到 bind: permission denied。
func (a *App) CanBindPrivilegedPorts() bool { return priv.CanBindPrivilegedPorts() }

// ExecutablePath 返回当前可执行文件的绝对路径，供界面拼 setcap 命令；
// 取不到时返回空串，调用方自行退回占位写法。
func (a *App) ExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}
