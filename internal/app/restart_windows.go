//go:build windows

package app

import "errors"

// Restart 在 Windows 上没有用处：那边绑定 67/69 不需要授权，
// 界面只在 Linux 的 setcap 流程里用到重启，走到这里是程序逻辑出了问题。
func (a *App) Restart() error {
	return errors.New("Windows 上不需要重启程序来获得端口权限")
}
