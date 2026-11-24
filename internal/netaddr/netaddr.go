// Package netaddr 提供 IPv4 的 32 位整数表示与地址区间判断。
//
// 配置校验（internal/config）与地址池（internal/dhcp）都要做同一类位运算，
// 各自实现一份必然在掩码、非 IPv4 输入这类边角上漂移。
package netaddr

import (
	"encoding/binary"
	"net"
)

// ToU32 把 IPv4 地址转成网络序整数；非 IPv4（含 nil）返回 0。
func ToU32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

// FromU32 把网络序整数还原成 4 字节 IPv4 地址。
func FromU32(v uint32) net.IP {
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, v)
	return ip
}

// InRange 判断 ip 是否落在 start 起、连续 count 个地址的闭区间内。
func InRange(start net.IP, count int, ip net.IP) bool {
	if start == nil || ip == nil || count <= 0 {
		return false
	}
	s := uint64(ToU32(start))
	v := uint64(ToU32(ip))
	return v >= s && v <= s+uint64(count)-1
}

// Add 把 IPv4 地址往后数 n 个（按 32 位整数进位，溢出回绕）。
func Add(ip net.IP, n int) net.IP {
	return FromU32(ToU32(ip) + uint32(n))
}
