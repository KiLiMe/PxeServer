package ui

import (
	"encoding/binary"
	"net"

	"github.com/egoist/mygo"
	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
	"pxeserv/internal/netif"
)

// home 是「主页」：服务、网络与启动参数，日常调参都在这一页。
func (u *UI) home(c *ui.Context) {
	// 特权端口那条要放在最上面：普通用户在 Linux 上必然绑不上 67/69，
	// 与其让用户点完「启动」去日志里找 bind: permission denied，不如开机就说清楚。
	if !u.privilegedOK && !u.status.Running {
		switch {
		case u.privilegedGranted:
			hintBox(c, locales.PrivilegedGrantedHint(), msgSuccess)
		default:
			hintBoxButton(c, locales.PrivilegedPortHint(u.setcapHint()), msgWarn, locales.PrivilegedGrant(), u.grantPrivileges)
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

// grantPrivileges 走 pkexec 让用户在一次系统认证里完成授权：setcap 打在当前可执行文件上，
// 之后普通用户启动就能绑 67/69。当前进程的 CapEff 不会因此改变，所以只把提示切成
// 「重启后生效」，不动 privilegedOK——否则下一句「启动」就会撞回 bind 失败。
func (u *UI) grantPrivileges() {
	u.async(func() result {
		if msg := u.app.GrantPrivilegedPorts(); msg != "" {
			return failed(msg)
		}
		return notice(locales.PrivilegedGranted(), func() { u.privilegedGranted = true })
	})
}

// ifaceLabel 是网卡在下拉里的显示名：没有链路的网卡照样列出、照样可选，
// 但在后面标注「未连接」，免得把没插网线的适配器当成能用的那个。
func ifaceLabel(i netif.Iface) string {
	label := i.Name + " · " + i.IP
	if !i.Up {
		label += "（" + locales.IfaceDown() + "）"
	}
	return label
}

func (u *UI) findIface() *netif.Iface {
	for j := range u.ifaces {
		if ifaceLabel(u.ifaces[j]) == u.iface {
			return &u.ifaces[j]
		}
	}
	return nil
}

// syncIface 让下拉与「服务器 IP」保持一致：选了真实网卡就跟着走，否则回到「手动填写」。
func (u *UI) syncIface() {
	if u.iface != "" && u.iface != u.manualLabel() {
		return
	}
	u.iface = u.manualLabel()
	for _, i := range u.ifaces {
		if i.IP == u.cfg.ServerIP {
			u.iface = ifaceLabel(i)
			return
		}
	}
}

func (u *UI) detectInterface() {
	u.async(func() result {
		ip := u.app.DetectIP()
		ifaces := u.app.ListInterfaces()
		return done(func() {
			selected := u.iface
			u.ifaces = ifaces
			for _, i := range u.ifaces {
				if i.IP == ip {
					u.applyIface(i)
					return
				}
			}
			u.iface = selected
			u.setMsg(msgError, locales.IfaceNotFound())
		})
	})
}

func (u *UI) applyIface(i netif.Iface) {
	ip := net.ParseIP(i.IP).To4()
	maskIP := net.ParseIP(i.Mask).To4()
	if ip == nil || maskIP == nil {
		u.setMsg(msgError, locales.IfaceNoIPv4())
		u.syncIface()
		return
	}
	old := u.cfg.ServerIP
	u.cfg.ServerIP = i.IP
	u.cfg.SubnetMask = i.Mask
	u.cfg.Broadcast = i.Broadcast
	if u.cfg.NextServer == "" || u.cfg.NextServer == old {
		u.cfg.NextServer = i.IP
	}
	if u.cfg.Router == old {
		u.cfg.Router = i.IP
	}
	if u.cfg.DNS == old {
		u.cfg.DNS = i.IP
	}
	network := net.IPNet{IP: ip.Mask(net.IPMask(maskIP)), Mask: net.IPMask(maskIP)}
	pool := net.ParseIP(u.cfg.PoolStart)
	if pool == nil || !network.Contains(pool) {
		// 地址池默认从网关地址往后的第一个地址开始；撞上服务器自己就再挪一位。
		candidate := ipAdd(network.IP, 2)
		if candidate.Equal(ip) {
			candidate = ipAdd(network.IP, 3)
		}
		if network.Contains(candidate) && candidate.String() != i.Broadcast {
			u.cfg.PoolStart = candidate.String()
		}
	}
	u.syncIface()
	u.setMsg(msgSuccess, locales.IfaceApplied(i.Name))
}

// manualLabel 是「手动填写」在界面语言里的写法：网卡下拉没有选中真实网卡时的取值。
func (u *UI) manualLabel() string { return locales.ManualEntry() }

// ipAdd 把 IPv4 地址往后数 n 个（按 32 位整数进位，溢出回绕）。
func ipAdd(ip net.IP, n int) net.IP {
	out := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(out, binary.BigEndian.Uint32(ip.To4())+uint32(n))
	return out
}

// setcapHint 返回给用户直接抄的一条命令：路径取当前可执行文件，
// 取不到时写成占位，用户至少知道该填什么。
func (u *UI) setcapHint() string {
	exe := u.app.ExecutablePath()
	if exe == "" {
		exe = "<pxeserver 的路径>"
	}
	return "sudo setcap 'cap_net_bind_service=+ep' " + exe
}
