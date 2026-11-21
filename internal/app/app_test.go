package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pxeserv/internal/config"
	"pxeserv/internal/netif"
	"pxeserv/internal/pxe"
)

func TestReadySummary(t *testing.T) {
	ifaceOf := func(ip string) string {
		if ip == "192.168.5.1" {
			return "以太网 2"
		}
		return ""
	}
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"本机地址带网卡名", config.Config{ServerIP: "192.168.5.1", NextServer: "192.168.5.1", TFTPRoot: "./boot"},
			"192.168.5.1（以太网 2），启动目录 ./boot"},
		{"找不到网卡时不写括号", config.Config{ServerIP: "192.0.2.10", NextServer: "192.0.2.10", TFTPRoot: "./boot"},
			"192.0.2.10，启动目录 ./boot"},
		{"下一台服务器不同则写出", config.Config{ServerIP: "192.168.5.1", NextServer: "192.168.5.9", TFTPRoot: "./boot"},
			"192.168.5.1（以太网 2），下一台服务器 192.168.5.9，启动目录 ./boot"},
		{"下一台服务器留空则不写", config.Config{ServerIP: "192.168.5.1", TFTPRoot: "./tftp"},
			"192.168.5.1（以太网 2），启动目录 ./tftp"},
	}
	for _, c := range cases {
		if got := readySummary(&c.cfg, ifaceOf); got != c.want {
			t.Errorf("%s：得到 %q，应为 %q", c.name, got, c.want)
		}
	}
}

func TestOtherSubnetHint(t *testing.T) {
	cfg := &config.Config{ServerIP: "192.168.5.1", SubnetMask: "255.255.255.0"}
	ifaces := []netif.Iface{
		{Name: "LAN", IP: "192.168.5.2", Mask: "255.255.255.0"},
		{Name: "Wi-Fi", IP: "10.0.0.8", Mask: "255.255.255.0"},
	}
	got := otherSubnetHint(ifaces, cfg)
	if !strings.Contains(got, "Wi-Fi=10.0.0.8") || strings.Contains(got, "LAN=") {
		t.Fatalf("otherSubnetHint = %q", got)
	}
}

func TestLogsSinceAndStartReentry(t *testing.T) {
	a := NewInMemory(config.Default())
	a.Logf("第一条")
	lines, seq, ok := a.LogsSince(0)
	if !ok || len(lines) != 1 || seq == 0 {
		t.Fatalf("首次 LogsSince 异常: ok=%v len=%d seq=%d", ok, len(lines), seq)
	}
	if _, _, ok := a.LogsSince(seq); ok {
		t.Fatalf("无新日志时 LogsSince 应返回 ok=false")
	}
	a.services = &pxe.Services{}
	if msg := a.StartServices(); msg != "" {
		t.Fatalf("运行中重入 StartServices 返回意外错误: %q", msg)
	}
}

func TestDebugListenAddr(t *testing.T) {
	cases := []struct {
		name    string
		port    int
		want    string
		wantErr bool
	}{
		{"不传参数不开接口", 0, "", false},
		{"正常端口只监听回环地址", 18080, "127.0.0.1:18080", false},
		{"上边界 65535", 65535, "127.0.0.1:65535", false},
		{"负数拒绝", -1, "", true},
		{"超过 65535 拒绝", 70000, "", true},
	}
	for _, c := range cases {
		got, err := debugListenAddr(c.port)
		if (err != nil) != c.wantErr {
			t.Errorf("%s：err = %v，wantErr = %v", c.name, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%s：地址 = %q，应为 %q", c.name, got, c.want)
		}
	}
}

func TestDebugAPIMethodGuard(t *testing.T) {
	a := NewInMemory(config.Default())
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]string{"result": orOK(a.StartServices())})
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.StopServices()
		writeJSON(w, map[string]string{"result": "已停止"})
	})

	for _, path := range []string{"/start", "/stop"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s 状态码 = %d，应为 405", path, rec.Code)
		}
	}
}

func TestDebugAPIGuard(t *testing.T) {
	const addr = "127.0.0.1:54321"
	h := guardLocal(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	cases := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{"本机直连放行", addr, "", http.StatusOK},
		{"localhost 也算本机", "localhost:54321", "", http.StatusOK},
		{"端口不符拒绝", "127.0.0.1:54322", "", http.StatusForbidden},
		{"域名重绑定拒绝", "evil.example:54321", "", http.StatusForbidden},
		{"浏览器跨站带 Origin 拒绝", addr, "https://evil.example", http.StatusForbidden},
		{"Host 缺端口拒绝", "127.0.0.1", "", http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		req.Host = c.host
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s：状态码 = %d，应为 %d", c.name, rec.Code, c.want)
		}
	}
}
