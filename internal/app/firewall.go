package app

import (
	"pxeserv/internal/firewall"
	"pxeserv/internal/locales"
)

func (a *App) FirewallState() string {
	return firewall.State()
}

// ApplyFirewall 按“程序路径”放行入站（不按端口）。返回空串表示成功。
// 当前进程不是管理员时会弹 UAC，由提权后的自己完成操作。

func (a *App) ApplyFirewall() string {
	if err := firewall.Apply(); err != nil {
		msg := locales.AllowFirewallFailed(err.Error())
		a.Errorf("%s", msg)
		return msg
	}
	a.Logf("防火墙：已按程序路径放行 %s（入站，全部协议）", firewall.ExePath())
	return ""
}

// RemoveFirewall 撤销本程序添加的防火墙规则。

func (a *App) RemoveFirewall() string {
	if err := firewall.Remove(); err != nil {
		msg := locales.RemoveFirewallFailed(err.Error())
		a.Errorf("%s", msg)
		return msg
	}
	a.Logf("防火墙：已撤销本程序的放行规则")
	return ""
}

// ListInterfaces 列出所有非环回 IPv4 网卡（界面「网卡」下拉的数据源）。
