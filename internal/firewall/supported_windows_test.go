//go:build windows

package firewall

import "testing"

// TestSupportedTrue Windows 按程序路径放行，界面要画出「设置 → 放行程序」这一块。
func TestSupportedTrue(t *testing.T) {
	if !Supported() {
		t.Fatal("Windows 上 Supported 应为 true")
	}
}
