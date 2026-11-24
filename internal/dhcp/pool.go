// 拆分点：地址池（池范围算术、租约表、隔离表、快照）。
// 引用方向：server.go / dispatch.go / reply.go → 本文件，本文件不回调它们。
// 按需阅读：改「分配哪个地址」只看本文件；改「应答里写什么」去 reply.go。

package dhcp

import (
	"net"
	"sort"
	"time"
)

// quarantinePeriod 是地址被客户端声明冲突（DECLINE）后的隔离时长，
// 隔离期内不再分配，避免把已知冲突的地址立刻发给下一台机器。
const quarantinePeriod = 60 * time.Second

type lease struct {
	mac     string
	ip      net.IP
	expires time.Time
}

// requestUnusable 判断客户端在 REQUEST 里点名的地址能不能给它。
func (s *Server) requestUnusable(mac string, ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	if !s.poolContains(ip4) || s.reserved(ip4) {
		return true
	}
	if owner, taken := s.byIP[ip4.String()]; taken && owner != mac {
		return true
	}
	if until, bad := s.quarantine[ip4.String()]; bad && time.Now().Before(until) {
		return true
	}
	return false
}

func (s *Server) baseIP() net.IP {
	ip := net.ParseIP(s.cfg.PoolStart).To4()
	return ip
}

func ipToU32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

func u32ToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).To4()
}

// poolRange 返回地址池的有效主机范围 [startU, endU]（已排除网络号和广播地址）。
func (s *Server) poolRange() (startU, endU uint64, ok bool) {
	base := s.baseIP()
	if base == nil || s.cfg.PoolSize <= 0 {
		return 0, 0, false
	}
	maskIP := net.ParseIP(s.cfg.SubnetMask).To4()
	if maskIP == nil {
		maskIP = net.IPv4(255, 255, 255, 0).To4()
	}
	maskU := ipToU32(maskIP)
	sU := ipToU32(base)
	netU := sU & maskU
	bcastU := netU | ^maskU
	if sU <= netU || sU >= bcastU {
		return 0, 0, false
	}
	eU := uint64(sU) + uint64(s.cfg.PoolSize) - 1
	if eU >= uint64(bcastU) {
		eU = uint64(bcastU) - 1
	}
	return uint64(sU), eU, eU >= uint64(sU)
}

func (s *Server) poolContains(ip net.IP) bool {
	ip4 := ip.To4()
	startU, endU, ok := s.poolRange()
	if !ok || ip4 == nil {
		return false
	}
	v := uint64(ipToU32(ip4))
	if v < startU || v > endU {
		return false
	}
	return !s.reserved(ip4)
}

// reserved 判断该地址是否被服务器自己或网关占用，绝不会分配给客户端。
func (s *Server) reserved(ip net.IP) bool {
	for _, v := range []string{s.cfg.ServerIP, s.cfg.Router} {
		if v == "" {
			continue
		}
		if own := net.ParseIP(v).To4(); own != nil && own.Equal(ip.To4()) {
			return true
		}
	}
	return false
}

// alloc 返回应当分配给该 MAC 的地址；失败说明池子已满。
func (s *Server) alloc(mac string, requested net.IP) (net.IP, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()

	ttl := time.Duration(s.cfg.LeaseTime) * time.Second
	now := time.Now()

	if l, ok := s.leases[mac]; ok && (requested == nil || requested.Equal(l.ip)) {
		l.expires = now.Add(ttl)
		return l.ip, true
	}
	if requested != nil && s.poolContains(requested) {
		owner, taken := s.byIP[requested.String()]
		until, bad := s.quarantine[requested.String()]
		if (!taken || owner == mac) && (!bad || now.After(until)) && !s.reserved(requested) {
			s.bindLocked(mac, requested, now.Add(ttl))
			return requested, true
		}
	}
	if l, ok := s.leases[mac]; ok {
		l.expires = now.Add(ttl)
		return l.ip, true
	}
	startU, endU, ok := s.poolRange()
	if !ok {
		return nil, false
	}
	for v := startU; v <= endU; v++ {
		ip := u32ToIP(uint32(v))
		if s.reserved(ip) {
			continue
		}
		if _, taken := s.byIP[ip.String()]; taken {
			continue
		}
		if until, bad := s.quarantine[ip.String()]; bad && now.Before(until) {
			continue
		}
		s.bindLocked(mac, ip, now.Add(ttl))
		return ip, true
	}
	return nil, false
}

func (s *Server) bindLocked(mac string, ip net.IP, expires time.Time) {
	if old, ok := s.leases[mac]; ok {
		delete(s.byIP, old.ip.String())
	}
	s.leases[mac] = &lease{mac: mac, ip: ip, expires: expires}
	s.byIP[ip.String()] = mac
}

func (s *Server) release(mac string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.leases[mac]; ok {
		delete(s.byIP, l.ip.String())
		delete(s.leases, mac)
	}
}

func (s *Server) dropAddress(ip net.IP) {
	if ip == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	s.quarantine[ip4.String()] = time.Now().Add(quarantinePeriod)
	if mac, ok := s.byIP[ip4.String()]; ok {
		if l, ok2 := s.leases[mac]; ok2 && l.ip.Equal(ip4) {
			delete(s.leases, mac)
		}
		delete(s.byIP, ip4.String())
	}
}

func (s *Server) sweepLocked() {
	now := time.Now()
	for mac, l := range s.leases {
		if now.After(l.expires) {
			delete(s.byIP, l.ip.String())
			delete(s.leases, mac)
		}
	}
}

// Lease 是暴露给界面的一条租约。
type Lease struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	ExpiresAt string `json:"expiresAt"`
	Remaining int    `json:"remainingSec"`
}

func (s *Server) Snapshots() []Lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	out := make([]Lease, 0, len(s.leases))
	now := time.Now()
	for _, l := range s.leases {
		left := int(l.expires.Sub(now).Seconds())
		if left < 0 {
			left = 0
		}
		out = append(out, Lease{
			MAC:       l.mac,
			IP:        l.ip.String(),
			ExpiresAt: l.expires.Format("15:04:05"),
			Remaining: left,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

func (s *Server) ReleaseLease(mac string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leases[mac]
	if !ok {
		return false
	}
	delete(s.byIP, l.ip.String())
	delete(s.leases, mac)
	return true
}
