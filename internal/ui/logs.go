package ui

import (
	"strings"

	"github.com/egoist/mygo"
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/app"
	"pxeserv/internal/locales"
	"pxeserv/internal/logbook"
)

// logText 把日志行拼成可直接复制的文本。
func logText(lines []app.LogLine) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return strings.Join(out, "\n")
}

// troubleLines 只挑出需要人看的行（警告与错误）。
// 级别由写日志的一方标注，不靠中文关键词猜。
func troubleLines(lines []app.LogLine) []string {
	var out []string
	for _, l := range lines {
		if l.Level >= logbook.Warn {
			out = append(out, l.Text)
		}
	}
	return out
}

// filteredLogText 按当前筛选把日志拼成可直接复制的文本。
func (u *UI) filteredLogText(troubles []string) string {
	if u.filter == 1 {
		return strings.Join(troubles, "\n")
	}
	return logText(u.logs)
}

func (u *UI) logView(c *ui.Context) {
	section(c, locales.SectionLogs(), func() {
		troubles := troubleLines(u.logs)
		ui.Segmented(c, &u.filter, locales.FilterAllCount(len(u.logs)), locales.FilterErrorsCount(len(troubles)))
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.CopyLog()).OnClick(func() {
				mygo.Clipboard.WriteText(u.filteredLogText(troubles))
				u.setMsg(msgSuccess, locales.LogCopied())
			})
			ui.Button(c, locales.ClearLog()).OnClick(func() {
				u.async(func() result {
					u.app.ClearLogs()
					return ok(locales.LogCleared(), nil)
				})
			})
			ui.Button(c, locales.ExportLog()).OnClick(func() {
				u.async(func() result {
					if msg := u.app.SaveLogFile(); msg != "" {
						return failed(msg)
					}
					return ok(locales.LogExported(), nil)
				})
			})
		})
		text := u.filteredLogText(troubles)
		ui.TextArea(c, &text).Height(440)
	})
}
