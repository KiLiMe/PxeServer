package ui

import (
	"encoding/json"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/app"
	"pxeserv/internal/config"
	"pxeserv/internal/locales"
	"pxeserv/internal/logbook"
	"pxeserv/internal/netif"
)

const (
	msgInfo = iota
	msgSuccess
	msgWarn
	msgError
)

type UI struct {
	savedKey                      string
	tasks                         sync.WaitGroup
	app                           *app.App
	Win                           *mygo.Window
	cfg                           config.Config
	status                        app.Status
	ifaces                        []netif.Iface
	iface                         string
	tab                           int
	busy                          bool
	loaded                        bool
	firewall                      string
	firewallQueried               bool
	port, poolSize, leaseTime     string
	msg, menu, archKey, archValue string
	msgKind                       int
	lang                          string
	menuLoaded                    bool
	menuPath, menuSaved           string
	filter                        int
	logs                          []app.LogLine
	logSeq                        uint64
}

func New(a *app.App) *UI {
	u := &UI{app: a, cfg: *clone(a.GetConfig()), logs: a.Logs()}
	u.port = strconv.Itoa(u.cfg.HTTPPort)
	u.poolSize = strconv.Itoa(u.cfg.PoolSize)
	u.leaseTime = strconv.FormatUint(uint64(u.cfg.LeaseTime), 10)
	u.msg = locales.LoadingConfig()
	return u
}
func clone(c *config.Config) *config.Config {
	if c == nil {
		return config.Default()
	}
	v := *c
	v.BootFileByArch = map[string]string{}
	for k, x := range c.BootFileByArch {
		v.BootFileByArch[k] = x
	}
	return &v
}
func (u *UI) setMsg(kind int, text string) {
	u.msg = text
	u.msgKind = kind
}

func inferMsgKind(text string) int {
	switch text {
	case "", locales.Done(), locales.ServicesStopped(), locales.SavedStarted(),
		locales.ConfigSavedApplied(), locales.MenuSaved(), locales.DebugAddrCopied(),
		locales.LogCopied(), locales.LogCleared(), locales.LogExported(),
		locales.FirewallRefreshed(), locales.MenuOpened():
		return msgSuccess
	}
	if strings.Contains(text, "失败") || strings.Contains(text, "不能") ||
		strings.Contains(text, "必须") || strings.Contains(text, "超出") ||
		strings.Contains(text, "未启动") || strings.Contains(text, "未找到") ||
		strings.Contains(text, "没有可用") || strings.Contains(text, "非法") ||
		strings.Contains(text, "failed") || strings.Contains(text, "must be") ||
		strings.Contains(text, "cannot") || strings.Contains(text, "not start") {
		return msgError
	}
	return msgInfo
}

