package dhcp

import (
	"net"
	"testing"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

// newTestServer 起一个真实 socket 的服务端，客户端占住 127.0.0.1:67，
// 借助 giaddr 非 0 时单播到 giaddr:67 的规则把应答收回来看。
func newTestServer(t *testing.T, mut func(*config.Config)) (*Server, *net.UDPConn, *net.UDPConn) {
	t.Helper()
	cfg := config.Default()
	cfg.ProxyDHCP = false
	cfg.ServerIP = "192.168.100.1"
	cfg.NextServer = "192.168.100.1"
	cfg.PoolStart = "192.168.100.100"
	cfg.PoolSize = 4
	cfg.SubnetMask = "255.255.255.0"
	cfg.Router = "192.168.100.1"
	cfg.BootFile = "ipxe.efi"
	if mut != nil {
		mut(cfg)
	}
	s := New(cfg, func(logbook.Level, string, ...any) {})

	cli, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 67})
	if err != nil {
		t.Skipf("127.0.0.1:67 不可用: %v", err)
	}
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		cli.Close()
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { cli.Close(); srv.Close() })
	return s, srv, cli
}

func testRequest(xid uint32, mac []byte, msgType byte, opts func(*Packet)) *Packet {
	req := &Packet{Op: 1, HType: 1, HLen: 6, XID: xid, Flags: 0x8000}
	req.GIAddr = [4]byte{127, 0, 0, 1}
	copy(req.CHAddr[:], mac)
	req.SetByte(OptMessageType, msgType)
	req.SetString(OptVendorClass, "PXEClient:Arch:00007:UNDI:003016")
	req.SetOpt(OptClientArch, []byte{0, 7})
	if opts != nil {
		opts(req)
	}
	return req
}

func recv(t *testing.T, cli *net.UDPConn, wait time.Duration) (*Packet, error) {
	t.Helper()
	_ = cli.SetReadDeadline(time.Now().Add(wait))
	buf := make([]byte, 2048)
	n, _, err := cli.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	return Parse(buf[:n])
}

var (
	macA = []byte{0x00, 0x15, 0x5d, 0x01, 0x97, 0x03}
	macB = []byte{0x00, 0x15, 0x5d, 0x01, 0x97, 0x04}
)

// 客户端点名的地址已经被别人占用时，必须回 NAK，而不是默默把旧地址再给它。
func TestRequestConflictingAddressGetsNak(t *testing.T) {
	s, srv, cli := newTestServer(t, nil)

	// 先把 .100 租给 A
	s.reply(srv, testRequest(1, macA, Discover, nil), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, Offer)
	if got := s.Snapshots(); len(got) != 1 || got[0].IP != "192.168.100.100" {
		t.Fatalf("预置租约失败: %+v", got)
	}
	if _, err := recv(t, cli, 2*time.Second); err != nil {
		t.Fatalf("没收到 OFFER: %v", err)
	}

	// B 上来就要 .100
	req := testRequest(2, macB, Request, func(p *Packet) {
		p.SetIP(OptRequestedIP, net.ParseIP("192.168.100.100"))
	})
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	p, err := recv(t, cli, 2*time.Second)
	if err != nil {
		t.Fatalf("没收到应答: %v", err)
	}
	if got := p.MessageType(); got != Nak {
		t.Fatalf("应答类型 = %s，期望 NAK", TypeName(got))
	}
	if !net.IP(p.YIAddr[:]).Equal(net.IPv4zero) {
		t.Fatalf("NAK 不该带 yiaddr，实际 %s", net.IP(p.YIAddr[:]))
	}
	if got := string(p.Get(optMessage)); got == "" {
		t.Fatalf("NAK 应带 option 56 说明原因")
	}
}

// 池外的地址（换网段后客户端死磕旧地址）也回 NAK。
func TestRequestOutOfPoolGetsNak(t *testing.T) {
	s, srv, cli := newTestServer(t, nil)
	req := testRequest(3, macA, Request, func(p *Packet) {
		p.SetIP(OptRequestedIP, net.ParseIP("192.168.100.250"))
	})
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	p, err := recv(t, cli, 2*time.Second)
	if err != nil {
		t.Fatalf("没收到应答: %v", err)
	}
	if got := p.MessageType(); got != Nak {
		t.Fatalf("应答类型 = %s，期望 NAK", TypeName(got))
	}
}

// 客户端在 option 54 里点名了别的 DHCP 服务器时，本机不能抢答。
func TestRequestForOtherServerIsIgnored(t *testing.T) {
	s, srv, cli := newTestServer(t, nil)
	req := testRequest(4, macA, Request, func(p *Packet) {
		p.SetIP(OptRequestedIP, net.ParseIP("192.168.100.100"))
		p.SetIP(OptServerID, net.ParseIP("192.168.100.2"))
	})
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	if p, err := recv(t, cli, 400*time.Millisecond); err == nil {
		t.Fatalf("不该有应答，却收到 %s", TypeName(p.MessageType()))
	}
	if got := s.Snapshots(); len(got) != 0 {
		t.Fatalf("不该留下租约，实际 %+v", got)
	}
}

// 我们自己被选为服务器时的正常 REQUEST 仍然要 ACK。
func TestRequestForUsGetsAck(t *testing.T) {
	s, srv, cli := newTestServer(t, nil)
	s.reply(srv, testRequest(5, macA, Discover, nil), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, Offer)
	if _, err := recv(t, cli, 2*time.Second); err != nil {
		t.Fatalf("没收到 OFFER: %v", err)
	}

	req := testRequest(5, macA, Request, func(p *Packet) {
		p.SetIP(OptRequestedIP, net.ParseIP("192.168.100.100"))
		p.SetIP(OptServerID, net.ParseIP("192.168.100.1"))
	})
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	p, err := recv(t, cli, 2*time.Second)
	if err != nil {
		t.Fatalf("没收到应答: %v", err)
	}
	if got := p.MessageType(); got != Ack {
		t.Fatalf("应答类型 = %s，期望 ACK", TypeName(got))
	}
}

// DECLINE 报上来的冲突地址要隔离一段时间，不能立刻分给下一台机器。
func TestDeclinedAddressIsQuarantined(t *testing.T) {
	s, _, _ := newTestServer(t, func(c *config.Config) { c.PoolSize = 8 })

	s.dropAddress(net.ParseIP("192.168.100.100"))
	if !s.requestUnusable("00:00:00:00:00:99", net.ParseIP("192.168.100.100")) {
		t.Fatalf("被举报冲突的地址应判定为不可用")
	}
	ip, ok := s.alloc("00:00:00:00:00:99", nil)
	if !ok {
		t.Fatalf("应该还能分到别的地址")
	}
	if ip.String() == "192.168.100.100" {
		t.Fatalf("隔离期内的地址被分配出去了")
	}
	if ip.String() != "192.168.100.101" {
		t.Logf("分到 %s（可接受，只要不是 .100）", ip)
	}
}

// REQUEST 指定了被隔离的地址，也要 NAK。
func TestRequestQuarantinedAddressGetsNak(t *testing.T) {
	s, srv, cli := newTestServer(t, nil)
	s.dropAddress(net.ParseIP("192.168.100.100"))

	req := testRequest(6, macA, Request, func(p *Packet) {
		p.SetIP(OptRequestedIP, net.ParseIP("192.168.100.100"))
	})
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	p, err := recv(t, cli, 2*time.Second)
	if err != nil {
		t.Fatalf("没收到应答: %v", err)
	}
	if got := p.MessageType(); got != Nak {
		t.Fatalf("应答类型 = %s，期望 NAK", TypeName(got))
	}
}
