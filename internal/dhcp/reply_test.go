package dhcp

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

// 回归：零值 Packet 直接写选项曾经 panic（assignment to entry in nil map），
// 表现为收到第一个 DHCPDISCOVER 就崩溃。
func TestZeroPacketSetters(t *testing.T) {
	var p Packet
	p.SetIP(OptServerID, net.ParseIP("192.168.100.1"))
	p.SetByte(OptMessageType, 2)
	p.SetU32(OptLeaseTime, 3600)
	p.SetString(OptBootFile, "ipxe.efi")
	p.SetOpt(OptVendorClass, []byte("PXEClient"))
	if got := net.IP(p.Get(OptServerID)).String(); got != "192.168.100.1" {
		t.Fatalf("server id = %s", got)
	}
	if got := string(p.Get(OptBootFile)); got != "ipxe.efi" {
		t.Fatalf("boot file = %q", got)
	}
	if got := p.MessageType(); got != 2 {
		t.Fatalf("msg type = %d", got)
	}
	b := p.Marshal()
	if _, err := Parse(b); err != nil {
		t.Fatalf("parse marshalled packet: %v", err)
	}
}

// 回归：reply 曾经在构造应答时报 nil map panic，这里确保不会崩且真的分配了地址。
func TestReplyAllocatesLease(t *testing.T) {
	cfg := config.Default()
	cfg.ServerIP = "192.168.100.1"
	cfg.NextServer = "192.168.100.1"
	cfg.PoolStart = "192.168.100.100"
	cfg.PoolSize = 4
	cfg.SubnetMask = "255.255.255.0"
	cfg.Router = "192.168.100.1"
	cfg.Broadcast = "192.168.100.255"
	cfg.ProxyDHCP = false // 直连模式才会分配地址

	var logs []string
	s := New(cfg, func(_ logbook.Level, f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })

	srv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()

	req := &Packet{Op: 1, HType: 1, HLen: 6, XID: 0x11223344, Flags: 0x8000}
	copy(req.CHAddr[:], []byte{0x00, 0x0c, 0x29, 0xaa, 0xbb, 0xcc})
	req.SetByte(OptMessageType, 1)
	req.SetString(OptVendorClass, "PXEClient:Arch:00007:UNDI:003016")

	from := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}
	s.reply(srv, req, from, Offer)

	snap := s.Snapshots()
	if len(snap) != 1 {
		t.Fatalf("leases = %d, want 1 (logs: %v)", len(snap), logs)
	}
	if snap[0].IP != "192.168.100.100" {
		t.Fatalf("lease ip = %s", snap[0].IP)
	}
}

// 回归：Hyper-V 的 UEFI 客户端（以及多数老 PXE ROM）只读 BOOTP 固定字段 file/sname，
// 不读 option 67。这里用真实 socket（giaddr 非 0 时服务端单播到 giaddr:67）验证应答内容。
func TestReplyFillsBootFileFields(t *testing.T) {
	cfg := config.Default()
	cfg.ProxyDHCP = false
	cfg.ServerIP = "192.168.100.1"
	cfg.NextServer = "192.168.100.1"
	cfg.PoolStart = "192.168.100.100"
	cfg.PoolSize = 4
	cfg.SubnetMask = "255.255.255.0"
	cfg.Router = "192.168.100.1"
	cfg.Broadcast = "192.168.100.255"
	cfg.BootFile = "ipxe.efi"

	var logs []string
	s := New(cfg, func(_ logbook.Level, f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })

	cli, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 67})
	if err != nil {
		t.Skipf("127.0.0.1:67 不可用: %v", err)
	}
	defer cli.Close()
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()

	req := &Packet{Op: 1, HType: 1, HLen: 6, XID: 0x99887766, Flags: 0x8000}
	req.GIAddr = [4]byte{127, 0, 0, 1}
	copy(req.CHAddr[:], []byte{0x00, 0x15, 0x5d, 0x01, 0x97, 0x03})
	req.SetByte(OptMessageType, 1)
	req.SetString(OptVendorClass, "PXEClient:Arch:00007:UNDI:003016")
	req.SetOpt(OptClientArch, []byte{0, 7})

	s.reply(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, Offer)

	_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := cli.ReadFrom(buf)
	if err != nil {
		t.Fatalf("没收到应答: %v (logs %v)", err, logs)
	}
	p, err := Parse(buf[:n])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := strings.TrimRight(string(p.File[:]), "\x00"); got != "snponly.efi" {
		t.Fatalf("BOOTP file 字段 = %q, want snponly.efi", got)
	}
	if got := string(p.Get(OptBootFile)); got != "snponly.efi" {
		t.Fatalf("option 67 = %q", got)
	}
	if got := string(p.Get(OptTFTPServer)); got != "192.168.100.1" {
		t.Fatalf("option 66 = %q", got)
	}
	if got := net.IP(p.SIAddr[:]).String(); got != "192.168.100.1" {
		t.Fatalf("siaddr = %s", got)
	}
	if got := p.MessageType(); got != Offer {
		t.Fatalf("msgtype = %d", got)
	}
}

