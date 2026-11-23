// Package netif 枚举本机的 IPv4 网卡，供界面「网卡」下拉和启动日志提示使用。
// 只读取系统信息，不改动任何网络配置。
package netif

import "net"

// Iface 是界面「网卡」下拉里的一项。
// 直接选网卡就能自动填好 Option 54 / 掩码 / 广播地址，不用手敲。
// Up=false 表示这块网卡当前没有链路（网线未插 / 被禁用）：照样列出来，
// 方便先把地址配好（比如换机器前预填 192.168.5.1），插上网线后直接可用。
type Iface struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Mask      string `json:"mask"`
	Broadcast string `json:"broadcast"`
	MAC       string `json:"mac"`
	Up        bool   `json:"up"`
}

// List 列出所有非环回 IPv4 网卡。
// 以前只返回 FlagUp 的网卡，导致「网线拔了但已经配了 192.168.5.1」的网卡
// 在界面上根本选不到；现在未连接的也返回，用 Up 字段区分。
func List() []Iface {
	out := []Iface{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagLoopback != 0 || ifi.Name == "" {
			continue
		}
		up := linkUp(ifi.Flags)
		mac := ifi.HardwareAddr.String()
		found := false
		if addrs, err := ifi.Addrs(); err == nil {
			for _, ad := range addrs {
				ipnet, ok := ad.(*net.IPNet)
				if !ok {
					continue
				}
				ip4 := ipnet.IP.To4()
				if ip4 == nil || ip4.IsLinkLocalUnicast() {
					continue
				}
				out = append(out, Iface{
					Name:      ifi.Name,
					IP:        ip4.String(),
					Mask:      net.IP(ipnet.Mask).String(),
					Broadcast: Broadcast(ip4, ipnet.Mask).String(),
					MAC:       mac,
					Up:        up,
				})
				found = true
			}
		}
		if found {
			continue
		}
		// 没有可用 IPv4 的网卡也列一条（IP 为空），前端只做「选中/提示」，不覆盖地址。
		// 已禁用又没有 MAC 的虚拟适配器噪声太大，跳过。
		if !up && mac == "" {
			continue
		}
		out = append(out, Iface{Name: ifi.Name, MAC: mac, Up: up})
	}
	return out
}

// linkUp 报告网卡是不是真的「连着线」。
// net.FlagUp 对应 IFF_UP，那是管理状态：网线拔着、carrier=0 的网口只要被
// NetworkManager / systemd-networkd 设成 up，这一位依然为真（本机 enp3s0 就是这样），
// 拿它判断会误报成已连接。有载波时内核会同时置 IFF_RUNNING（net.FlagRunning），
// 两个一起看才对得上用户眼里「插没插网线」。
func linkUp(flags net.Flags) bool {
	return flags&net.FlagUp != 0 && flags&net.FlagRunning != 0
}

// Broadcast 由地址和掩码算广播地址。
func Broadcast(ip net.IP, mask net.IPMask) net.IP {
	v4 := ip.To4()
	if v4 == nil {
		return net.IPv4zero
	}
	m := mask
	if len(m) == 16 {
		m = m[12:]
	}
	out := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		out[i] = v4[i] | ^m[i]
	}
	return out
}
