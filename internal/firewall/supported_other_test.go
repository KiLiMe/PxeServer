//go:build !windows

package firewall

import "testing"

// TestSupportedFalse 非 Windows 没有「按程序放行」：Supported 为假，界面据此整块隐藏；
// 状态仍给 on（入站默认放行），动作接口保持不崩。
func TestSupportedFalse(t *testing.T) {
	if Supported() {
		t.Fatal("非 Windows 平台 Supported 应为 false")
	}
	if got := State(); got != "on" {
		t.Errorf("State() = %q，期望 on", got)
	}
	if err := Apply(); err == nil {
		t.Error("Apply() 应返回“此平台不需要放行防火墙”")
	}
	if err := Remove(); err != nil {
		t.Errorf("Remove() 应为空操作，实际：%v", err)
	}
	if HandleCLI() {
		t.Error("HandleCLI() 应为 false")
	}
	if Hint() != "" {
		t.Error("Hint() 应为空")
	}
	if MovedFrom("/old", "/new") != "" {
		t.Error("MovedFrom() 应为空")
	}
}
