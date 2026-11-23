//go:build !linux

package priv

// CanBindPrivilegedPorts 在非 Linux 平台恒为真：Windows 绑定 67/69 不需要管理员权限，
// 其它平台的界面也不做这段提示，调用处因此不必写 runtime.GOOS 分支。
func CanBindPrivilegedPorts() bool { return true }
