//go:build !linux

package priv

import "errors"

// CanBindPrivilegedPorts 在非 Linux 平台恒为真：Windows 绑定 67/69 不需要管理员权限，
// 其它平台的界面也不做这段提示，调用处因此不必写 runtime.GOOS 分支。
func CanBindPrivilegedPorts() bool { return true }

// Grant 在非 Linux 平台没有「特权端口」这回事：界面上不会出现授权按钮，
// 真被调用到也直接报错，免得静默成功。
func Grant(string) error { return errors.New("此平台不需要为端口授权") }
