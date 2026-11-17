//go:build !windows

package firewall

import "fmt"

// 非 Windows 平台不需要放行防火墙：状态恒为 on，动作返回错误或空操作。

func HandleCLI() bool { return false }
func Apply() error    { return fmt.Errorf("此平台不需要放行防火墙") }
func Remove() error   { return nil }
func State() string   { return "on" }
func Hint() string    { return "" }
func ExePath() string { return "?" }

// 没有防火墙规则可查，也就没有「程序移动过」这回事。
func MovedFrom(_, _ string) string { return "" }
