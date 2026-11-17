package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRuntimeKeepsValueWhenConfigInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"pool_size": 300, "runtime": {"firewall_exe": "C:\\Tools\\PxeServer.exe"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("配置不合法时 Load 应报错，用例前提不成立")
	}
	rt, err := ReadRuntime(path)
	if err != nil {
		t.Fatalf("ReadRuntime 失败：%v", err)
	}
	if rt.FirewallExe != `C:\Tools\PxeServer.exe` {
		t.Fatalf("FirewallExe = %q，期望原样读出", rt.FirewallExe)
	}
}
