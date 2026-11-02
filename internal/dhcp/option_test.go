package dhcp

import (
	"bytes"
	"testing"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

// TestPxeVendorOptions 校验 Option 43 的子选项布局，以及 Subopt1 开关。
func TestPxeVendorOptions(t *testing.T) {
	cfg := &config.Config{NextServer: "192.168.5.1"}
	s := New(cfg, func(logbook.Level, string, ...any) {})

	// 子项 6（发现控制）+ 子项 8（启动服务器清单）是前缀，后面必须跟子项 10（菜单）与 11（提示），
	// 否则 Intel UNDI 的 BIOS ROM 会报 PXE-E74 直接退出（实测）。
	head := []byte{6, 1, 0x03, 8, 7, 0, 0, 1, 192, 168, 5, 1}

	got := s.pxeVendorOptions("undionly.kpxe")
	if !bytes.HasPrefix(got, head) {
		t.Fatalf("未开 Subopt1 时 = %v, 应以 %v 开头", got, head)
	}
	if !bytes.Contains(got, []byte{10, 20, 0, 0, 17}) {
		t.Fatalf("缺少子项 10（引导菜单）：%v", got)
	}
	if !bytes.Contains(got, []byte{11, 23, 3}) {
		t.Fatalf("缺少子项 11（菜单提示）：%v", got)
	}

	cfg.Opt43Subopt1 = true
	if got := s.pxeVendorOptions("undionly.kpxe"); !bytes.HasPrefix(got, append([]byte{1, 4, 0, 0, 0, 0}, head...)) {
		t.Fatalf("开启 Subopt1 后 = %v, 应以子项 1 开头", got)
	}

	cfg.NextServer = "not-an-ip"
	if got := s.pxeVendorOptions("x"); got != nil {
		t.Fatalf("NextServer 非法时应返回 nil, 实际 %v", got)
	}
}

// TestApplyExtras 确认额外选项能写进应答并覆盖标准选项。
func TestApplyExtras(t *testing.T) {
	cfg := &config.Config{NextServer: "192.168.5.1", ExtraOptions: "175.6=ip:1.8.1.1; 175.8=ip:192.168.5.1; 51=u32:7200"}
	extra, err := cfg.ParseExtraOptions()
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	s := New(cfg, func(logbook.Level, string, ...any) {})
	s.extra = extra
	r := &Packet{Options: map[byte][]byte{}}
	r.SetU32(OptLeaseTime, 3600)
	s.applyExtras(r)

	if got, want := r.Options[175], []byte{6, 4, 1, 8, 1, 1, 8, 4, 192, 168, 5, 1}; !bytes.Equal(got, want) {
		t.Fatalf("option 175 = %v, 期望 %v", got, want)
	}
	if got, want := r.Options[OptLeaseTime], []byte{0, 0, 0x1c, 0x20}; !bytes.Equal(got, want) {
		t.Fatalf("option 51 应被覆盖为 7200, 实际 %v", got)
	}
}
