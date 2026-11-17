package app

import (
	"pxeserv/internal/config"
	"pxeserv/internal/firewall"
	"pxeserv/internal/locales"
)

func (a *App) FirewallState() string {
	return firewall.State()
}

// FirewallMovedFrom 返回程序移动前的旧路径；没移动过（或没放行过）返回空串。
// 非管理员读不到规则，但 config.json 的放行标记会指出路径已经变了。
func (a *App) FirewallMovedFrom() string {
	return firewall.MovedFrom(config.DefaultPath(), firewall.ExePath())
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
