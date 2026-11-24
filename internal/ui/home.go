// 拆分点：主页布局（状态卡、启动/停止、特权提示条的装配）。
// 引用方向：本文件 → iface.go / privilege.go；页面逻辑只调 app 的导出方法。

package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"pxeserv/internal/locales"
)

// home 是「主页」：服务、网络与启动参数，日常调参都在这一页。
func (u *UI) home(c *ui.Context) {
	// 特权端口那条要放在最上面：普通用户在 Linux 上必然绑不上 67/69，
	// 与其让用户点完「启动」去日志里找 bind: permission denied，不如开机就说清楚。
	if !u.privilegedOK && !u.status.Running {
		switch {
		case u.privilegedGranted:
			// 授权只改了文件，能力要下一次 exec 才进 CapEff：这里直接给「立即重启」，
			// 免得用户看完「重启后生效」还得自己关窗口、再去菜单里重新打开。
			hintBoxActions(c, locales.PrivilegedGrantedHint(), msgSuccess,
				hintAction{locales.PrivilegedRestart(), u.restartApp})
		default:
			// 提示条只留一句结论：三种解法的完整说明在「启动」被拦下的弹框里，
			// 命令给一个「复制命令」按钮，SSH / 没有 polkit 的用户拿去终端贴。
			hintBoxActions(c, locales.PrivilegedPortHint(), msgWarn,
				hintAction{locales.PrivilegedGrant(), u.grantPrivileges},
				hintAction{locales.PrivilegedCopyCmd(), u.copySetcapHint})
		}
	}
	section(c, locales.SectionServices(), func() {
		ui.Checkbox(c, &u.cfg.ProxyDHCP, locales.ProxyDHCP())
		ui.Checkbox(c, &u.cfg.HTTPEnable, locales.EnableHTTP())
		row(c, sf(c, locales.BootRoot(), &u.cfg.TFTPRoot), sf(c, locales.HttpPort(), &u.port))
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.ChooseFolder()).OnClick(func() {
				current := u.cfg.TFTPRoot
				u.async(func() result {
					path := u.app.PickFolder(current)
					if path == "" {
						return notice(locales.FolderUnchanged(), nil)
					}
					return done(func() { u.cfg.TFTPRoot = path })
				})
			})
			ui.Button(c, locales.OpenBootFolder()).OnClick(func() {
				u.async(func() result { return notice(u.app.OpenBootFolder(), nil) })
			})
		})
	})
	section(c, locales.SectionNetwork(), func() {
		opts := []string{u.manualLabel()}
		for _, x := range u.ifaces {
			opts = append(opts, ifaceLabel(x))
		}
		ui.Select(c, &u.iface, opts).FillWidth().OnChange(func() {
			if x := u.findIface(); x != nil {
				u.applyIface(*x)
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.Rescan()).OnClick(func() {
				selected := u.iface
				u.async(func() result {
					ifaces := u.app.ListInterfaces()
					return done(func() {
						u.ifaces = ifaces
						u.iface = selected
						// 原来选中的网卡不在了就清空，让 syncIface 按「服务器 IP」重新对齐。
						if u.findIface() == nil {
							u.iface = ""
							u.syncIface()
						}
					})
				})
			})
			ui.Button(c, locales.DetectIP()).OnClick(u.detectInterface)
			if x := u.findIface(); x != nil {
				ui.Button(c, locales.ApplyIface()).OnClick(func() { u.applyIface(*x) })
			}
		})
		if u.cfg.ProxyDHCP {
			hintBox(c, locales.ProxyPoolHint(), msgWarn)
		}
		row(c, sf(c, locales.ServerIP(), &u.cfg.ServerIP), sf(c, locales.SubnetMask(), &u.cfg.SubnetMask), sf(c, locales.NextServer(), &u.cfg.NextServer))
		ui.Column(c).FillWidth().Gap(10).Disabled(u.cfg.ProxyDHCP).Children(func() {
			row(c, sf(c, locales.PoolStart(), &u.cfg.PoolStart), sf(c, locales.PoolSize(), &u.poolSize), sf(c, locales.LeaseSeconds(), &u.leaseTime))
			row(c, sf(c, locales.Router(), &u.cfg.Router), sf(c, locales.Dns(), &u.cfg.DNS), sf(c, locales.Broadcast(), &u.cfg.Broadcast), sf(c, locales.DomainName(), &u.cfg.DomainName))
		})
	})
	section(c, locales.SectionBootOptions(), func() {
		row(c, sf(c, locales.BootFile(), &u.cfg.BootFile), sf(c, locales.MenuFile(), &u.cfg.MenuFile), sf(c, locales.RootPath(), &u.cfg.RootPath), sf(c, locales.ClassID(), &u.cfg.ClassID))
		ui.Checkbox(c, &u.cfg.FilterUserClass, locales.FilterUserClass())
		ui.Checkbox(c, &u.cfg.Opt43Subopt1, locales.Opt43Subopt1())
		ui.Checkbox(c, &u.cfg.NoOption43, locales.NoOption43())
		ui.Checkbox(c, &u.cfg.AutoStart, locales.AutoStart())
		addr := u.status.DebugAddr
		if addr == "" {
			addr = locales.NotRunning()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.DebugAddr(addr)).Grow(1)
			ui.Button(c, locales.CopyAddr()).Disabled(u.status.DebugAddr == "").OnClick(func() {
				mygo.Clipboard.WriteText(u.status.DebugAddr)
				u.setMsg(msgSuccess, locales.DebugAddrCopied())
			})
		})
		row(c, sf(c, locales.Option66(), &u.cfg.BootServerHost), sf(c, locales.Option67(), &u.cfg.BootFileOption), sf(c, locales.Option252(), &u.cfg.PrivateOption))
		field(c, locales.ExtraOptions(), &u.cfg.ExtraOptions)
	})
}
