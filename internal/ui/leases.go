package ui

import (
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
)

func (u *UI) leases(c *ui.Context) {
	section(c, locales.SectionLeases(), func() {
		ui.Text(c, locales.LeasesSummary(statusText(u.status), len(u.status.Leases)))
		for _, l := range u.status.Leases {
			ui.Row(c).Gap(14).Padding(5).Children(func() {
				ui.Text(c, l.MAC+" · "+l.IP+" · "+l.ExpiresAt).Grow(1)
				ui.Text(c, locales.Seconds(l.Remaining))
				ui.Button(c, locales.Release()).OnClick(func() {
					u.async(func() result {
						if msg := u.app.ReleaseLease(l.MAC); msg != "" {
							return failed(msg)
						}
						return done(nil)
					})
				})
			})
		}
	})
}
