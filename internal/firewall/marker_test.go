//go:build windows

package firewall

import (
	"path/filepath"
	"testing"

	"pxeserv/internal/config"
)

// TestMovedFrom 校验「程序换过位置」的判定：只认有标记且路径不一致的情况。
func TestMovedFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := `D:\Old\PxeServer.exe`
	if err := config.SetRuntime(path, func(r *config.Runtime) { r.FirewallExe = old }); err != nil {
		t.Fatalf("写运行态失败：%v", err)
	}
	if got := MovedFrom(path, `D:\New\PxeServer.exe`); got != old {
		t.Fatalf("移动后应返回旧路径，得到 %q", got)
	}
	if got := MovedFrom(path, old); got != "" {
		t.Fatalf("路径未变时不该报告移动，得到 %q", got)
	}
	missing := filepath.Join(t.TempDir(), "missing.json")
	if got := MovedFrom(missing, old); got != "" {
		t.Fatalf("没有标记时不该报告移动，得到 %q", got)
	}
}
