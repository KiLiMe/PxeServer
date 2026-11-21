//go:build !windows

package firewall

import "fmt"

// 非 Windows 平台没有「按程序放行」这回事：Linux/macOS 的防火墙按端口/服务，
// 入站默认就放行（firewalld/ufw 开着的发行版需要用户自己开 67/69/80）。
// 因此这里状态恒为 on、Supported 恒为 false，界面据此整块隐藏「防火墙放行」。

// Supported 让界面知道该不该画「放行程序 / 撤销放行」这一块。
func Supported() bool { return false }

func HandleCLI() bool { return false }
func Apply() error    { return fmt.Errorf("此平台不需要放行防火墙") }
func Remove() error   { return nil }
func State() string   { return "on" }
func Hint() string    { return "" }
func ExePath() string { return "?" }

// 没有防火墙规则可查，也就没有「程序移动过」这回事。
func MovedFrom(_, _ string) string { return "" }
