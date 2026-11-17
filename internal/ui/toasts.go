package ui

import (
	"time"

	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
)

// toastType 把内部消息级别映射成 Toast 的类型，供浮层挑配色与停留时长。
func toastType(kind int) string {
	switch kind {
	case msgSuccess:
		return "success"
	case msgWarn:
		return "warning"
	case msgError:
		return "error"
	}
	return "info"
}

// toastTimeout 让错误常驻到用户关掉，其余按 Toast 默认的 4 秒。
func toastTimeout(kind int) time.Duration {
	if kind == msgError {
		return -1
	}
	return 0
}

// toastLimit 与 mygo 的同屏上限一致：更早的直接丢掉，免得连点攒出一长串。
const toastLimit = 3

// setMsg 排进下一帧的 Toast：错误固定 ID，新的顶掉旧的；同文提示只留一条。
func (u *UI) setMsg(kind int, text string) {
	if text == "" {
		return
	}
	id := text
	if kind == msgError {
		id = "error"
	}
	t := ui.Toast{ID: id, Title: text, Type: toastType(kind), Timeout: toastTimeout(kind)}
	for i := range u.pending {
		if u.pending[i].ID == id {
			u.pending[i] = t
			return
		}
	}
	if len(u.pending) == toastLimit {
		u.pending = u.pending[1:]
	}
	u.pending = append(u.pending, t)
}

// flushToasts 把排队的消息交给 mygo：只在本帧构建时调用，帧外 AddToast 会被丢弃。
func (u *UI) flushToasts(c *ui.Context) {
	for _, t := range u.pending {
		c.AddToast(t)
	}
	u.pending = nil
}

// toastViewport 接管 mygo 的默认 Toast 外观：右下角浮层，按级别配色，
// 每条带关闭按钮；不占布局，所以出现和消失都不会推动选项卡与内容。
func (u *UI) toastViewport(c *ui.Context) {
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(16).AlignItems(ui.End).Gap(8)
		for _, t := range toasts {
			bg, border := toastColors(t.Type)
			toast := ui.ToastBase(c, t)
			toast.Root.Row().Gap(10).Padding(10, 12).Radius(8).Background(bg).Border(1, border).MaxWidth(420)
			toast.Root.Children(func() {
				ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, t.Title).Wrap()
					if t.Description != "" {
						ui.Text(c, t.Description).FontSize(12).Wrap()
					}
				})
				toast.CloseButton().Label(locales.CloseToast()).Children(func() {
					ui.Text(c, "×").FontSize(16)
				})
			})
		}
	})
}

// toastColors 按 Toast 的类型返回底色与边框色，内联提示条与浮层共用。
func toastColors(kind string) (bg, border ui.Color) {
	switch kind {
	case "success":
		return ui.Hex("#f0f9f4"), ui.Hex("#badbcc")
	case "warning":
		return ui.Hex("#fff8e6"), ui.Hex("#ffe58f")
	case "error":
		return ui.Hex("#fdf2f2"), ui.Hex("#f5c2c7")
	}
	return ui.Hex("#f0f6fc"), ui.Hex("#cfe2ff")
}
