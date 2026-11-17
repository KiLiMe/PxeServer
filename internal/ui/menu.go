package ui

import (
	"path/filepath"

	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
)

// currentMenuPath 是磁盘上（已保存配置指向的）菜单路径；
// configuredMenuPath 是草稿配置里的路径。两者不同说明改了路径还没保存。
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
			current := u.currentMenuPath()
			ui.Button(c, locales.LoadFile()).OnClick(func() {
				u.async(func() result {
					path := u.currentMenuPath()
					menu := u.app.ReadMenuFile()
					return notice(locales.MenuLoaded(path), func() {
						u.menu = menu
						u.menuSaved = menu
						u.menuPath = path
						u.menuLoaded = true
					})
				})
			})
			ui.Button(c, locales.OpenEditor()).OnClick(func() {
				u.async(func() result {
					if msg := u.app.OpenMenuFile(); msg != "" {
						return failed(msg)
					}
					return ok(locales.MenuOpened(), nil)
				})
			})
			ui.Button(c, locales.SaveMenu()).Disabled(!u.menuLoaded || u.menuPath != current || u.configuredMenuPath() != current).OnClick(func() {
				menu := u.menu
				u.async(func() result {
					if errMsg := u.app.WriteMenuFile(menu); errMsg != "" {
						return failed(errMsg)
					}
					return ok(locales.MenuSaved(), func() { u.menuSaved = menu })
				})
			})
		})
		if u.menuDirty() {
			hintBox(c, locales.MenuUnsaved(), msgWarn)
		}
		if u.menuLoaded {
			ui.TextArea(c, &u.menu).Height(420)
		} else {
			ui.Text(c, locales.MenuHint()).Wrap()
		}
	})
}
