package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"pxeserv/internal/app"
	"pxeserv/internal/config"
	"pxeserv/internal/locales"
	"pxeserv/internal/logbook"
	"pxeserv/internal/netif"
)

func TestNativeInterfaceDraft(t *testing.T) {
	cfg := config.Default()
	cfg.ServerIP = "192.168.5.1"
	cfg.NextServer = cfg.ServerIP
	cfg.Router = cfg.ServerIP
	cfg.DNS = "1.1.1.1"
	a := app.NewInMemory(cfg)
	u := New(a)
	u.Load()
	target := netif.Iface{Name: "内部", IP: "192.168.100.1", Mask: "255.255.255.0", Broadcast: "192.168.100.255"}
	u.ifaces = []netif.Iface{target}
	u.applyIface(target)
	if u.cfg.ServerIP != target.IP || u.cfg.NextServer != target.IP || u.cfg.Router != target.IP || u.cfg.PoolStart != "192.168.100.2" || u.cfg.Broadcast != target.Broadcast {
		t.Fatalf("incomplete network update: %+v", u.cfg)
	}
	if u.cfg.DNS != "1.1.1.1" {
		t.Fatal("custom DNS overwritten")
	}
	if !u.dirty() {
		t.Fatal("draft not marked dirty")
	}
	if cfg.ServerIP != "192.168.5.1" {
		t.Fatal("unsaved draft changed active config")
	}
	u.applyIface(netif.Iface{Name: "无地址"})
	if u.cfg.ServerIP != target.IP {
		t.Fatal("invalid interface changed config")
	}
}
func TestNativeDirtyNumericInput(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	if u.dirty() {
		t.Fatal("loaded config marked dirty")
	}
	u.port = "8080"
	if !u.dirty() {
		t.Fatal("numeric change not detected")
	}
}
func TestNativeRefreshPreservesDraftSelection(t *testing.T) {
	cfg := config.Default()
	cfg.ServerIP = "192.168.5.1"
	a := app.NewInMemory(cfg)
	u := New(a)
	u.Load()
	u.ifaces = []netif.Iface{{Name: "A", IP: "192.168.5.1"}, {Name: "B", IP: "192.168.100.1"}}
	u.iface = "B · 192.168.100.1"
	u.Refresh()
	if u.iface != "B · 192.168.100.1" {
		t.Fatalf("selection overwritten: %q", u.iface)
	}
}
func TestNativeMenuPathBinding(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.menuLoaded = true
	u.menuPath = u.currentMenuPath()
	u.cfg.MenuFile = "other.ipxe"
	if u.menuPath == u.configuredMenuPath() {
		t.Fatal("changed menu path still considered loaded")
	}
}
func TestTroubleLines(t *testing.T) {
	lines := []app.LogLine{
		{Level: logbook.Info, Text: "12:00:00 I 服务已启动"},
		{Level: logbook.Warn, Text: "12:00:01 W TFTPd：GET a.efi 失败：不是文件"},
		{Level: logbook.Error, Text: "12:00:02 E 服务异常退出：udp:69 占用"},
	}
	got := troubleLines(lines)
	if len(got) != 2 || got[0] != lines[1].Text || got[1] != lines[2].Text {
		t.Fatalf("只应挑出警告与错误两行，实际 %v", got)
	}
	if text := logText(lines); len(strings.Split(text, "\n")) != 3 {
		t.Fatalf("logText 应保留全部三行：%q", text)
	}
}
func TestNativeStartFailureMessage(t *testing.T) {
	result := startResult("", "端口占用")
	if !strings.Contains(result, "配置已保存") || !strings.Contains(result, "服务未启动") {
		t.Fatalf("ambiguous result: %s", result)
	}
}
func TestNativeBusyKeepsPage(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.busy = true
	u.msg = "处理中…"
	tt := ui.NewTester(u.View, 1000, 800)
	for _, label := range []string{"服务", "网络", "服务器 IP"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("busy view removed %s", label)
		}
	}
}