// 回归：4011 上的 PXE 启动服务应答必须单播回请求方（它不在 68 端口等），
// 以前一律广播到 255.255.255.255:68，客户端永远收不到，于是报 "A boot image was not found"。
func TestBootServiceReplyUnicast(t *testing.T) {
	cfg := config.Default()
	cfg.ProxyDHCP = false
	cfg.ServerIP = "192.168.100.1"
	cfg.NextServer = "192.168.100.1"
	s := New(cfg, func(logbook.Level, string, ...any) {})

	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()
	s.bootConn = srv // 冒充 4011 的监听套接字

	cli, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer cli.Close()
	from := cli.LocalAddr().(*net.UDPAddr)

	r := &Packet{Op: 2, HType: 1, HLen: 6, XID: 7}
	r.SetByte(OptMessageType, Ack)
	s.send(srv, r, from)

	_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1024)
	n, _, err := cli.ReadFrom(buf)
	if err != nil {
		t.Fatalf("4011 应答没有单播回请求方: %v", err)
	}
	p, err := Parse(buf[:n])
	if err != nil || p.XID != 7 {
		t.Fatalf("回包异常: %v", err)
	}
}

// 回归：iPXE/gPXE 起来后第二次发 DHCP 请求是为了拿“脚本”，必须给它菜单文件；
// 以前一律回 ipxe.efi，导致 iPXE 又把固件拉一遍、或没脚本可执行而停在命令行。
func TestReplyServesMenuToIPXE(t *testing.T) {
	newBase := func() (*config.Config, *Server, *net.UDPConn) {
		cfg := config.Default()
		cfg.ProxyDHCP = false
		cfg.ServerIP = "192.168.100.1"
		cfg.NextServer = "192.168.100.1"
		cfg.PoolStart = "192.168.100.100"
		cfg.PoolSize = 4
		cfg.SubnetMask = "255.255.255.0"
		cfg.BootFile = "ipxe.efi"
		cfg.MenuFile = "menu.ipxe"
		s := New(cfg, func(logbook.Level, string, ...any) {})
		return cfg, s, nil
	}

	ask := func(t *testing.T, ipxe bool) *Packet {
		t.Helper()
		_, s, _ := newBase()
		cli, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 67})
		if err != nil {
			t.Skipf("127.0.0.1:67 不可用: %v", err)
		}
		defer cli.Close()
		srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer srv.Close()

		req := &Packet{Op: 1, HType: 1, HLen: 6, XID: 0x42424242, Flags: 0x8000}
		req.GIAddr = [4]byte{127, 0, 0, 1}
		copy(req.CHAddr[:], []byte{0x00, 0x15, 0x5d, 0x01, 0x97, 0x03})
		req.SetByte(OptMessageType, 1)
		req.SetString(OptVendorClass, "PXEClient:Arch:00007:UNDI:003016")
		req.SetOpt(OptClientArch, []byte{0, 7})
		if ipxe {
			req.SetString(OptUserClass, "iPXE")
		}
		s.reply(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, Offer)

		_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 2048)
		n, _, err := cli.ReadFrom(buf)
		if err != nil {
			t.Fatalf("没收到应答: %v", err)
		}
		p, err := Parse(buf[:n])
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return p
	}

	// iPXE 拿到的是菜单的完整 URL（菜单里因此可以用相对路径，不必写端口）。
	// 默认端口 80 不写进 URL，所以这里是 http://192.168.100.1/menu.ipxe。
	if got := string(ask(t, true).Get(OptBootFile)); got != "http://192.168.100.1/menu.ipxe" {
		t.Fatalf("iPXE 客户端的 option 67 = %q, 期望 http://192.168.100.1/menu.ipxe", got)
	}
	if got := string(ask(t, false).Get(OptBootFile)); got != "snponly.efi" {
		t.Fatalf("传统 PXE 客户端的 option 67 = %q, 期望 snponly.efi", got)
	}
}

