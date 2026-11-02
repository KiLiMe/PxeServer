//go:build windows

package dhcp

import (
	"net"

	"golang.org/x/sys/windows"
)

// enableBroadcast sets SO_BROADCAST so we may send to 255.255.255.255.
// Without it Windows fails the send with WSAEACCES.
func enableBroadcast(c *net.UDPConn) error {
	rc, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	if cerr := rc.Control(func(fd uintptr) {
		serr = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_BROADCAST, 1)
	}); cerr != nil {
		return cerr
	}
	return serr
}
