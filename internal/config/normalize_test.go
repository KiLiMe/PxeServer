package config

import (
	"strings"
	"testing"
)

func baseCfg() Config {
	c := *Default()
	c.ServerIP = "192.168.5.1"
	c.NextServer = "192.168.5.1"
	c.PoolStart = "192.168.5.2"
	c.PoolSize = 10
	c.SubnetMask = "255.255.255.0"
	c.Router = "192.168.5.1"
	c.Broadcast = "192.168.5.255"
	c.ProxyDHCP = false
	c.HTTPPort = 18080
	c.LeaseTime = 3600
	return c
}

// 这些校验都是为了拦住“配置保存成功、但客户端就是启不来”的隐性错误。
func TestNormalizeRejectsBadPool(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
		want string
	}{
		{"池越过 254", func(c *Config) { c.PoolStart = "192.168.5.250"; c.PoolSize = 10 }, "越过"},
		{"/25 池越过广播地址", func(c *Config) {
			c.SubnetMask = "255.255.255.128"
			c.Broadcast = "192.168.5.127"
			c.PoolStart = "192.168.5.120"
			c.PoolSize = 10
		}, "越过"},
		{"池包含本机", func(c *Config) { c.PoolStart = "192.168.5.1" }, "包含了"},
		{"池大小超限", func(c *Config) { c.PoolSize = 300 }, "超过一个 C 段"},
		{"池不在同网段", func(c *Config) { c.PoolStart = "192.168.6.2" }, "不在同一子网"},
		{"起始为 0", func(c *Config) { c.PoolStart = "192.168.5.0" }, "必须 >= 1"},
		{"HTTP 端口占 TFTP", func(c *Config) { c.HTTPPort = 69 }, "不能占用"},
		{"端口越界", func(c *Config) { c.HTTPPort = 99999 }, "超出范围"},
		{"租约太短", func(c *Config) { c.LeaseTime = 10 }, "租约时间"},
	}
	for _, tc := range cases {
		c := baseCfg()
		tc.mut(&c)
		err := c.Normalize()
		if err == nil {
			t.Fatalf("%s：期望报错，实际通过", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s：错误信息 %q 不含 %q", tc.name, err.Error(), tc.want)
		}
	}
}

func TestNormalizeAcceptsSanePool(t *testing.T) {
	c := baseCfg()
	if err := c.Normalize(); err != nil {
		t.Fatalf("正常配置被拒: %v", err)
	}
	// /25 合法地址池（192.168.5.2 ~ 192.168.5.126）应通过
	c.SubnetMask = "255.255.255.128"
	c.Broadcast = "192.168.5.127"
	c.PoolStart = "192.168.5.120"
	c.PoolSize = 7
	if err := c.Normalize(); err != nil {
		t.Fatalf("合法 /25 配置被拒: %v", err)
	}
}