func (u *UI) async(fn func() (string, func())) {
	if u.busy {
		return
	}
	u.busy = true
	u.setMsg(msgInfo, locales.Working())
	u.tasks.Add(1)
	u.Win.Invalidate()
	go func() {
		defer u.tasks.Done()
		msg, apply := fn()
		u.Win.Update(func() {
			u.busy = false
			text := msgOr(msg, locales.Done())
			u.setMsg(inferMsgKind(text), text)
			if apply != nil {
				apply()
			}
			u.Win.Invalidate()
		})
	}()
}
func (u *UI) run(fn func() string) { u.async(func() (string, func()) { return fn(), nil }) }
func (u *UI) refreshFirewall() {
	u.firewallQueried = true
	u.async(func() (string, func()) {
		state := u.app.FirewallState()
		label := firewallLabel(state)
		return locales.FirewallRefreshed(), func() { u.firewall = label }
	})
}
func (u *UI) runFirewall(fn func() string) {
	u.firewallQueried = true
	u.async(func() (string, func()) {
		msg := fn()
		state := u.app.FirewallState()
		firewall := firewallLabel(state)
		return msg, func() { u.firewall = firewall }
	})
}
func (u *UI) message(title, msg string) {
	u.Win.Update(func() { u.setMsg(msgError, title+": "+msg); u.Win.Invalidate() })
}
func (u *UI) Load() {
	u.cfg = *clone(u.app.GetConfig())
	u.ifaces = u.app.ListInterfaces()
	u.iface = u.manualLabel()
	u.syncIface()
	u.firewall = locales.FirewallPending()
	u.lang = langLabel(u.cfg.Language)
	u.port = strconv.Itoa(u.cfg.HTTPPort)
	u.poolSize = strconv.Itoa(u.cfg.PoolSize)
	u.leaseTime = strconv.FormatUint(uint64(u.cfg.LeaseTime), 10)
	u.savedKey = u.draftKey()
	u.loaded = true
	u.msg = ""
}
func (u *UI) Refresh() {
	if u.busy {
		return
	}
	u.status = u.app.Status()
	if logs, seq, changed := u.app.LogsSince(u.logSeq); changed {
		u.logs = logs
		u.logSeq = seq
	}
}
func (u *UI) View(c *ui.Context) {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f3f5f7")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#f0f4f8")
	t.Border = ui.Hex("#dfe3e8")
	t.Accent = ui.Hex("#0067c0")
	t.Font = "Segoe UI Variable Text, Segoe UI, Microsoft YaHei"
	t.FontSize = 13
	t.Radius = 8
	c.SetTheme(t)
	ui.Column(c).Fill().Padding(18).Gap(12).Disabled(!u.loaded || u.busy).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AppTitle()).FontSize(20).Bold().Grow(1)
			ui.Text(c, statusText(u.status))
			ui.Button(c, locales.Start()).Disabled(u.busy || u.status.Running).OnClick(func() { u.saveAndStart(true) })
			ui.Button(c, locales.Stop()).Disabled(u.busy || !u.status.Running).OnClick(func() { u.run(func() string { u.app.StopServices(); return locales.ServicesStopped() }) })
			ui.Button(c, locales.SaveConfig()).Disabled(u.busy).OnClick(u.save)
		})
		if u.loaded && u.dirty() {
			noticeBox(c, locales.UnsavedChanges(), msgWarn)
		}
		if u.msg != "" {
			noticeBox(c, u.msg, u.msgKind)
		}
		ui.Segmented(c, &u.tab, locales.TabSettings(), locales.TabLeases(), locales.TabBootMenu(), locales.TabArch(), locales.TabLogs(), locales.TabFirewallAbout()).FillWidth()
		if u.tab == 5 && !u.firewallQueried && !u.busy {
			u.refreshFirewall()
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(12).Padding(4).Children(func() {
				switch u.tab {
				case 0:
					u.settings(c)
				case 1:
					u.leases(c)
				case 2:
					u.menuView(c)
				case 3:
					u.archView(c)
				case 4:
					u.logView(c)
				case 5:
					u.infoView(c)
				}
			})
		})
	})
}
func noticeBox(c *ui.Context, text string, kind int) {
	var bg, border ui.Color
	switch kind {
	case msgError:
		bg, border = ui.Hex("#fdf2f2"), ui.Hex("#f5c2c7")
	case msgSuccess:
		bg, border = ui.Hex("#f0f9f4"), ui.Hex("#badbcc")
	case msgWarn:
		bg, border = ui.Hex("#fff8e6"), ui.Hex("#ffe58f")
	default:
		bg, border = ui.Hex("#f0f6fc"), ui.Hex("#cfe2ff")
	}
	ui.Row(c).FillWidth().Padding(8).Radius(6).Background(bg).Border(1, border).Children(func() {
		ui.Text(c, text).Wrap()
	})
}
func statusText(s app.Status) string {
	if s.Running {
		return locales.StatusRunning(s.ServerIP)
	}
	return locales.StatusStopped()
}
func section(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(14).Gap(10).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() { ui.Text(c, title).FontSize(15).Bold(); fn() })
}
func field(c *ui.Context, label string, v *string) {
	ui.Column(c).Gap(4).Children(func() { ui.Text(c, label).FontSize(12); ui.TextInput(c, v).Height(32) })
}
func str(c *ui.Context, l string, p *string)    { field(c, l, p) }
func checkbox(c *ui.Context, l string, p *bool) { ui.Checkbox(c, p, l) }
func row(c *ui.Context, fs ...func()) {
	ui.Row(c).Gap(10).Children(func() {
		for _, f := range fs {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(f)
		}
	})
}
func sf(c *ui.Context, l string, p *string) func() { return func() { str(c, l, p) } }
func (i *UI) findIface() *netif.Iface {
	for j := range i.ifaces {
		if i.ifaces[j].Name+" · "+i.ifaces[j].IP == i.iface {
			return &i.ifaces[j]
		}
	}
	return nil
}
func (u *UI) draftKey() string {
	b, _ := json.Marshal(struct {
		Config            config.Config
		Port, Pool, Lease string
	}{u.cfg, u.port, u.poolSize, u.leaseTime})
	return string(b)
}
func (u *UI) dirty() bool { return u.savedKey != u.draftKey() }
func (u *UI) syncIface() {
	if u.iface != "" && u.iface != u.manualLabel() {
		return
	}
	u.iface = u.manualLabel()
	for _, i := range u.ifaces {
		if i.IP == u.cfg.ServerIP {
			u.iface = i.Name + " · " + i.IP
			return
		}
	}
}
func (u *UI) detectInterface() {
	u.async(func() (string, func()) {
		ip := u.app.DetectIP()
		ifaces := u.app.ListInterfaces()
		return "", func() {
			selected := u.iface
			u.ifaces = ifaces
			for _, i := range u.ifaces {
				if i.IP == ip {
					u.applyIface(i)
					return
				}
			}
			u.iface = selected
			u.setMsg(msgError, locales.IfaceNotFound())
		}
	})
}
func (u *UI) applyIface(i netif.Iface) {
	ip := net.ParseIP(i.IP).To4()
	maskIP := net.ParseIP(i.Mask).To4()
	if ip == nil || maskIP == nil {
		u.setMsg(msgError, locales.IfaceNoIPv4())
		u.syncIface()
		return
	}
	old := u.cfg.ServerIP
	u.cfg.ServerIP = i.IP
	u.cfg.SubnetMask = i.Mask
	u.cfg.Broadcast = i.Broadcast
	if u.cfg.NextServer == "" || u.cfg.NextServer == old {
		u.cfg.NextServer = i.IP
	}
	if u.cfg.Router == old {
		u.cfg.Router = i.IP
	}
	if u.cfg.DNS == old {
		u.cfg.DNS = i.IP
	}
	network := net.IPNet{IP: ip.Mask(net.IPMask(maskIP)), Mask: net.IPMask(maskIP)}
	pool := net.ParseIP(u.cfg.PoolStart)
	if pool == nil || !network.Contains(pool) {
		candidate := append(net.IP(nil), network.IP...)
		for count := 0; count < 2; count++ {
			for j := 3; j >= 0; j-- {
				candidate[j]++
				if candidate[j] != 0 {
					break
				}
			}
		}
		if candidate.Equal(ip) {
			for j := 3; j >= 0; j-- {
				candidate[j]++
				if candidate[j] != 0 {
					break
				}
			}
		}
		if network.Contains(candidate) && candidate.String() != i.Broadcast {
			u.cfg.PoolStart = candidate.String()
		}
	}
	u.syncIface()
	u.setMsg(msgSuccess, locales.IfaceApplied(i.Name))
}

