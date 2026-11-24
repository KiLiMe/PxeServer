// 拆分点：跨页面复用的控件词汇（提示条、分节、表单行、状态文字）。
// 引用方向：各页面文件 → 本文件。本文件不持有状态、不调 app。

package ui

import (
	"github.com/egoist/mygo/ui"
	"pxeserv/internal/app"
	"pxeserv/internal/locales"
)

// hintBox 是页面里的内联提示条（不是浮动通知），配色与 Toast 共用一张表。
func hintBox(c *ui.Context, text string, kind int) {
	hintBoxActions(c, text, kind)
}

// hintAction 是提示条右侧的补救按钮：把文案里的一步做成可点的。
type hintAction struct {
	label string
	on    func()
}

// hintBoxActions 是带补救按钮的提示条（按钮最多两个，多了挤不下）。
// 文案要短：长解释留给弹框与文档，提示条只留一句结论。
func hintBoxActions(c *ui.Context, text string, kind int, actions ...hintAction) {
	bg, border := toastColors(toastType(kind))
	ui.Row(c).FillWidth().Padding(8).Radius(6).Gap(10).AlignItems(ui.Center).Background(bg).Border(1, border).Children(func() {
		ui.Text(c, text).Wrap().Grow(1)
		for _, a := range actions {
			ui.Button(c, a.label).OnClick(a.on)
		}
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
