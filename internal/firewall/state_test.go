//go:build windows

package firewall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateFromMarker(t *testing.T) {
	cases := []struct {
		name   string
		marked bool
		want   string
	}{
		{"上次放行过", true, "on"},
		{"没有放行记录", false, "off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stateFromMarker(tc.marked); got != tc.want {
				t.Fatalf("stateFromMarker(%v) = %q，期望 %q", tc.marked, got, tc.want)
			}
		})
	}
}

// 配置文件本身不合法（地址池超限）时，放行标记仍要能读到，
// 否则界面会误报「未放行」。
func TestMarkerMatchesExeIgnoresInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"pool_size": 300, "runtime": {"firewall_exe": "C:\\Tools\\PxeServer.exe"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		exe  string
		want bool
	}{
		{"路径一致", `C:\Tools\PxeServer.exe`, true},
		{"路径不同", `D:\Other\PxeServer.exe`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := markerMatchesExe(path, tc.exe); got != tc.want {
				t.Fatalf("markerMatchesExe(%q) = %v，期望 %v", tc.exe, got, tc.want)
			}
		})
	}
}