// 普通 DHCP 客户端（WinPE 自己的网络栈、笔记本的 DHCP 客户端等）既没有 option 60
// 也没有 option 93，不该被塞启动文件，否则日志会被误导、某些客户端也会困惑。
func TestReplySkipsBootFileForPlainClient(t *testing.T) {
	cfg := config.Default()
	cfg.ProxyDHCP = false
	cfg.ServerIP = "192.168.100.1"
	cfg.NextServer = "192.168.100.1"
	cfg.PoolStart = "192.168.100.100"
	cfg.PoolSize = 4
	cfg.BootFile = "ipxe.efi"
	s := New(cfg, func(logbook.Level, string, ...any) {})

	cli, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 67})
	if err != nil {
		t.Skipf("127.0.0.1:67 不可用: %v", err)
	}
	defer cli.Close()
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()

	req := &Packet{Op: 1, HType: 1, HLen: 6, XID: 0x51515151, Flags: 0x8000}
	req.GIAddr = [4]byte{127, 0, 0, 1}
	copy(req.CHAddr[:], []byte{0x00, 0x15, 0x5d, 0x01, 0x97, 0x03})
	req.SetByte(OptMessageType, 1)

	s.reply(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, Offer)

	_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := cli.ReadFrom(buf)
	if err != nil {
		t.Fatalf("没收到应答: %v", err)
	}
	p, err := Parse(buf[:n])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := string(p.Get(OptBootFile)); got != "" {
		t.Fatalf("普通客户端的 option 67 = %q，应该不下发", got)
	}
	if got := strings.TrimRight(string(p.File[:]), "\x00"); got != "" {
		t.Fatalf("普通客户端的 BOOTP file = %q，应该为空", got)
	}
	if got := net.IP(p.YIAddr[:]).String(); got != "192.168.100.100" {
		t.Fatalf("普通客户端仍应分到地址，yiaddr = %s", got)
	}
}

func TestAllocRespectsSubnetMask(t *testing.T) {
	cfg := config.Default()
	cfg.ServerIP = "192.168.5.1"
	cfg.SubnetMask = "255.255.255.128" // /25，广播地址为 .127
	cfg.PoolStart = "192.168.5.125"
	cfg.PoolSize = 10 // 故意给大，验证不会分出 .127 及之后地址
	srv := New(cfg, func(logbook.Level, string, ...any) {})

	ip1, ok := srv.alloc("aa:bb:cc:dd:ee:01", nil)
	if !ok || ip1.String() != "192.168.5.125" {
		t.Fatalf("第 1 次分配 = %v (%v)，期望 192.168.5.125", ip1, ok)
	}
	ip2, ok := srv.alloc("aa:bb:cc:dd:ee:02", nil)
	if !ok || ip2.String() != "192.168.5.126" {
		t.Fatalf("第 2 次分配 = %v (%v)，期望 192.168.5.126", ip2, ok)
	}
	if ip3, ok := srv.alloc("aa:bb:cc:dd:ee:03", nil); ok {
		t.Fatalf("越过 /25 广播地址后仍分配出 %v", ip3)
	}
}
