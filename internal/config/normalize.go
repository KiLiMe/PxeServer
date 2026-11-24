// 拆分点：配置校验与回落（Normalize：缺项补齐、越界纠正、运行态覆盖）。
// 引用方向：Load / Default 调用它；本文件只读 Config，不读写盘。

package config

import (
	"errors"
	"net"

	"pxeserv/internal/locales"
)

// Normalize 填充自动探测项并校验所有地址。
func (c *Config) Normalize() error {
	detected := DetectIP()
	if c.ServerIP == "" {
		c.ServerIP = detected
	}
	if c.NextServer == "" {
		c.NextServer = c.ServerIP
	}
	if c.BootFileByArch == nil {
		c.BootFileByArch = Default().BootFileByArch
	}
	if c.PoolSize <= 0 {
		c.PoolSize = 1
	}
	if c.PoolSize > 254 {
		return errors.New(locales.ErrPoolSize(c.PoolSize))
	}
	if c.LeaseTime == 0 {
		c.LeaseTime = 3600
	}
	if c.LeaseTime < 60 || c.LeaseTime > 604800 {
		return errors.New(locales.ErrLeaseRange(int(c.LeaseTime)))
	}
	if c.HTTPPort == 0 {
		c.HTTPPort = 80
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return errors.New(locales.ErrHTTPPortRange(c.HTTPPort))
	}
	switch c.HTTPPort {
	case 67, 68, 69, 4011:
		return errors.New(locales.ErrHTTPPortReserved(c.HTTPPort))
	}
	// 单个 DHCP 选项值上限 255 字节，BOOTP 的 file 字段上限 128 字节，超了会被静默截断，
	// 与其让用户在客户端那边猜，不如直接报错。
	for _, f := range []struct{ name, val string }{
		{"option17", c.RootPath},
		{"option252", c.PrivateOption},
	} {
		if len(f.val) > 255 {
			return errors.New(locales.ErrOptionTooLong(f.name, len(f.val)))
		}
	}
	for _, f := range []struct{ name, val string }{
		{"boot_file", c.BootFile},
		{"menu_file", c.MenuFile},
		{"boot_file_option67", c.BootFileOption},
	} {
		if len(f.val) > 127 {
			return errors.New(locales.ErrBootFieldTooLong(f.name, len(f.val)))
		}
	}
	// 按界面从上到底的顺序固定校验次序，报错指向的字段才可预期。
	for _, f := range []struct{ name, val string }{
		{"server_ip", c.ServerIP},
		{"next_server", c.NextServer},
		{"pool_start", c.PoolStart},
		{"subnet_mask", c.SubnetMask},
		{"router", c.Router},
		{"dns", c.DNS},
		{"broadcast", c.Broadcast},
	} {
		if f.val == "" {
			continue
		}
		if net.ParseIP(f.val).To4() == nil {
			return errors.New(locales.ErrNotIPv4(f.name, f.val))
		}
	}
	if _, err := c.ParseExtraOptions(); err != nil {
		return err
	}
	if c.BootServerHost != "" && net.ParseIP(c.BootServerHost).To4() == nil {
		return errors.New(locales.ErrNotIPv4("boot_server_host", c.BootServerHost))
	}
	if c.ServerIP == "" {
		return errors.New(locales.ErrNoServerIP())
	}
	// 地址池必须落在服务器所在子网内，且不能把网络号、广播地址、服务器自己/网关分给客户端。
	if c.PoolStart != "" && c.SubnetMask != "" {
		maskIP := net.ParseIP(c.SubnetMask).To4()
		srv := net.ParseIP(c.ServerIP).To4()
		start := net.ParseIP(c.PoolStart).To4()
		if srv != nil && start != nil && maskIP != nil {
			maskU := ipToU32(maskIP)
			netU := ipToU32(srv) & maskU
			bcastU := netU | ^maskU
			startU := ipToU32(start)
			if (startU & maskU) != netU {
				return errors.New(locales.ErrPoolSubnet(c.PoolStart, c.ServerIP, c.SubnetMask))
			}
			if startU <= netU {
				return errors.New(locales.ErrPoolStartZero(c.PoolStart))
			}
			endU := uint64(startU) + uint64(c.PoolSize) - 1
			if endU >= uint64(bcastU) {
				return errors.New(locales.ErrPoolOverflow(c.PoolStart, c.PoolSize))
			}
		}
		for _, reserved := range []string{c.ServerIP, c.Router} {
			if reserved == "" {
				continue
			}
			if ip := net.ParseIP(reserved).To4(); ip != nil && start != nil && inPool(start, c.PoolSize, ip) {
				return errors.New(locales.ErrPoolReserved(reserved))
			}
		}
	}
	return nil
}