// manualLabel 是「手动填写」在界面语言里的写法：网卡下拉没有选中真实网卡时的取值。
func (u *UI) manualLabel() string { return locales.ManualEntry() }

// setLanguage 立即切换界面语言，并把所选语言记进草稿配置（仍需保存配置才落盘）。
func (u *UI) setLanguage(label string) {
	u.cfg.Language = langTag(label)
	u.app.SetLanguage(u.cfg.Language)
	u.iface = ""
	u.syncIface()
	u.Win.Invalidate()
}

// langLabel 返回界面语言在语言下拉里的显示名。
func langLabel(tag string) string {
	switch tag {
	case "zh-Hans":
		return locales.LanguageChinese()
	case "en":
		return locales.LanguageEnglish()
	}
	return locales.LanguageSystem()
}

// langTag 返回语言下拉某一项对应的配置值，空串表示跟随系统。
func langTag(label string) string {
	switch label {
	case locales.LanguageChinese():
		return "zh-Hans"
	case locales.LanguageEnglish():
		return "en"
	}
	return ""
}

// firewallLabel 把后端的放行状态（on / off / unknown）换成界面语言的文字。
func firewallLabel(state string) string {
	switch state {
	case "on":
		return locales.FirewallOn()
	case "off":
		return locales.FirewallOff()
	}
	return locales.FirewallUnknown()
}

