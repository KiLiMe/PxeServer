//go:build !linux

package dhcp

import (
	"net"

	"pxeserv/internal/logbook"
)

type dhcpNetwork struct{}

func (s *Server) setupNetwork() error { return nil }

func (s *Server) bindService(port string) (*net.UDPConn, error) {
	conn, err := bindUDP(net.JoinHostPort(s.cfg.ServerIP, port))
	if err == nil {
		return conn, nil
	}
	conn, err = bindUDP(":" + port)
	if err == nil && port == "67" {
		s.logf(logbook.Warn, "DHCPd：无法绑定 %s，已退回监听 0.0.0.0:67；广播应答可能从错误网卡发出", s.cfg.ServerIP)
	}
	return conn, err
}

func (*dhcpNetwork) read(conn *net.UDPConn, buf []byte) (int, *net.UDPAddr, error) {
	return conn.ReadFromUDP(buf)
}

func (*dhcpNetwork) write(conn *net.UDPConn, payload []byte, to *net.UDPAddr) error {
	_, err := conn.WriteToUDP(payload, to)
	return err
}
