// 拆分点：与本机网络相关的纯计算（地址位运算、地址池包含判断、网卡 IP 探测）。
// 引用方向：normalize.go 与 app 层调用；本文件无副作用，除 DetectIP 外可纯测。

package config

import (
	"encoding/binary"
	"net"
	"time"
)

func ipToU32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

// inPool 判断 ip 是否落在 start 开始、count 个地址的范围里。
func inPool(start net.IP, count int, ip net.IP) bool {
	if start == nil || ip == nil || count <= 0 {
		return false
	}
	s := uint64(ipToU32(start))
	v := uint64(ipToU32(ip))
	return v >= s && v <= s+uint64(count)-1
}

// DetectIP 返回默认路由所在网卡的 IPv4 地址（用 UDP 连接探测，不实际发包）。
func DetectIP() string {
	d := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := d.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if a, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.To4().String()
	}
	return ""
}
