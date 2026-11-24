package netaddr

import (
	"net"
	"testing"
)

func TestToU32FromU32(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		u32  uint32
	}{
		{"零点", "0.0.0.0", 0},
		{"常规", "192.168.5.1", 0xC0A80501},
		{"末位全 1", "10.0.0.255", 0x0A0000FF},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToU32(net.ParseIP(tc.ip)); got != tc.u32 {
				t.Errorf("ToU32(%s) = %#x，期望 %#x", tc.ip, got, tc.u32)
			}
			if got := FromU32(tc.u32); !got.Equal(net.ParseIP(tc.ip).To4()) {
				t.Errorf("FromU32(%#x) = %v，期望 %s", tc.u32, got, tc.ip)
			}
		})
	}

	// 非 IPv4 输入按 0 处理，调用方无需自己判 To4。
	if got := ToU32(net.ParseIP("2001:db8::1")); got != 0 {
		t.Errorf("IPv6 应当返回 0，得到 %#x", got)
	}
	if got := ToU32(nil); got != 0 {
		t.Errorf("nil 应当返回 0，得到 %#x", got)
	}
}

func TestInRange(t *testing.T) {
	start := net.ParseIP("192.168.5.10")
	tests := []struct {
		name  string
		count int
		ip    string
		want  bool
	}{
		{"起点", 10, "192.168.5.10", true},
		{"区间内", 10, "192.168.5.15", true},
		{"终点", 10, "192.168.5.19", true},
		{"终点之后", 10, "192.168.5.20", false},
		{"起点之前", 10, "192.168.5.9", false},
		{"上界溢出到下一网段", 10, "192.168.6.1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := InRange(start, tc.count, net.ParseIP(tc.ip)); got != tc.want {
				t.Errorf("InRange(%s, %d, %s) = %v，期望 %v", start, tc.count, tc.ip, got, tc.want)
			}
		})
	}

	if InRange(start, 0, net.ParseIP("192.168.5.10")) {
		t.Error("数量为 0 时不应命中")
	}
	if InRange(nil, 10, net.ParseIP("192.168.5.10")) {
		t.Error("起点为 nil 时不应命中")
	}
	if InRange(start, 10, nil) {
		t.Error("待判地址为 nil 时不应命中")
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		n    int
		want string
	}{
		{"网段起点加二", "192.168.100.0", 2, "192.168.100.2"},
		{"撞上服务器再挪一位", "192.168.100.1", 3, "192.168.100.4"},
		{"末位进位", "192.168.100.255", 1, "192.168.101.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Add(net.ParseIP(tc.ip), tc.n).String(); got != tc.want {
				t.Errorf("Add(%s, %d) = %s，期望 %s", tc.ip, tc.n, got, tc.want)
			}
		})
	}
}
