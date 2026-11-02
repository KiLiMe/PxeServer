package dhcp

import (
	"net"
	"testing"
	"time"

	"pxeserv/internal/config"
)

// 传统 BIOS（arch=0）走“最小应答”，与正版 TinyPXE 实测报文一致：
// 不发 option 43，也不发 60/66/67，只靠 BOOTP 的 file/siaddr 指路。
// 依据：正版对同一台 Realtek PCIe GBE（Intel UNDI PXE-2.1 build 083）只回
// 1/3/6/13/28/51/53/54 + file='ipxe.pxe' + sname='PXE-SRV' + option 175，客户端就正常 TFTP 引导；
// 我们发完整 option 43 时它报 “PXE-E74: Bad or missing PXE menu and/or prompt information”。
func TestLegacyGetsMinimalReply(t *testing.T) {
	s, srv, cli := newTestServer(t, func(c *config.Config) {
		c.NoOption43 = true
		c.Opt43Subopt1 = true
	})
	req := testRequest(0x1a2b3c01, macA, Discover, func(p *Packet) {
		p.SetOpt(OptClientArch, []byte{0x00, 0x00})
		p.SetString(OptVendorClass, "PXEClient:Arch:00000:UNDI:002001")
	})
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	r, err := recv(t, cli, 2*time.Second)
	if err != nil {
		t.Fatalf("没收到 OFFER: %v", err)
	}
	if v := r.Get(OptVendorSpecific); len(v) != 0 {
		t.Fatalf("BIOS 不该收到 Option 43（会触发 PXE-E74），实际 %x", v)
	}
	if v := r.Get(OptVendorClass); len(v) != 0 {
		t.Fatalf("BIOS 不该收到 Option 60，实际 %q", v)
	}
	if v := r.Get(OptTFTPServer); len(v) != 0 {
		t.Fatalf("BIOS 不该收到 Option 66，实际 %q", v)
	}
	if v := r.Get(OptBootFile); len(v) != 0 {
		t.Fatalf("BIOS 不该收到 Option 67，实际 %q", v)
	}
	if got := bootFileName(r); got != "undionly.kpxe" {
		t.Fatalf("BOOTP file 字段 = %q，应为 undionly.kpxe", got)
	}
	if got := string(trimNul(r.SName[:])); got == "" {
		t.Fatalf("sname 应写主机名，实际为空")
	}
}

// bootFileName / trimNul 是小工具：取 BOOTP 固定字段里的字符串。
func bootFileName(p *Packet) string { return string(trimNul(p.File[:])) }

func trimNul(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// EFI（arch=7）在 no_option43=true 时不下发 Option 43：个别 UEFI 固件见到它就卡住。
func TestNoOption43ForEFI(t *testing.T) {
	s, srv, cli := newTestServer(t, func(c *config.Config) {
		c.NoOption43 = true
	})
	req := testRequest(0x1a2b3c02, macB, Discover, nil) // 默认就是 arch=7
	s.handle(srv, req, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68}, false)

	r, err := recv(t, cli, 2*time.Second)
	if err != nil {
		t.Fatalf("没收到 OFFER: %v", err)
	}
	if v := r.Get(OptVendorSpecific); len(v) != 0 {
		t.Fatalf("EFI 的应答不该有 Option 43，实际 %x", v)
	}
	if got := string(r.Get(OptBootFile)); got != "snponly.efi" {
		t.Fatalf("boot file = %q", got)
	}
}