func (u *UI) settings(c *ui.Context) {
	section(c, locales.SectionLanguage(), func() {
		ui.Select(c, &u.lang, []string{locales.LanguageSystem(), locales.LanguageChinese(), locales.LanguageEnglish()}).FillWidth().OnChange(func() { u.setLanguage(u.lang) })
	})
	section(c, locales.SectionServices(), func() {
		checkbox(c, locales.ProxyDHCP(), &u.cfg.ProxyDHCP)
		checkbox(c, locales.EnableHTTP(), &u.cfg.HTTPEnable)
		row(c, sf(c, locales.BootRoot(), &u.cfg.TFTPRoot), sf(c, locales.HttpPort(), &u.port))
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.ChooseFolder()).OnClick(func() {
				current := u.cfg.TFTPRoot
				u.async(func() (string, func()) {
					path := u.app.PickFolder(current)
					if path == "" {
						return locales.FolderUnchanged(), nil
					}
					return "", func() { u.cfg.TFTPRoot = path }
				})
			})
			ui.Button(c, locales.OpenBootFolder()).OnClick(func() { u.run(func() string { return u.app.OpenBootFolder() }) })
		})
	})
	section(c, locales.SectionNetwork(), func() {
		opts := []string{u.manualLabel()}
		for _, x := range u.ifaces {
			opts = append(opts, x.Name+" · "+x.IP)
		}
		ui.Select(c, &u.iface, opts).FillWidth().OnChange(func() {
			if x := u.findIface(); x != nil {
				u.applyIface(*x)
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.Rescan()).OnClick(func() {
				selected := u.iface
				u.async(func() (string, func()) {
					ifaces := u.app.ListInterfaces()
					return "", func() {
						u.ifaces = ifaces
						if selected == "" || selected == u.manualLabel() {
							u.syncIface()
							return
						}
						u.iface = selected
						if u.findIface() == nil {
							u.iface = ""
							u.syncIface()
						}
					}
				})
			})
			ui.Button(c, locales.DetectIP()).OnClick(u.detectInterface)
			if x := u.findIface(); x != nil {
				ui.Button(c, locales.ApplyIface()).OnClick(func() { u.applyIface(*x) })
			}
		})
		if u.cfg.ProxyDHCP {
			noticeBox(c, locales.ProxyPoolHint(), msgWarn)
		}
		row(c, sf(c, locales.ServerIP(), &u.cfg.ServerIP), sf(c, locales.SubnetMask(), &u.cfg.SubnetMask), sf(c, locales.NextServer(), &u.cfg.NextServer))
		ui.Column(c).FillWidth().Gap(10).Disabled(u.cfg.ProxyDHCP).Children(func() {
			row(c, sf(c, locales.PoolStart(), &u.cfg.PoolStart), sf(c, locales.PoolSize(), &u.poolSize), sf(c, locales.LeaseSeconds(), &u.leaseTime))
			row(c, sf(c, locales.Router(), &u.cfg.Router), sf(c, locales.Dns(), &u.cfg.DNS), sf(c, locales.Broadcast(), &u.cfg.Broadcast), sf(c, locales.DomainName(), &u.cfg.DomainName))
		})
	})
	section(c, locales.SectionBootOptions(), func() {
		row(c, sf(c, locales.BootFile(), &u.cfg.BootFile), sf(c, locales.MenuFile(), &u.cfg.MenuFile), sf(c, locales.RootPath(), &u.cfg.RootPath), sf(c, locales.ClassID(), &u.cfg.ClassID))
		checkbox(c, locales.FilterUserClass(), &u.cfg.FilterUserClass)
		checkbox(c, locales.Opt43Subopt1(), &u.cfg.Opt43Subopt1)
		checkbox(c, locales.NoOption43(), &u.cfg.NoOption43)
		checkbox(c, locales.AutoStart(), &u.cfg.AutoStart)
		checkbox(c, locales.DebugAPI(), &u.cfg.DebugAPI)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.DebugAddr(msgOr(u.status.DebugAddr, locales.NotRunning()))).Grow(1)
			ui.Button(c, locales.CopyAddr()).Disabled(u.status.DebugAddr == "").OnClick(func() {
				mygo.Clipboard.WriteText(u.status.DebugAddr)
				u.setMsg(msgSuccess, locales.DebugAddrCopied())
			})
		})
		row(c, sf(c, locales.Option66(), &u.cfg.BootServerHost), sf(c, locales.Option67(), &u.cfg.BootFileOption), sf(c, locales.Option252(), &u.cfg.PrivateOption))
		field(c, locales.ExtraOptions(), &u.cfg.ExtraOptions)
	})
}
func startResult(saveMsg, startMsg string) string {
	if saveMsg != "" {
		return saveMsg
	}
	if startMsg != "" {
		return locales.SavedNotStarted(startMsg)
	}
	return locales.SavedStarted()
}
func (u *UI) save() { u.saveAndStart(false) }
func (u *UI) saveAndStart(start bool) {
	var err error
	if u.cfg.HTTPPort, err = strconv.Atoi(u.port); err != nil {
		u.setMsg(msgError, locales.HttpPortNotInt())
		return
	}
	if u.cfg.PoolSize, err = strconv.Atoi(u.poolSize); err != nil {
		u.setMsg(msgError, locales.PoolSizeNotInt())
		return
	}
	lease, err := strconv.ParseUint(u.leaseTime, 10, 32)
	if err != nil {
		u.setMsg(msgError, locales.LeaseNotNumber())
		return
	}
	u.cfg.LeaseTime = uint32(lease)
	draft := *clone(&u.cfg)
	u.async(func() (string, func()) {
		saveMsg := u.app.SaveConfig(draft)
		saved := saveMsg == ""
		result := saveMsg
		if saved {
			if start {
				result = startResult(saveMsg, u.app.StartServices())
			} else {
				result = locales.ConfigSavedApplied()
			}
		}
		return result, func() {
			if saved {
				u.cfg = *clone(u.app.GetConfig())
				u.port = strconv.Itoa(u.cfg.HTTPPort)
				u.poolSize = strconv.Itoa(u.cfg.PoolSize)
				u.leaseTime = strconv.FormatUint(uint64(u.cfg.LeaseTime), 10)
				u.savedKey = u.draftKey()
				if !u.menuDirty() {
					u.menuLoaded = false
					u.menuPath = ""
					u.menuSaved = ""
				}
				u.ifaces = u.app.ListInterfaces()
				u.syncIface()
			}
			u.Refresh()
		}
	})
}
func msgOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func (u *UI) leases(c *ui.Context) {
	section(c, locales.SectionLeases(), func() {
		ui.Text(c, locales.LeasesSummary(statusText(u.status), len(u.status.Leases)))
		for _, l := range u.status.Leases {
			lease := l
			ui.Row(c).Gap(14).Padding(5).Children(func() {
				ui.Text(c, lease.MAC+" · "+lease.IP+" · "+lease.ExpiresAt).Grow(1)
				ui.Text(c, locales.Seconds(lease.Remaining))
				ui.Button(c, locales.Release()).OnClick(func() { u.run(func() string { return u.app.ReleaseLease(lease.MAC) }) })
			})
		}
	})
}
func (u *UI) currentMenuPath() string {
	cfg := u.app.GetConfig()
	if cfg == nil {
		return ""
	}
	return filepath.Join(cfg.TFTPRoot, cfg.MenuFile)
}
func (u *UI) configuredMenuPath() string { return filepath.Join(u.cfg.TFTPRoot, u.cfg.MenuFile) }
func (u *UI) menuDirty() bool            { return u.menuLoaded && u.menu != u.menuSaved }

