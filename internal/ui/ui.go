// 拆分点：界面外壳（状态、选项卡顺序、异步调度入口、顶层视图装配）。
// 引用方向：本文件 → action.go（提交结果）与 widget.go（通用控件），单向；只调 app 的导出方法。
// 按需阅读：加一个页面就改这里的分发 + 新建一个页面文件。

package ui

import (
	"strconv"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"pxeserv/internal/app"
	"pxeserv/internal/config"
	"pxeserv/internal/locales"
	"pxeserv/internal/netif"
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
	firewallSupported         bool // 当前平台有没有「按程序放行」这件事（只有 Windows 有）
	privilegedOK              bool // 当前进程能不能绑定特权端口（只有 Linux 会为假）
	privilegedGranted         bool // 本次运行里刚授权过：文件已有能力，但进程 CapEff 要重启才变
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

func (u *UI) Load() {
	u.adoptConfig()
	u.ifaces = u.app.ListInterfaces()
	u.iface = u.manualLabel()
	u.syncIface()
	u.firewall = locales.FirewallPending()
	u.firewallSupported = u.app.FirewallSupported()
	u.privilegedOK = u.app.CanBindPrivilegedPorts()
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
		// 只有 Windows 有放行状态可查：进设置页时偷一次（不弹提示），其它平台不必要。
		if u.tab == tabSettings && !u.firewallQueried && !u.busy && u.firewallSupported {
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

// Wait 等界面线程排队的异步任务收尾，避免退出时丢日志。
func (u *UI) Wait() { u.tasks.Wait() }
