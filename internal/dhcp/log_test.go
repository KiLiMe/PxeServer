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

func TestPacketBurstHit(t *testing.T) {
	var b packetBurst
	const key = "aa DISCOVER → OFFER 192.168.5.2"
	for i := 1; i <= 6; i++ {
		ok, n := b.hit(key)
		if n != i {
			t.Fatalf("第 %d 次出现时计数为 %d", i, n)
		}
		if want := i == 1 || i%5 == 0; ok != want {
			t.Errorf("第 %d 次出现时 ok=%v，应为 %v", i, ok, want)
		}
	}
}

func TestPacketBurstResetsOnKeyChange(t *testing.T) {
	var b packetBurst
	b.hit("a")
	b.hit("a")
	ok, n := b.hit("b")
	if !ok || n != 1 {
		t.Fatalf("换 key 后应从 1 重新计数并打日志，得到 ok=%v n=%d", ok, n)
	}
}

func TestPacketBurstResetsAfterWindow(t *testing.T) {
	var b packetBurst
	b.hit("a")
	b.hit("a")
	// 把上次时间推到窗口之外，比 sleep 5 秒快得多。
	b.last = time.Now().Add(-burstWindow - time.Second)
	ok, n := b.hit("a")
	if !ok || n != 1 {
		t.Fatalf("窗口过期后应重新打日志，得到 ok=%v n=%d", ok, n)
	}
}

func TestReplySummary(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		addr     string
		bootFile string
		bootSrv  string
		want     string
	}{
		{"分配地址与启动文件", "OFFER", "192.168.5.2", "snponly.efi", "192.168.5.1",
			"→ OFFER 192.168.5.2，启动文件 snponly.efi（TFTP/HTTP 192.168.5.1）"},
		{"ProxyDHCP 不分配地址", "ACK", "", "ipxe.efi", "192.168.5.1",
			"→ ACK，启动文件 ipxe.efi（TFTP/HTTP 192.168.5.1）"},
		{"普通客户端只拿地址", "ACK", "192.168.5.3", "", "", "→ ACK 192.168.5.3"},
	}
	for _, c := range cases {
		if got := replySummary(c.kind, c.addr, c.bootFile, c.bootSrv); got != c.want {
			t.Errorf("%s：得到 %q，应为 %q", c.name, got, c.want)
		}
	}
}

func TestLogPacketCollapsesRepeats(t *testing.T) {
	var logs []string
	var levels []logbook.Level
	s := New(config.Default(), func(level logbook.Level, f string, a ...any) {
		levels = append(levels, level)
		logs = append(logs, fmt.Sprintf(f, a...))
	})
	const mac = "aa:bb:cc:dd:ee:ff"
	const result = "→ OFFER 192.168.100.100，启动文件 ipxe.efi（TFTP/HTTP 192.168.100.1）"
	for i := 0; i < 5; i++ {
		s.logPacket(mac, "DHCPDISCOVER", 7, false, result)
	}
	if len(logs) != 2 {
		t.Fatalf("5 次相同报文应只留 2 行日志（第 1 次与第 5 次），实际 %d 行：%v", len(logs), logs)
	}
	if levels[0] != logbook.Info {
		t.Errorf("正常应答应为信息级，实际 %v", levels[0])
	}
	if want := "DHCPd：MAC=" + mac + " DHCPDISCOVER " + result; logs[0] != want {
		t.Errorf("首行日志为 %q，应为 %q", logs[0], want)
	}
	if !strings.Contains(logs[1], "（同 MAC 第 5 次）") {
		t.Errorf("第 5 次应标记合并次数，实际为 %q", logs[1])
	}
}

func TestLogPacketMarksBootService(t *testing.T) {
	var logs []string
	s := New(config.Default(), func(_ logbook.Level, f string, a ...any) {
		logs = append(logs, fmt.Sprintf(f, a...))
	})
	s.logPacket("aa:bb:cc:dd:ee:ff", "DHCPREQUEST", 7, true, "→ ACK")
	want := "DHCPd：MAC=aa:bb:cc:dd:ee:ff DHCPREQUEST（4011，架构=7） → ACK"
	if len(logs) != 1 || logs[0] != want {
		t.Fatalf("得到 %v，应为 [%q]", logs, want)
	}
}

func TestStartupSummary(t *testing.T) {
	addr := &net.UDPAddr{IP: net.IPv4(192, 168, 5, 1), Port: 67}
	dests := []string{"192.168.5.255:68", "255.255.255.255:68"}
	cases := []struct {
		name string
		bind bool
		want string
	}{
		{"4011 就绪", true,
			"DHCPd：已在 192.168.5.1:67 上监听（普通 DHCP，地址池 192.168.5.2×10，广播应答 → 192.168.5.255:68、255.255.255.255:68，PXE 4011）"},
		{"4011 未绑定", false,
			"DHCPd：已在 192.168.5.1:67 上监听（普通 DHCP，地址池 192.168.5.2×10，广播应答 → 192.168.5.255:68、255.255.255.255:68，PXE 4011 未绑定）"},
	}
	for _, c := range cases {
		got := startupSummary(addr, "普通 DHCP", "192.168.5.2", 10, dests, c.bind)
		if got != c.want {
			t.Errorf("%s：得到 %q，应为 %q", c.name, got, c.want)
		}
	}
}

func TestPacketLevel(t *testing.T) {
	cases := []struct {
		result string
		want   logbook.Level
	}{
		{"→ OFFER 192.168.5.2，启动文件 ipxe.efi（TFTP/HTTP 192.168.5.1）", logbook.Info},
		{"→ 地址池 192.168.5.2×10 已满，未应答", logbook.Warn},
		{"→ 地址冲突，已回收", logbook.Warn},
		{"→ 已释放", logbook.Info},
		{"", logbook.Info},
	}
	for _, c := range cases {
		if got := packetLevel(c.result); got != c.want {
			t.Errorf("packetLevel(%q) = %v，应为 %v", c.result, got, c.want)
		}
	}
}
