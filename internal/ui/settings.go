package ui

import (
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
)

// appSettings 是「设置」页：程序自身的选项（界面语言）与防火墙放行，
// 服务/网络参数留在主页，两边职责不要混。
func (u *UI) appSettings(c *ui.Context) {
	section(c, locales.SectionLanguage(), func() {
		ui.Select(c, &u.lang, []string{locales.LanguageSystem(), locales.LanguageChinese(), locales.LanguageEnglish()}).FillWidth().OnChange(func() { u.setLanguage(u.lang) })
	})
	section(c, locales.SectionFirewall(), func() {
		ui.Button(c, locales.RefreshState()).Disabled(u.busy).OnClick(func() { u.firewallTask(true, nil) })
		ui.Text(c, locales.FirewallState(u.firewall))
		if u.firewallMoved != "" {
			ui.Text(c, locales.FirewallMoved(u.firewallMoved)).FontSize(12).TextColor(ui.Hex("#b54708"))
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.AllowProgram()).Disabled(u.busy).OnClick(func() { u.firewallTask(true, u.app.ApplyFirewall) })
			ui.Button(c, locales.RemoveAllowance()).Disabled(u.busy).OnClick(func() { u.firewallTask(true, u.app.RemoveFirewall) })
		})
	})
}

// firewallTask 统一「刷新状态」与「放行/撤销」两条路径：都在后台执行，
// 回到界面线程后再把最新状态写进状态行。action 为 nil 表示只刷新；
// notify 为假时连提示都不弹（进设置页时的静默刷新）。
func (u *UI) firewallTask(notify bool, action func() string) {
	if u.busy {
		return
	}
	u.firewallQueried = true
	u.async(func() result {
		apply := u.firewallApply()
		if action != nil {
			if msg := action(); msg != "" {
				return result{kind: msgError, text: msg, apply: apply}
			}
			return done(apply)
		}
		if !notify {
			return silent(apply)
		}
		return ok(locales.FirewallRefreshed(), apply)
	})
}

// firewallApply 把最新放行状态与「程序是否移动过」写进界面，供刷新与放行/撤销共用。
func (u *UI) firewallApply() func() {
	label := firewallLabel(u.app.FirewallState())
	moved := u.app.FirewallMovedFrom()
	return func() { u.firewall = label; u.firewallMoved = moved }
}

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