// TestLanguageLabels 覆盖语言下拉与配置值之间的换算，以及防火墙状态码的译法：
// 这三处是「配置里存标签、界面上显示文字」的唯一桥梁，写错就会切到空语言。
func TestLanguageLabels(t *testing.T) {
	for _, tc := range []struct{ tag, label string }{
		{"", locales.LanguageSystem()},
		{"zh-Hans", locales.LanguageChinese()},
		{"en", locales.LanguageEnglish()},
		{"de", locales.LanguageSystem()}, // 未支持的语言按「跟随系统」处理
	} {
		if got := langLabel(tc.tag); got != tc.label {
			t.Errorf("langLabel(%q) = %q，期望 %q", tc.tag, got, tc.label)
		}
	}
	for _, tc := range []struct{ label, tag string }{
		{locales.LanguageSystem(), ""},
		{locales.LanguageChinese(), "zh-Hans"},
		{locales.LanguageEnglish(), "en"},
		{"Español", ""},
	} {
		if got := langTag(tc.label); got != tc.tag {
			t.Errorf("langTag(%q) = %q，期望 %q", tc.label, got, tc.tag)
		}
	}
}

// TestFirewallLabel 保证后端状态码不会直接漏到界面上。
func TestFirewallLabel(t *testing.T) {
	for state, want := range map[string]string{
		"on":      locales.FirewallOn(),
		"off":     locales.FirewallOff(),
		"unknown": locales.FirewallUnknown(),
		"":        locales.FirewallUnknown(),
	} {
		if got := firewallLabel(state); got != want {
			t.Errorf("firewallLabel(%q) = %q，期望 %q", state, got, want)
		}
	}
}

func TestMenuDirtyAndViewHint(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.tab = 2
	u.menuLoaded = true
	u.menuPath = u.currentMenuPath()
	u.menu = "#!ipxe\nboot\n"
	u.menuSaved = u.menu
	if u.menuDirty() {
		t.Fatal("刚载入的菜单不应标为脏")
	}
	u.menu = "#!ipxe\nreboot\n"
	if !u.menuDirty() {
		t.Fatal("修改后的菜单应标为脏")
	}
	tt := ui.NewTester(u.View, 1000, 800)
	if _, ok := tt.Find(locales.MenuUnsaved()); !ok {
		t.Fatal("菜单未保存时应展示提示")
	}
}

func TestAddArchMappingValidation(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.cfg.BootFileByArch = nil

	u.archKey = "abc"
	u.archValue = "ipxe.efi"
	if u.addArchMapping() || u.msgKind != msgError {
		t.Fatalf("非数字架构号应校验失败: kind=%d msg=%q", u.msgKind, u.msg)
	}

	u.archKey = "-1"
	u.archValue = "ipxe.efi"
	if u.addArchMapping() || u.msgKind != msgError {
		t.Fatalf("负数架构号应校验失败")
	}

	u.archKey = "7"
	u.archValue = "   "
	if u.addArchMapping() || u.msgKind != msgError {
		t.Fatalf("空文件名应校验失败")
	}

	u.archKey = " 10 "
	u.archValue = " arm64.efi "
	if !u.addArchMapping() {
		t.Fatalf("合法输入添加失败: %q", u.msg)
	}
	if got := u.cfg.BootFileByArch["10"]; got != "arm64.efi" {
		t.Fatalf("映射值 = %q，期望 arm64.efi", got)
	}
	if u.archKey != "" || u.archValue != "" {
		t.Fatal("添加成功后应清空输入框")
	}
}

func TestFilteredLogText(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.logs = []app.LogLine{
		{Level: logbook.Info, Text: "info line"},
		{Level: logbook.Warn, Text: "warn line"},
		{Level: logbook.Error, Text: "err line"},
	}
	u.filter = 0
	if got := u.filteredLogText(); !strings.Contains(got, "info line") || !strings.Contains(got, "err line") {
		t.Fatalf("全量复制内容异常: %q", got)
	}
	u.filter = 1
	if got := u.filteredLogText(); strings.Contains(got, "info line") || !strings.Contains(got, "warn line") || !strings.Contains(got, "err line") {
		t.Fatalf("故障筛选复制内容异常: %q", got)
	}
}

func TestProxyDHCPShowsPoolHint(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.cfg.ProxyDHCP = true
	tt := ui.NewTester(u.View, 1000, 800)
	if _, ok := tt.Find(locales.ProxyPoolHint()); !ok {
		t.Fatal("开启 ProxyDHCP 时应展示地址池禁用提示")
	}
}

func TestInferMsgKind(t *testing.T) {
	if got := inferMsgKind(locales.ConfigSavedApplied()); got != msgSuccess {
		t.Fatalf("成功消息分级 = %d，期望 msgSuccess", got)
	}
	if got := inferMsgKind(locales.HttpPortNotInt()); got != msgError {
		t.Fatalf("错误消息分级 = %d，期望 msgError", got)
	}
	if got := inferMsgKind(locales.FolderUnchanged()); got != msgInfo {
		t.Fatalf("普通提示分级 = %d，期望 msgInfo", got)
	}
}
