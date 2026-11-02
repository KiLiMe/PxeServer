package netif

import (
	"net"
	"testing"
)

// TestList 确认网卡枚举至少能返回本机的 IPv4 地址，且掩码/广播地址可解析。
func TestList(t *testing.T) {
	list := List()
	if len(list) == 0 {
		t.Skip("本机没有可用的 IPv4 网卡，跳过")
	}
	for _, it := range list {
		if it.IP == "" {
			t.Logf("%s 无 IPv4（up=%v mac=%s）", it.Name, it.Up, it.MAC)
			continue
		}
		if net.ParseIP(it.IP).To4() == nil {
			t.Fatalf("网卡 %s 的 IP 非法: %q", it.Name, it.IP)
		}
		if net.ParseIP(it.Mask).To4() == nil {
			t.Fatalf("网卡 %s 的掩码非法: %q", it.Name, it.Mask)
		}
		if net.ParseIP(it.Broadcast).To4() == nil {
			t.Fatalf("网卡 %s 的广播地址非法: %q", it.Name, it.Broadcast)
		}
		t.Logf("%s ip=%s mask=%s bcast=%s mac=%s", it.Name, it.IP, it.Mask, it.Broadcast, it.MAC)
	}
}

// TestBroadcast 校验广播地址计算：192.168.5.1/24 -> 192.168.5.255。
func TestBroadcast(t *testing.T) {
	ip := net.ParseIP("192.168.5.1").To4()
	mask := net.CIDRMask(24, 32)
	if got := Broadcast(ip, mask).String(); got != "192.168.5.255" {
		t.Fatalf("Broadcast = %s, 期望 192.168.5.255", got)
	}
	if got := Broadcast(ip, net.IPMask{255, 255, 255, 0}).String(); got != "192.168.5.255" {
		t.Fatalf("Broadcast(4字节掩码) = %s", got)
	}
	if got := Broadcast(nil, mask).String(); got != "0.0.0.0" {
		t.Fatalf("Broadcast(nil) = %s，期望 0.0.0.0", got)
	}
}
