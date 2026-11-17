package ui

import (
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
)

func (u *UI) aboutView(c *ui.Context) {
	section(c, locales.SectionAbout(), func() { ui.Text(c, u.app.About()).Wrap() })
}
