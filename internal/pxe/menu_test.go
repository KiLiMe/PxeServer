package pxe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

func TestMenuURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			"HTTP 80 端口省略端口且去前导斜杠",
			config.Config{HTTPEnable: true, HTTPPort: 80, ServerIP: "192.168.5.1", MenuFile: "/menu.ipxe"},
			"http://192.168.5.1/menu.ipxe",
		},
		{
			"非 80 端口保留端口且优先 BootServerHost",
			config.Config{HTTPEnable: true, HTTPPort: 8080, ServerIP: "192.168.5.1", NextServer: "192.168.5.2", BootServerHost: "192.168.5.3", MenuFile: "boot.ipxe"},
			"http://192.168.5.3:8080/boot.ipxe",
		},
		{
			"关闭 HTTP 时只返回文件名",
			config.Config{HTTPEnable: false, HTTPPort: 80, ServerIP: "192.168.5.1", MenuFile: "/menu.ipxe"},
			"menu.ipxe",
		},
	}
	for _, tc := range cases {
		if got := MenuURL(&tc.cfg); got != tc.want {
			t.Errorf("%s：得到 %q，应为 %q", tc.name, got, tc.want)
		}
	}
}

func TestWriteHTTPHint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		enable bool
	}{
		{"HTTP 开启时生成脚本", true},
		{"HTTP 关闭时跳过生成", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{TFTPRoot: t.TempDir(), ServerIP: "192.0.2.1", HTTPEnable: tc.enable, HTTPPort: 8080, MenuFile: "custom.ipxe"}
			writeHTTPHint(cfg, func(level logbook.Level, format string, args ...any) {
				if level != logbook.Info {
					t.Errorf(format, args...)
				}
			})
			body, err := os.ReadFile(filepath.Join(cfg.TFTPRoot, "httpboot.ipxe"))
			if tc.enable {
				if err != nil || !strings.HasPrefix(string(body), "#!ipxe\n") || !strings.Contains(string(body), "\nchain http://192.0.2.1:8080/custom.ipxe\n") {
					t.Fatalf("引导脚本 = %q，读取错误 = %v", body, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("关闭 HTTP 时不应创建引导脚本，读取错误 = %v", err)
			}
		})
	}
}
