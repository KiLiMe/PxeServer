// 拆分点：UDP 收发的底层动作（绑端口、单播/广播目的地、启动摘要一行）。
// 引用方向：server.go 与 reply.go → 本文件。本文件是唯一调用 s.network.write 的地方。
// 按需阅读：应答发错网卡、广播收不到，只看这里 + network_linux.go。

package dhcp

import (
	"fmt"
	"net"
	"strings"

	"pxeserv/internal/logbook"
)

// bindUDP 绑定一个 UDP 套接字并打开 SO_BROADCAST。
// 不要加 SO_REUSEADDR：Windows 上它会让第二个进程“静默劫持”已经在用的端口，
// 用户就看不到「udp:67 已被占用」的报错了。实测 0.0.0.0:67 与 192.168.5.1:67
// 在不加 REUSEADDR 的情况下本来就可以并存，而且两个 socket 都能收到广播。
func bindUDP(addr string) (*net.UDPConn, error) {
	a, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp4", a)
	if err != nil {
		return nil, err
	}
	if err := enableBroadcast(c); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// broadcastDest 计算广播应答的目的地址。
//
// 用 server_ip + 子网掩码算出的子网广播（如 192.168.5.255）比 255.255.255.255 更可靠：
// Windows 对有限广播 255.255.255.255 只有一个模糊路由，可能从错误的网卡发出去；
// 子网广播则能用明确的网段路由，从配置的那块网卡发出。
func (s *Server) broadcastDest() *net.UDPAddr {
	if ip := net.ParseIP(s.cfg.ServerIP).To4(); ip != nil {
		if m := net.ParseIP(s.cfg.SubnetMask).To4(); m != nil {
			b := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				b[i] = ip[i] | ^m[i]
			}
			return &net.UDPAddr{IP: b, Port: 68}
		}
	}
	if ip := net.ParseIP(s.cfg.Broadcast).To4(); ip != nil && !ip.Equal(net.IPv4zero) {
		return &net.UDPAddr{IP: ip, Port: 68}
	}
	return &net.UDPAddr{IP: net.IPv4(255, 255, 255, 255), Port: 68}
}

// startupSummary 生成本次监听的启动摘要行。监听地址、模式、地址池、广播目标、4011 是否就绪
// 合成一行，以免每次启停都刷六行固定台词。
func startupSummary(addr net.Addr, mode, poolStart string, poolSize int, dests []string, pxeBound bool) string {
	pxe := "PXE 4011 未绑定"
	if pxeBound {
		pxe = "PXE 4011"
	}
	return fmt.Sprintf("DHCPd：已在 %s 上监听（%s，地址池 %s×%d，广播应答 → %s，%s）",
		addr, mode, poolStart, poolSize, strings.Join(dests, "、"), pxe)
}

// broadcastDests 返回广播应答要去的全部地址：配置算出来的广播 + 有限广播 255.255.255.255。
func (s *Server) broadcastDests() []*net.UDPAddr {
	first := s.broadcastDest()
	limited := &net.UDPAddr{IP: net.IPv4(255, 255, 255, 255), Port: 68}
	if first.IP.Equal(limited.IP) {
		return []*net.UDPAddr{first}
	}
	return []*net.UDPAddr{first, limited}
}

func (s *Server) send(c *net.UDPConn, r *Packet, from *net.UDPAddr) {
	var dst *net.UDPAddr
	switch {
	case r.GIAddr != [4]byte{}:
		// 经过中继代理：回给中继的 67 端口。
		dst = &net.UDPAddr{IP: net.IP(r.GIAddr[:]), Port: 67}
	case c == s.bootConn && from != nil && r.Flags&0x8000 == 0:
		// 4011 上的 PXE 启动服务请求：客户端没要求广播时单播回请求方（它不在 68 端口等）。
		// 但 BIOS 的 PXE ROM 在阶段 2 往往还没把 IP 协议栈跑起来，收不到单播 IP 包，
		// 它会置广播标志位（flags & 0x8000），此时必须走广播。
		dst = from
	default:
		// 广播应答：子网广播（192.168.5.255）与有限广播（255.255.255.255）各发一份。
		// 不同 UEFI 固件只认其中一种 —— 有的最小实现只接受 255.255.255.255，
		// 有的只接受本网段广播。两份内容完全一样，客户端会忽略重复的那份。
		payload := r.Marshal()
		ok := false
		for _, d := range s.broadcastDests() {
			if err := s.network.write(c, payload, d); err == nil {
				ok = true
			} else {
				s.logf(logbook.Error, "DHCPd：向 %s 发送应答失败：%v", d.IP, err)
			}
		}
		if !ok {
			s.logf(logbook.Warn, "DHCPd：广播应答发送失败")
		}
		return
	}
	if err := s.network.write(c, r.Marshal(), dst); err != nil {
		s.logf(logbook.Error, "DHCPd：发送应答失败: %v", err)
	}
}
