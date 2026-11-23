//go:build !linux

package dhcp

import (
	"net"
	"strings"
	"testing"

	"pxeserv/internal/config"
)

// bindService 只认 server_ip：成功时本地地址必须就是它，失败时不许退回 0.0.0.0。
func TestOtherBindServiceServerIP(t *testing.T) {
	t.Run("绑定到 server_ip", func(t *testing.T) {
		s := New(&config.Config{ServerIP: "127.0.0.1"}, nil)
		conn, err := s.bindService("0")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if got := conn.LocalAddr().(*net.UDPAddr).IP.String(); got != "127.0.0.1" {
			t.Fatalf("监听地址 = %s，期望 127.0.0.1", got)
		}
	})

	t.Run("地址不属于本机时报错且不退回 0.0.0.0", func(t *testing.T) {
		// 192.0.2.0/24 是 TEST-NET-1，正常不会被配置到本机网卡上。
		s := New(&config.Config{ServerIP: "192.0.2.1"}, nil)
		conn, err := s.bindService("0")
		if err == nil {
			conn.Close()
			t.Fatalf("绑定 192.0.2.1:0 应当失败，却成功监听了 %s", conn.LocalAddr())
		}
		if !strings.Contains(err.Error(), "192.0.2.1") {
			t.Fatalf("错误未指明配置的地址：%v", err)
		}
	})
}
