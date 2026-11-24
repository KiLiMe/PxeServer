package config

import (
	"net"
	"testing"
)

// TestBootHost 逐级验证「回给客户端的服务器地址」：专用覆盖值优先，
// 其次是 NextServer（Normalize 已把它落成实际值），最后才是 ServerIP。
func TestBootHost(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"专用覆盖值优先", Config{BootServerHost: "10.0.0.9", NextServer: "10.0.0.2", ServerIP: "10.0.0.1"}, "10.0.0.9"},
		{"其次 NextServer", Config{NextServer: "10.0.0.2", ServerIP: "10.0.0.1"}, "10.0.0.2"},
		{"最后 ServerIP", Config{ServerIP: "10.0.0.1"}, "10.0.0.1"},
		{"全空给空串", Config{}, ""},
	}
	for _, c := range cases {
		if got := c.cfg.BootHost(); got != c.want {
			t.Errorf("%s：BootHost() = %q，期望 %q", c.name, got, c.want)
		}
	}
}

// TestIsReserved 验证地址池不会把服务器自己与网关分出去。
func TestIsReserved(t *testing.T) {
	cfg := &Config{ServerIP: "192.168.5.1", Router: "192.168.5.254"}
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.5.1", true},   // 服务器自己
		{"192.168.5.254", true}, // 网关
		{"192.168.5.2", false},
		{"192.168.5.255", false}, // 广播地址不在本判定里，由调用方另判
	}
	for _, c := range cases {
		if got := cfg.IsReserved(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("IsReserved(%s) = %v，期望 %v", c.ip, got, c.want)
		}
	}

	// 没配网关时只挡服务器自己，空串不能被当成 0.0.0.0 误判。
	only := &Config{ServerIP: "192.168.5.1"}
	if only.IsReserved(net.ParseIP("0.0.0.0")) {
		t.Error("空网关不应把 0.0.0.0 判成保留地址")
	}
	if !only.IsReserved(net.ParseIP("192.168.5.1")) {
		t.Error("服务器自己的地址必须判成保留地址")
	}
}
