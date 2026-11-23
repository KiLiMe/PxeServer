//go:build !linux

// 非 Linux 平台的网络实现：绑定、收发都用 net 包的默认行为，不需要 Linux 那套
// IP_PKTINFO 核对入口网卡（见 network_linux.go）。文件名不带 _windows 后缀是因为
// 这里没有 Windows 专有代码，只是把 Linux 排除在外（与其它平台共用同一份实现）。
package dhcp

import (
	"fmt"
	"net"
)

type dhcpNetwork struct{}

func (s *Server) setupNetwork() error { return nil }

// bindService 只能绑到 server_ip 上：Windows 对 255.255.255.255 的广播按路由表选出口
// 网卡，绑 0.0.0.0 时应答常从无关的虚拟网卡发出，源 IP 不匹配会被客户端丢弃
// （见 docs/architecture.md「单网段限制与广播 socket 绑定」）。
//
// 因此这里不退回 0.0.0.0。端口被占用时回退同样绑不上，起不到兜底作用；剩下的失败原因
// 基本只有「本机没有这个地址」（网卡 IP 变了、网卡没就绪、配置被手改），此时退回
// 0.0.0.0 只会跑起一份应答里仍写着 server_ip、实际却绑在别的地址上的服务：客户端按
// Option 54 单播续租、按 Option 66 去取引导文件，全都打不通，表现成「服务在跑但一直
// 卡在 Configuring」，比启动报错难排查得多。地址不对就让用户去「网卡」下拉里重选。
func (s *Server) bindService(port string) (*net.UDPConn, error) {
	addr := net.JoinHostPort(s.cfg.ServerIP, port)
	conn, err := bindUDP(addr)
	if err != nil {
		return nil, fmt.Errorf("无法绑定 %s（请确认「服务器 IP」是本机某块网卡上的地址）：%w", addr, err)
	}
	return conn, nil
}

func (*dhcpNetwork) read(conn *net.UDPConn, buf []byte) (int, *net.UDPAddr, error) {
	return conn.ReadFromUDP(buf)
}

func (*dhcpNetwork) write(conn *net.UDPConn, payload []byte, to *net.UDPAddr) error {
	_, err := conn.WriteToUDP(payload, to)
	return err
}