func (u *UI) menuView(c *ui.Context) {
	section(c, locales.SectionMenu(), func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.LoadFile()).OnClick(func() {
				path := u.currentMenuPath()
				u.async(func() (string, func()) {
					menu := u.app.ReadMenuFile()
					return locales.MenuLoaded(path), func() {
						u.menu = menu
						u.menuSaved = menu
						u.menuPath = path
						u.menuLoaded = true
					}
				})
			})
			ui.Button(c, locales.OpenEditor()).OnClick(func() {
				u.run(func() string {
					if msg := u.app.OpenMenuFile(); msg != "" {
						return msg
					}
					return locales.MenuOpened()
				})
			})
			ui.Button(c, locales.SaveMenu()).Disabled(!u.menuLoaded || u.menuPath != u.currentMenuPath() || u.configuredMenuPath() != u.currentMenuPath()).OnClick(func() {
				menu := u.menu
				u.async(func() (string, func()) {
					if errMsg := u.app.WriteMenuFile(menu); errMsg != "" {
						return errMsg, nil
					}
					return locales.MenuSaved(), func() { u.menuSaved = menu }
				})
			})
		})
		if u.menuDirty() {
			noticeBox(c, locales.MenuUnsaved(), msgWarn)
		}
		if u.menuLoaded {
			ui.TextArea(c, &u.menu).Height(420)
		} else {
			ui.Text(c, locales.MenuHint()).Wrap()
		}
	})
}
func (u *UI) addArchMapping() bool {
	key := strings.TrimSpace(u.archKey)
	val := strings.TrimSpace(u.archValue)
	n, err := strconv.Atoi(key)
	if key == "" || err != nil || n < 0 {
		u.setMsg(msgError, locales.ArchKeyInvalid())
		return false
	}
	if val == "" {
		u.setMsg(msgError, locales.ArchFileEmpty())
		return false
	}
	if u.cfg.BootFileByArch == nil {
		u.cfg.BootFileByArch = map[string]string{}
	}
	u.cfg.BootFileByArch[strconv.Itoa(n)] = val
	u.archKey = ""
	u.archValue = ""
	return true
}
func (u *UI) archView(c *ui.Context) {
	section(c, locales.SectionArch(), func() {
		keys := make([]string, 0, len(u.cfg.BootFileByArch))
		for k := range u.cfg.BootFileByArch {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { a, _ := strconv.Atoi(keys[i]); b, _ := strconv.Atoi(keys[j]); return a < b })
		for _, k := range keys {
			v := u.cfg.BootFileByArch[k]
			row(c, func() { ui.Text(c, k+" · "+archName(k)).Height(32) }, func() { ui.TextInput(c, &v).Height(32).OnChange(func() { u.cfg.BootFileByArch[k] = v }) }, func() { ui.Button(c, locales.Remove()).OnClick(func() { delete(u.cfg.BootFileByArch, k) }) })
		}
		row(c, sf(c, locales.CustomArch(), &u.archKey), sf(c, locales.BootFileLabel(), &u.archValue))
		ui.Button(c, locales.AddMapping()).OnClick(func() { u.addArchMapping() })
		ui.Text(c, locales.ArchFallback(u.cfg.BootFile))
		ui.Button(c, locales.SaveArch()).OnClick(u.save)
	})
}
func archName(k string) string {
	switch k {
	case "0":
		return locales.ArchBiosX86()
	case "6":
		return locales.ArchUefiX86()
	case "7":
		return locales.ArchUefiX64()
	case "9":
		return locales.ArchUefiX64Bc()
	case "10":
		return locales.ArchArm64()
	case "11":
		return locales.ArchArm64Bc()
	}
	return locales.ArchCustom()
}

