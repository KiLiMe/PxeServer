// 拆分点：与本机网络相关的探测（默认路由网卡的 IPv4）。地址位运算与区间判断
// 已收进 internal/netaddr，配置与地址池共用一份。
// 引用方向：app 层与 normalize.go 调用；除 DetectIP 外无副作用。

package config

import (
	"net"
	"time"
)

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
