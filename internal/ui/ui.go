package ui

import (
	"encoding/json"
	"strconv"
	"sync"

	"github.com/egoist/mygo"
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/app"
	"pxeserv/internal/config"
	"pxeserv/internal/locales"
	"pxeserv/internal/netif"
)

const (
	msgInfo = iota
	msgSuccess
	msgWarn
	msgError
)

// 选项卡顺序与视图里 Segmented 的参数一一对应；调整顺序时两处一起改。
const (
	tabHome = iota
	tabLeases
	tabMenu
	tabArch
	tabLogs
	tabSettings
	tabAbout
)

type UI struct {
	savedKey                  string
	tasks                     sync.WaitGroup
	app                       *app.App
	Win                       *mygo.Window
	cfg                       config.Config
	status                    app.Status
	ifaces                    []netif.Iface
	iface                     string
	tab                       int
	busy                      bool
	loaded                    bool
	firewall                  string
	firewallMoved             string // 程序移动前的旧路径；空串表示没移动过
	firewallQueried           bool
	port, poolSize, leaseTime string
	menu, archKey, archValue  string
	pending                   []ui.Toast
	lang                      string
	menuLoaded                bool
	menuPath, menuSaved       string
	filter                    int
	logs                      []app.LogLine
	logSeq                    uint64
}

func New(a *app.App) *UI {
	u := &UI{app: a, logs: a.Logs()}
	u.adoptConfig()
	return u
}

// adoptConfig 用 App 的配置副本刷新草稿，并把三个数字输入框对齐到配置值。
func (u *UI) adoptConfig() {
	cfg := u.app.GetConfig()
	if cfg == nil {
		cfg = config.Default()
	}
	u.cfg = *cfg
	u.port = strconv.Itoa(u.cfg.HTTPPort)
	u.poolSize = strconv.Itoa(u.cfg.PoolSize)
	u.leaseTime = strconv.FormatUint(uint64(u.cfg.LeaseTime), 10)
}

// result 是一次异步任务的产物：级别、提示文案，以及回到界面线程后的收尾动作。
// silent 表示只做收尾、不弹提示（例如进入设置页时的防火墙静默刷新）。
type result struct {
	kind   int
	text   string
	silent bool
	apply  func()
}

func ok(text string, apply func()) result     { return result{kind: msgSuccess, text: text, apply: apply} }
func notice(text string, apply func()) result { return result{kind: msgInfo, text: text, apply: apply} }
func failed(text string) result               { return result{kind: msgError, text: text} }
func done(apply func()) result                { return ok(locales.Done(), apply) }
func silent(apply func()) result              { return result{silent: true, apply: apply} }

func (u *UI) async(fn func() result) {
	if u.busy {
		return
	}
	u.busy = true
	u.tasks.Add(1)
	u.Win.Invalidate()
	go func() {
		defer u.tasks.Done()
		res := fn()
		u.Win.Update(func() {
			u.busy = false
			if !res.silent {
				u.setMsg(res.kind, res.text)
			}
			if res.apply != nil {
				res.apply()
			}
			u.Win.Invalidate()
		})
	}()
}

func (u *UI) Load() {
	u.adoptConfig()
	u.ifaces = u.app.ListInterfaces()
	u.iface = u.manualLabel()
	u.syncIface()
	u.firewall = locales.FirewallPending()
	u.lang = langLabel(u.cfg.Language)
	u.savedKey = u.draftKey()
	u.loaded = true
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

// appTheme 是界面统一主题：浅灰底、白色卡片、Windows 蓝强调色。
func appTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f3f5f7")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#f0f4f8")
	t.Border = ui.Hex("#dfe3e8")
	t.Accent = ui.Hex("#0067c0")
	t.Font = "Segoe UI Variable Text, Segoe UI, Microsoft YaHei"
	t.FontSize = 13
	t.Radius = 8
	return t
}

func (u *UI) View(c *ui.Context) {
	c.SetTheme(appTheme())
	u.flushToasts(c)
	ui.Column(c).Fill().Padding(18).Gap(12).Disabled(!u.loaded || u.busy).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AppTitle()).FontSize(20).Bold().Grow(1)
			ui.Text(c, statusText(u.status))
			if u.loaded && u.dirty() {
				ui.Text(c, "● "+locales.UnsavedBadge()).FontSize(12).TextColor(ui.Hex("#b54708"))
			}
			ui.Button(c, locales.Start()).Disabled(u.busy || u.status.Running).OnClick(func() { u.saveAndStart(true) })
			ui.Button(c, locales.Stop()).Disabled(u.busy || !u.status.Running).OnClick(func() {
				u.async(func() result {
					u.app.StopServices()
					return ok(locales.ServicesStopped(), nil)
				})
			})
			ui.Button(c, locales.SaveConfig()).Disabled(u.busy).OnClick(func() { u.saveAndStart(false) })
		})
		ui.Segmented(c, &u.tab, locales.TabHome(), locales.TabLeases(), locales.TabBootMenu(), locales.TabArch(), locales.TabLogs(), locales.TabSettings(), locales.TabAbout()).FillWidth()
		if u.tab == tabSettings && !u.firewallQueried && !u.busy {
			u.firewallTask(false, nil)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(12).Padding(4).Children(func() {
				switch u.tab {
				case tabHome:
					u.home(c)
				case tabLeases:
					u.leases(c)
				case tabMenu:
					u.menuView(c)
				case tabArch:
					u.archView(c)
				case tabLogs:
					u.logView(c)
				case tabSettings:
					u.appSettings(c)
				case tabAbout:
					u.aboutView(c)
				}
			})
		})
	})
	u.toastViewport(c)
}

// hintBox 是页面里的内联提示条（不是浮动通知），配色与 Toast 共用一张表。
func hintBox(c *ui.Context, text string, kind int) {
	bg, border := toastColors(toastType(kind))
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
func row(c *ui.Context, fs ...func()) {
	ui.Row(c).Gap(10).Children(func() {
		for _, f := range fs {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(f)
		}
	})
}
func sf(c *ui.Context, l string, p *string) func() { return func() { field(c, l, p) } }

func (u *UI) draftKey() string {
	b, _ := json.Marshal(struct {
		Config            config.Config
		Port, Pool, Lease string
	}{u.cfg, u.port, u.poolSize, u.leaseTime})
	return string(b)
}
func (u *UI) dirty() bool { return u.savedKey != u.draftKey() }

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
	draft := u.cfg.Clone()
	u.async(func() result {
		if saveMsg := u.app.SaveConfig(*draft); saveMsg != "" {
			return failed(saveMsg)
		}
		apply := func() {
			u.adoptConfig()
			u.savedKey = u.draftKey()
			if !u.menuDirty() {
				u.menuLoaded = false
				u.menuPath = ""
				u.menuSaved = ""
			}
			u.ifaces = u.app.ListInterfaces()
			u.syncIface()
			u.Refresh()
		}
		if !start {
			return ok(locales.ConfigSavedApplied(), apply)
		}
		if startMsg := u.app.StartServices(); startMsg != "" {
			return result{kind: msgError, text: locales.SavedNotStarted(startMsg), apply: apply}
		}
		return ok(locales.SavedStarted(), apply)
	})
}

// Wait 等界面线程排队的异步任务收尾，避免退出时丢日志。
func (u *UI) Wait() { u.tasks.Wait() }