// logText 把日志行拼成可直接复制的文本。
func logText(lines []app.LogLine) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return strings.Join(out, "\n")
}

// troubleLines 只挑出需要人看的行（警告与错误）。
// 级别由写日志的一方标注，不再靠中文关键词猜 —— 猜错方向的两个例子：
// 「拒绝 MAC 的请求」其实是正常拒绝，而「传输中断」不含任何关键词。
func troubleLines(lines []app.LogLine) []string {
	var out []string
	for _, l := range lines {
		if l.Level >= logbook.Warn {
			out = append(out, l.Text)
		}
	}
	return out
}
func (u *UI) filteredLogText() string {
	if u.filter == 1 {
		return strings.Join(troubleLines(u.logs), "\n")
	}
	return logText(u.logs)
}

func (u *UI) logView(c *ui.Context) {
	section(c, locales.SectionLogs(), func() {
		troubles := troubleLines(u.logs)
		ui.Segmented(c, &u.filter, locales.FilterAllCount(len(u.logs)), locales.FilterErrorsCount(len(troubles)))
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.CopyLog()).OnClick(func() {
				mygo.Clipboard.WriteText(u.filteredLogText())
				u.setMsg(msgSuccess, locales.LogCopied())
			})
			ui.Button(c, locales.ClearLog()).OnClick(func() { u.run(func() string { u.app.ClearLogs(); return locales.LogCleared() }) })
			ui.Button(c, locales.ExportLog()).OnClick(func() { u.run(func() string { return msgOr(u.app.SaveLogFile(), locales.LogExported()) }) })
		})
		text := logText(u.logs)
		if u.filter == 1 {
			text = strings.Join(troubles, "\n")
		}
		ui.TextArea(c, &text).Height(440)
	})
}
func (u *UI) infoView(c *ui.Context) {
	section(c, locales.SectionFirewall(), func() {
		ui.Button(c, locales.RefreshState()).OnClick(u.refreshFirewall)
		ui.Text(c, locales.FirewallState(u.firewall))
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.AllowProgram()).Disabled(u.busy).OnClick(func() { u.runFirewall(u.app.ApplyFirewall) })
			ui.Button(c, locales.RemoveAllowance()).Disabled(u.busy).OnClick(func() { u.runFirewall(u.app.RemoveFirewall) })
		})
	})
	section(c, locales.SectionAbout(), func() { ui.Text(c, u.app.About()).Wrap() })
}

// Wait 等界面线程排队的异步任务收尾，避免退出时丢日志。
func (u *UI) Wait() { u.tasks.Wait() }
