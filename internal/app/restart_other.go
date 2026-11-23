//go:build !windows && !linux

package app

import "errors"

// Restart 只在 Linux 的 setcap 流程里用到（文件能力要下一次 exec 才生效），
// 其余平台没有这回事。
func (a *App) Restart() error {
	return errors.New("此平台不需要重启程序来获得端口权限")
}
