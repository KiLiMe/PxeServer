package dhcp

import (
	"net"
	"os"
	"testing"
	"time"

	"pxeserv/internal/config"
)

func TestLinuxDHCPBroadcast(t *testing.T) {
	cfg := &config.Config{ServerIP: "127.0.0.1"}
	s := New(cfg, nil)
	if err := s.setupNetwork(); err != nil {
		t.Fatal(err)
	}
	listener, err := s.bindService("0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := bindUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	for _, tc := range []struct {
		name   string
		accept bool
	}{
		{"接收所选接口的广播", true},
		{"丢弃其他接口的报文", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			network := s.network
			if !tc.accept {
				network.ifIndex++
			}
			to := &net.UDPAddr{IP: net.IPv4(127, 255, 255, 255), Port: listener.LocalAddr().(*net.UDPAddr).Port}
			if _, err := client.WriteToUDP([]byte("broadcast"), to); err != nil {
				t.Fatal(err)
			}
			if err := listener.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			var buf [64]byte
			n, from, err := network.read(listener, buf[:])
			if !tc.accept {
				if !os.IsTimeout(err) {
					t.Fatalf("其他接口的报文未被过滤：%v", err)
				}
				return
			}
			if err != nil || string(buf[:n]) != "broadcast" {
				t.Fatalf("接收广播 = %q，错误 = %v", buf[:n], err)
			}
			if err := network.write(listener, []byte("reply"), from); err != nil {
				t.Fatal(err)
			}
			if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			n, from, err = client.ReadFromUDP(buf[:])
			if err != nil || string(buf[:n]) != "reply" || from.IP.String() != cfg.ServerIP {
				t.Fatalf("应答 = %q，来源 = %v，错误 = %v", buf[:n], from, err)
			}
		})
	}
}
