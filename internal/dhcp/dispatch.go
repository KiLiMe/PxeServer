// 拆分点：报文分发与消息语义（DISCOVER / REQUEST / RELEASE / DECLINE 各自怎么应答）。
// 引用方向：server.go → 本文件 → reply.go（构造应答）与 pool.go（地址判定）。
// 本文件不涉及任何网络写操作，写出全在 reply.go / transport.go。

package dhcp

import (
	"fmt"
	"net"
)

func (s *Server) handle(c *net.UDPConn, req *Packet, from *net.UDPAddr, onBootService bool) {
	mac := req.ClientMAC().String()
	if mac == "" || mac == "00:00:00:00:00:00" {
		return // 畸形或全零 MAC，分配出去只会互相覆盖租约
	}
	arch, _ := req.ClientArch()
	msg := req.MessageType()

	// ProxyDHCP 只服务 PXE 客户端，其他机器的报文一概不理，避免干扰正常 DHCP。
	if s.cfg.ProxyDHCP && !req.IsPXEClient() {
		return
	}

	// 一条报文只留一行日志：收到什么、怎么应答。应答摘要由 dispatch 回填，
	// 同一客户端 5 秒内的相同结论交给 logPacket 合并。
	s.logPacket(mac, TypeName(msg), arch, onBootService, s.dispatch(c, req, from, onBootService))
}

// dispatch 处理一条报文，并把「怎么应答」压成一句摘要（“→ OFFER 192.168.5.2，启动文件 …”）。
func (s *Server) dispatch(c *net.UDPConn, req *Packet, from *net.UDPAddr, onBootService bool) string {
	mac := req.ClientMAC().String()
	msg := req.MessageType()

	switch msg {
	case Discover, Request, Inform:
		if onBootService {
			// 4011 上的 PXE 启动服务：老式 BIOS ROM 会先发 DISCOVER，再发 REQUEST。
			// 只回 REQUEST 的话，先发 DISCOVER 的那批固件收不到 OFFER 就直接放弃了。
			switch msg {
			case Discover:
				return s.reply(c, req, from, Offer)
			default:
				return s.reply(c, req, from, Ack)
			}
		}
		if msg == Discover {
			return s.reply(c, req, from, Offer)
		}
		// 客户端在 option 54 里点名了别的 DHCP 服务器，说明它没选我们，不要插嘴。
		if sid := req.Get(OptServerID); len(sid) == 4 {
			if own := net.ParseIP(s.cfg.ServerIP).To4(); own != nil && !net.IP(sid).Equal(own) {
				return fmt.Sprintf("→ 忽略（客户端选了其他服务器 %s）", net.IP(sid))
			}
		}
		// REQUEST 里点名的地址如果已被别人占用、不在池里或刚被举报冲突，必须回 NAK，
		// 否则客户端会一直拿着一个不能用的地址（典型是换网段后拿旧地址死磕）。
		if ip := req.RequestedIP(); ip != nil && s.requestUnusable(mac, ip) {
			return s.reply(c, req, from, Nak)
		}
		return s.reply(c, req, from, Ack)
	case Release:
		s.release(mac)
		return "→ 已释放"
	case Decline:
		s.dropAddress(req.RequestedIP())
		return "→ 地址冲突，已回收"
	}
	return "→ 已忽略"
}
