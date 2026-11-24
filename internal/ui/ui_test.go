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

// TestIfaceLabelMarksUnconnected 保证没链路的网卡在下拉里带标注，已启用的不带。
func TestIfaceLabelMarksUnconnected(t *testing.T) {
	up := netif.Iface{Name: "以太网", IP: "192.168.5.1", Up: true}
	if got := ifaceLabel(up); got != "以太网 · 192.168.5.1" {
		t.Fatalf("已启用的网卡不应带后缀：%q", got)
	}
	down := netif.Iface{Name: "以太网 2", IP: "192.168.1.151"}
	if got := ifaceLabel(down); !strings.Contains(got, locales.IfaceDown()) {
		t.Fatalf("未连接的网卡应标注：%q", got)
	}
	// 没配 IPv4 的网卡不能拼出「名字 ·」（空分隔符），要另给标注。
	bare := netif.Iface{Name: "以太网 3", Up: true}
	if got := ifaceLabel(bare); strings.Contains(got, " · ") || !strings.Contains(got, locales.IfaceNoAddr()) {
		t.Fatalf("无 IPv4 的网卡标注异常：%q", got)
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
func TestNativeBusyKeepsPage(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.busy = true
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
	u.tab = tabMenu
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
	u.addArchMapping()
	if lastToastType(u) != "error" {
		t.Fatalf("非数字架构号应校验失败: pending=%v", u.pending)
	}

	u.archKey = "-1"
	u.archValue = "ipxe.efi"
	u.addArchMapping()
	if lastToastType(u) != "error" {
		t.Fatalf("负数架构号应校验失败")
	}

	u.archKey = "7"
	u.archValue = "   "
	u.addArchMapping()
	if lastToastType(u) != "error" {
		t.Fatalf("空文件名应校验失败")
	}

	u.archKey = " 10 "
	u.archValue = " arm64.efi "
	u.addArchMapping()
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
	if got := u.filteredLogText(troubleLines(u.logs)); !strings.Contains(got, "info line") || !strings.Contains(got, "err line") {
		t.Fatalf("全量复制内容异常: %q", got)
	}
	u.filter = 1
	if got := u.filteredLogText(troubleLines(u.logs)); strings.Contains(got, "info line") || !strings.Contains(got, "warn line") || !strings.Contains(got, "err line") {
		t.Fatalf("故障筛选复制内容异常: %q", got)
	}
}

// TestHomeAndSettingsTabs 保证「界面」选项只出现在新的「设置」页，主页不再出现。
func TestHomeAndSettingsTabs(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.firewallQueried = true // 设置页会自动查一次防火墙，测试里没有窗口，先跳过
	home := ui.NewTester(u.View, 1000, 800)
	if _, ok := home.Find(locales.LanguageSystem()); ok {
		t.Fatal("主页不应再展示界面语言选项")
	}
	u.tab = tabSettings
	settings := ui.NewTester(u.View, 1000, 800)
	if _, ok := settings.Find(locales.LanguageSystem()); !ok {
		t.Fatal("设置页应展示界面语言选项")
	}
}

// TestFirewallOnSettingsTab 保证防火墙只在「设置」页，「关于」页只剩关于信息。
func TestFirewallOnSettingsTab(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.firewallQueried = true   // 设置页会自动查一次防火墙，测试里没有窗口，先跳过
	u.firewallSupported = true // 按 Windows 的布局验证（其它平台整块隐藏，见下一个测试）

	u.tab = tabAbout
	about := ui.NewTester(u.View, 1000, 800)
	if _, ok := about.Find(locales.AllowProgram()); ok {
		t.Fatal("「关于」页不应再出现防火墙按钮")
	}
	if _, ok := about.Find(locales.SectionAbout()); !ok {
		t.Fatal("「关于」页应保留关于信息")
	}

	u.tab = tabSettings
	settings := ui.NewTester(u.View, 1000, 800)
	if _, ok := settings.Find(locales.AllowProgram()); !ok {
		t.Fatal("「设置」页应包含防火墙按钮")
	}
}

// TestFirewallMovedHint 保证放行标记里的旧路径会在设置页显示成「请重新放行」。
func TestFirewallMovedHint(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.firewallQueried = true
	u.firewallSupported = true
	old := `D:\Old\PxeServer.exe`
	u.firewallMoved = old
	u.tab = tabSettings
	settings := ui.NewTester(u.View, 1000, 800)
	if _, ok := settings.Find(locales.FirewallMoved(old)); !ok {
		t.Fatal("程序移动后设置页应提示重新放行")
	}
}

// TestFirewallHiddenWhenUnsupported 非 Windows 没有「按程序放行」：设置页不画防火墙那一段，
// 免得按钮点下去只弹一句“此平台不需要放行防火墙”。
func TestFirewallHiddenWhenUnsupported(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.firewallQueried = true
	u.firewallSupported = false
	u.tab = tabSettings
	settings := ui.NewTester(u.View, 1000, 800)
	if _, ok := settings.Find(locales.AllowProgram()); ok {
		t.Fatal("不支持放行的平台不应出现防火墙按钮")
	}
	if _, ok := settings.Find(locales.SectionLanguage()); !ok {
		t.Fatal("其余设置项应照常显示")
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

func TestResultLevels(t *testing.T) {
	if got := ok("x", nil); got.kind != msgSuccess {
		t.Fatalf("成功结果级别 = %d，期望 msgSuccess", got.kind)
	}
	if got := notice("x", nil); got.kind != msgInfo {
		t.Fatalf("提示结果级别 = %d，期望 msgInfo", got.kind)
	}
	if got := failed("x"); got.kind != msgError {
		t.Fatalf("失败结果级别 = %d，期望 msgError", got.kind)
	}
	if got := silent(nil); !got.silent {
		t.Fatal("静默结果不应弹提示")
	}
	if got := done(nil); got.kind != msgSuccess || got.text != locales.Done() {
		t.Fatalf("done 结果 = %+v", got)
	}
}

// lastToastType 返回队列里最后一条提示的类型，供校验类测试断言。
func lastToastType(u *UI) string {
	if len(u.pending) == 0 {
		return ""
	}
	return u.pending[len(u.pending)-1].Type
}

// TestToastLevels 锁定「成功自动消失、错误常驻」这两条策略：类型写错会让
// 错误提示 4 秒后自己消失，用户就看不到失败原因了。
func TestToastLevels(t *testing.T) {
	u := &UI{}
	u.setMsg(msgSuccess, "已保存")
	u.setMsg(msgError, "保存失败")
	if len(u.pending) != 2 {
		t.Fatalf("应排队两条 Toast，实际 %d", len(u.pending))
	}
	if t0 := u.pending[0]; t0.Type != "success" || t0.Timeout != 0 || t0.Title != "已保存" {
		t.Fatalf("成功 Toast = %+v", t0)
	}
	if t1 := u.pending[1]; t1.Type != "error" || t1.Timeout >= 0 {
		t.Fatalf("错误 Toast 应常驻：%+v", t1)
	}
	u.setMsg(msgError, "又失败了")
	if len(u.pending) != 2 || u.pending[1].Title != "又失败了" {
		t.Fatalf("新错误应顶掉旧错误：%+v", u.pending)
	}
}

// TestToastAndBadge 确认消息真的上了界面，而且未保存标记在同一行显示。
func TestToastAndBadge(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	u.setMsg(msgSuccess, "已保存")
	tt := ui.NewTester(u.View, 1000, 800)
	if _, ok := tt.Find("已保存"); !ok {
		t.Fatal("Toast 未出现在界面里")
	}
	u.cfg.HTTPPort++
	tt = ui.NewTester(u.View, 1000, 800)
	if _, ok := tt.Find("● " + locales.UnsavedBadge()); !ok {
		t.Fatal("改了配置应显示未保存标记")
	}
	u.setMsg(msgError, "保存失败")
	tt = ui.NewTester(u.View, 1000, 800)
	if _, ok := tt.Find("保存失败"); !ok {
		t.Fatal("错误 Toast 未出现在界面里")
	}
}

func TestNativeSetcapHint(t *testing.T) {
	u := New(app.NewInMemory(config.Default()))
	u.Load()
	hint := u.setcapHint()
	if !strings.Contains(hint, "sudo setcap 'cap_net_bind_service=+ep' ") {
		t.Fatalf("setcap 命令不完整：%q", hint)
	}
	if strings.HasSuffix(hint, " ") {
		t.Fatalf("setcap 命令缺少可执行文件路径：%q", hint)
	}
}
