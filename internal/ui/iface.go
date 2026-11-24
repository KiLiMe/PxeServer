// 拆分点：网卡下拉与 server_ip 的联动（探测、套用、地址推算）。
// 引用方向：home.go → 本文件 → internal/netif + internal/config 的导出方法。

package ui

import (
	"net"

	"pxeserv/internal/locales"
	"pxeserv/internal/netaddr"
	"pxeserv/internal/netif"
)

// ifaceLabel 是网卡在下拉里的显示名：没有链路的网卡照样列出、照样可选，
// 但在后面标注「未连接」，免得把没插网线的适配器当成能用的那个。
// 没配 IPv4 时不能拼成「名字 ·」（尾巴挂个空分隔符），改标注「无 IPv4」。
func ifaceLabel(i netif.Iface) string {
	label := i.Name
	if i.IP != "" {
		label += " · " + i.IP
	}
	switch {
	case !i.Up:
		label += "（" + locales.IfaceDown() + "）"
	case i.IP == "":
		label += "（" + locales.IfaceNoAddr() + "）"
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
		candidate := netaddr.Add(network.IP, 2)
		if candidate.Equal(ip) {
			candidate = netaddr.Add(network.IP, 3)
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
