package app

import (
	"fmt"
	"net"
	"strings"

	"pxeserv/internal/config"
	"pxeserv/internal/dhcp"
	"pxeserv/internal/locales"
	"pxeserv/internal/netif"
	"pxeserv/internal/pxe"
)

// StartServices 启动 PXE 服务，返回空串表示成功。
func (a *App) StartServices() string {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	a.mu.Lock()
	if a.services != nil {
		a.mu.Unlock()
		return ""
	}
	cfg := a.cfg
	a.mu.Unlock()
	if cfg == nil {
		return locales.EmptyConfig()
	}
	if cfg.TFTPRoot == "" {
		msg := locales.EmptyBootRoot()
		a.Errorf("启动目录：%s", msg)
		return msg
	}

	services, err := pxe.Start(cfg, a.logAt)
	if err != nil {
		msg, title := err.Error(), locales.AppTitle()
		switch err.Stage {
		case pxe.BootRoot:
			msg = locales.BootRootUnusable(cfg.TFTPRoot, err.Err.Error())
			title = locales.BootRootDialogTitle()
		case pxe.DHCP:
			msg = locales.DhcpStartFailed(err.Err.Error())
			title = locales.DhcpFailDialogTitle()
		}
		a.Errorf("%v", err)
		a.dialog(title, msg)
		return msg
	}

	a.mu.Lock()
	a.services = services
	a.mu.Unlock()
	if hint := otherSubnetHint(a.ListInterfaces(), cfg); hint != "" {
		a.Warnf("%s", hint)
	}
	return ""
}

// otherSubnetHint 在启动完成后提醒「本机还有别的网段」。
// 客户端接在别的网卡上时，服务可能收不到它的广播，
// 日志里一个包都不会出现——这是「之前好好的，突然启动不了」最常见的原因。
func otherSubnetHint(list []netif.Iface, cfg *config.Config) string {
	own := net.ParseIP(cfg.ServerIP).To4()
	mask := net.ParseIP(cfg.SubnetMask).To4()
	if own == nil || mask == nil {
		return ""
	}
	var others []string
	for _, it := range list {
		// 没插网线的网卡上不会有客户端，拿它凑「还有别的网段」只会让人白查。
		if !it.Up {
			continue
		}
		ip := net.ParseIP(it.IP).To4()
		m := net.ParseIP(it.Mask).To4()
		if ip == nil || m == nil {
			continue
		}
		if ip.Mask(net.IPMask(m)).Equal(own.Mask(net.IPMask(mask))) {
			continue
		}
		others = append(others, fmt.Sprintf("%s=%s", it.Name, it.IP))
	}
	if len(others) == 0 {
		return ""
	}
	return locales.OtherSubnetHint(strings.Join(others, "、"))
}

func (a *App) StopServices() {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	a.mu.Lock()
	services := a.services
	a.services = nil
	a.mu.Unlock()
	if services != nil {
		services.Stop()
		a.Logf("服务已全部停止")
	}
}

func (a *App) Status() Status {
	a.mu.Lock()
	services := a.services
	st := Status{Leases: []dhcp.Lease{}}
	if services != nil {
		st.Running, st.DHCP, st.TFTP, st.HTTP = true, true, true, services.HTTP != nil
	}
	if a.cfg != nil {
		st.Proxy = a.cfg.ProxyDHCP
		st.ServerIP = a.cfg.ServerIP
	}
	if a.debug != nil {
		st.DebugAddr = a.debug.addr
	}
	a.mu.Unlock()
	// 注意：Snapshots 会取 DHCP 自己的锁，必须在放开 a.mu 之后调用，
	// 否则一旦 DHCP 侧在持锁时回调 Logf（会拿 a.mu）就会死锁。
	if services != nil {
		st.Leases = services.DHCP.Snapshots()
	}
	return st
}

// ReleaseLease 按 MAC 释放一条租约，让地址可以再次分配。
func (a *App) ReleaseLease(mac string) string {
	a.mu.Lock()
	services := a.services
	a.mu.Unlock()
	if services == nil {
		return locales.ServicesNotStarted()
	}
	if !services.DHCP.ReleaseLease(mac) {
		return locales.LeaseNotFound(mac)
	}
	a.Logf("DHCPd：已释放租约 %s", mac)
	return ""
}
