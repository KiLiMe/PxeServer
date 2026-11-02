//go:build !windows

package dhcp

import (
	"net"
	"syscall"
)

func enableBroadcast(c *net.UDPConn) error {
	rc, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	if cerr := rc.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); cerr != nil {
		return cerr
	}
	return serr
}
