package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readRaw 直接反序列化，不走 Load 的 Normalize（测试里不想被自动探测干扰）。
func readRaw(t *testing.T, path string) *Config {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	c := &Config{}
	if err := json.Unmarshal(b, c); err != nil {
		t.Fatalf("解析: %v", err)
	}
	return c
}

// TestSetRuntimeKeepsUserFields：写运行态不能碰用户配置。
func TestSetRuntimeKeepsUserFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Default()
	c.ServerIP = "10.0.0.1"
	c.PoolStart = "10.0.0.2"
	c.TFTPRoot = `C:\boot`
	if _, err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := SetRuntime(path, func(r *Runtime) { r.DebugAddr = "127.0.0.1:54321" }); err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}

	got := readRaw(t, path)
	if got.ServerIP != "10.0.0.1" || got.PoolStart != "10.0.0.2" || got.TFTPRoot != `C:\boot` {
		t.Fatalf("用户字段被改动: %+v", got)
	}
	if got.Runtime.DebugAddr != "127.0.0.1:54321" {
		t.Fatalf("DebugAddr = %q", got.Runtime.DebugAddr)
	}
}

// TestSaveKeepsRuntimeOnDisk：界面保存配置时要沿用磁盘上的运行态。
func TestSaveKeepsRuntimeOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Default()
	if _, err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := SetRuntime(path, func(r *Runtime) {
		r.DebugAddr = "127.0.0.1:1234"
		r.FirewallExe = `C:\PxeServer\PxeServer.exe`
	}); err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}

	// 模拟界面改配置：进程内存里是一份没有 runtime 的配置。
	fresh := Default()
	fresh.ServerIP = "192.168.9.1"
	if _, err := fresh.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := readRaw(t, path)
	if got.ServerIP != "192.168.9.1" {
		t.Fatalf("ServerIP = %q", got.ServerIP)
	}
	if got.Runtime.DebugAddr != "127.0.0.1:1234" || got.Runtime.FirewallExe == "" {
		t.Fatalf("runtime 被冲掉了: %+v", got.Runtime)
	}
}

// TestSetRuntimeCreatesFile：配置文件不存在时按默认值新建。
func TestSetRuntimeCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SetRuntime(path, func(r *Runtime) { r.FirewallExe = "x.exe" }); err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("没有建文件: %v", err)
	}
	got := readRaw(t, path)
	if got.Runtime.FirewallExe != "x.exe" {
		t.Fatalf("FirewallExe = %q", got.Runtime.FirewallExe)
	}
}
