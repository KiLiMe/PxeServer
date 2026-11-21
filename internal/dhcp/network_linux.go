package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

type dhcpNetwork struct {
	ifIndex uint32
	reply   []byte
}

func (s *Server) setupNetwork() error {
	ip := net.ParseIP(s.cfg.ServerIP).To4()
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if subnet, ok := addr.(*net.IPNet); ok && ip.Equal(subnet.IP) {
				s.network.ifIndex = uint32(iface.Index)
				info := unix.Inet4Pktinfo{Ifindex: int32(iface.Index)}
				copy(info.Spec_dst[:], ip)
				s.network.reply = unix.PktInfo4(&info)
				return nil
			}
		}
	}
	return fmt.Errorf("本机网卡未配置 DHCP 服务器地址 %s", s.cfg.ServerIP)
}

func (s *Server) bindService(port string) (*net.UDPConn, error) {
	// Linux 把目的地址为广播的报文交给通配监听，由 IP_PKTINFO 提供实际入口网卡。
	conn, err := bindUDP(":" + port)
	if err != nil {
		return nil, err
	}
	raw, err := conn.SyscallConn()
	if err == nil {
		var sockErr error
		if err = raw.Control(func(fd uintptr) {
			sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_PKTINFO, 1)
		}); err == nil {
			err = sockErr
		}
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (n *dhcpNetwork) read(conn *net.UDPConn, buf []byte) (int, *net.UDPAddr, error) {
	var control [64]byte
	for {
		count, controlLen, flags, from, err := conn.ReadMsgUDP(buf, control[:])
		if err != nil {
			return 0, nil, err
		}
		if flags&(unix.MSG_CTRUNC|unix.MSG_TRUNC) != 0 {
			continue
		}
		if packetInterface(control[:controlLen]) == n.ifIndex {
			return count, from, nil
		}
	}
}

// packetInterface 读取 struct in_pktinfo 的首字段 ipi_ifindex。
func packetInterface(control []byte) uint32 {
	for len(control) != 0 {
		header, data, rest, err := unix.ParseOneSocketControlMessage(control)
		if err != nil {
			return 0
		}
		if header.Level == unix.IPPROTO_IP && header.Type == unix.IP_PKTINFO && len(data) >= unix.SizeofInet4Pktinfo {
			return binary.NativeEndian.Uint32(data)
		}
		control = rest
	}
	return 0
}

func (n *dhcpNetwork) write(conn *net.UDPConn, payload []byte, to *net.UDPAddr) error {
	if n.reply == nil {
		_, err := conn.WriteToUDP(payload, to)
		return err
	}
	_, _, err := conn.WriteMsgUDP(payload, n.reply, to)
	return err
}
