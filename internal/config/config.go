package config

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"pxeserv/internal/locales"
)

// Config 与原生界面面板字段一一对应，通过配置持久化与服务启停绑定使用。
type Config struct {
	ProxyDHCP  bool `json:"proxy_dhcp"`  // 只应答 PXE 客户端、不分配地址，可与路由器 DHCP 共存
	HTTPEnable bool `json:"http_enable"` // 同时用 HTTP 提供启动文件（wimboot / iPXE kernel 必需）

	ServerIP   string `json:"server_ip"`   // Option 54；留空则自动探测
	PoolStart  string `json:"pool_start"`  // DHCP 地址池起始
	PoolSize   int    `json:"pool_size"`   // 地址数量
	NextServer string `json:"next_server"` // Option 66 / siaddr；留空则等于 ServerIP
	LeaseTime  uint32 `json:"lease_time"`  // Option 51，秒

	SubnetMask string `json:"subnet_mask"` // Option 1
	Router     string `json:"router"`      // Option 3
	DNS        string `json:"dns"`         // Option 6
	Broadcast  string `json:"broadcast"`   // Option 28
	DomainName string `json:"domain_name"` // Option 15

	BootFile        string            `json:"boot_file"`         // Option 67 默认值
	BootFileByArch  map[string]string `json:"boot_file_by_arch"` // option 93 架构 -> 文件名
	FilterUserClass bool              `json:"filter_user_class"` // 仅当 user-class=gPXE/iPXE 时才下发文件名
	MenuFile        string            `json:"menu_file"`         // iPXE 菜单脚本

	// 高级项：界面上直接排在主页的「启动与 DHCP 选项」里，没有折叠区。
	RootPath       string `json:"root_path"`          // Option 17 根路径
	ClassID        string `json:"class_id"`           // Option 60 ClassID，默认 PXEClient
	BootServerHost string `json:"boot_server_host"`   // Option 66 覆盖值
	BootFileOption string `json:"boot_file_option67"` // Option 67 覆盖值（优先于架构表）
	PrivateOption  string `json:"option252"`          // Option 252，常用于 WPAD

	ExtraOptions string `json:"extra_options"` // 额外 DHCP 选项，格式 code[.sub]=值; ...
	Opt43Subopt1 bool   `json:"opt43_subopt1"` // Option 43 子项 1 = 0.0.0.0，部分品牌网卡需要
	NoOption43   bool   `json:"no_option43"`   // EFI 客户端不下发 Option 43（个别 UEFI 固件见到 43 反而卡住）；BIOS 始终下发

	TFTPRoot string `json:"tftp_root"` // 启动文件根目录，同时也是 HTTP 文档根
	HTTPPort int    `json:"http_port"`

	// AutoStart 打开程序后自动启动服务（否则等用户点“启动”）。
	AutoStart bool `json:"autostart"`

	// Language 是界面语言（BCP 47 标签，如 "zh-Hans"、"en"），留空表示跟随系统。
	Language string `json:"language"`

	// Runtime 是程序自己写的运行态（调试接口地址、防火墙放行标记），
	// 和用户配置存在同一个 config.json 里，但只能由 SetRuntime 改写；
	// Save 时会沿用磁盘上的值，所以界面保存不会把它冲掉。
	Runtime Runtime `json:"runtime"`
}

// Runtime 记录跨进程可见的运行态：提权后的防火墙子进程写它，
// 主进程读它显示“已放行”；调试接口的端口也放这里，脚本读 config.json 即可。
type Runtime struct {
	// DebugAddr 是本次运行中调试接口的地址（如 127.0.0.1:18080），未运行时空。
	DebugAddr string `json:"debug_addr"`
	// FirewallExe 是上次按程序路径放行成功的 exe 路径；非管理员进程据此判断放行状态。
	FirewallExe string `json:"firewall_exe"`
}

// Default 返回首次运行时使用的配置：默认自己发 IP（普通 DHCP）。
// 网段里已经有路由器 DHCP、只想给 PXE 客户端补启动信息时，再勾上 ProxyDHCP。
func Default() *Config {
	return &Config{
		ProxyDHCP:  false,
		HTTPEnable: true,
		PoolStart:  "192.168.5.2",
		PoolSize:   10,
		LeaseTime:  3600,
		SubnetMask: "255.255.255.0",
		Broadcast:  "192.168.5.255",
		// UEFI 一律用 snponly 构建：全驱动版（ipxe.efi）会 DisconnectController()
		// 把固件绑在网卡上的驱动解绑、改用自己的原生驱动，iPXE 调 ExitBootServices()
		// 交出控制权后固件再也无法把网卡恢复原状，实机表现为「文件全下载完、然后卡死」。
		// snponly 只走固件的 EFI_SIMPLE_NETWORK_PROTOCOL，退出时网卡状态不变。
		BootFile: "snponly.efi",
		BootFileByArch: map[string]string{
			"0":  "undionly.kpxe",    // BIOS x86（UNDI，本来就是借固件驱动）
			"6":  "snponly-i386.efi", // UEFI x86
			"7":  "snponly.efi",      // UEFI x64
			"9":  "snponly.efi",      // UEFI x64（另一种编码）
			"10": "snponly-arm64.efi",
			"11": "snponly-arm64.efi",
		},
		FilterUserClass: false,
		MenuFile:        "menu.ipxe",
		ClassID:         "PXEClient",
		TFTPRoot:        "./boot",
		HTTPPort:        80, // 与正版 TinyPXE 一致，浏览器直接 http://<服务器>/ 就能看首页
		AutoStart:       false,
		NoOption43:      true, // 默认不给 EFI 发 Option 43；传统 BIOS 不受此开关影响
	}
}

// Clone 返回配置的深拷贝：调用方可以随意改，不会影响正在使用的配置。
// nil 视为「还没加载配置」，回落到默认配置。
func (c *Config) Clone() *Config {
	if c == nil {
		return Default()
	}
	v := *c
	v.BootFileByArch = make(map[string]string, len(c.BootFileByArch))
	for k, file := range c.BootFileByArch {
		v.BootFileByArch[k] = file
	}
	return &v
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// fileMu 串行化对 config.json 的整文件读写：界面保存（Save）和运行态写入
// （SetRuntime，可能来自提权后的子进程）会同时发生，必须互斥。
var fileMu sync.Mutex

// Save 把配置写到 path，并报告内容是否真的有变化。
// 内容一模一样时跳过写盘：界面每次点「保存配置」都会走到这里，
// 反复刷新 mtime 没有意义，日志里也会多出一串重复的“已保存”。
func (c *Config) Save(path string) (bool, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	// runtime 段归 SetRuntime 所有：沿用磁盘上的运行态，避免保存界面配置时
	// 把“调试接口在跑”“防火墙已放行”这类信息抹掉。
	if st, err := ReadRuntime(path); err == nil {
		c.Runtime = st
	}
	return c.saveLocked(path)
}

func (c *Config) saveLocked(path string) (bool, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return false, err
	}
	b = append(b, '\n')
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return false, nil
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// ReadRuntime 只读 runtime 段，不校验、不补默认值：
// 配置本身有错（比如地址池不在同一子网）时 Load 会报错，
// 但“防火墙放行过没有”这类运行态仍然有效。
func ReadRuntime(path string) (Runtime, error) {
	var out Runtime
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return out, err
	}
	return c.Runtime, nil
}

// SetRuntime 只修改 config.json 里的 runtime 段，其余字段原样保留。
// 文件不存在时按默认配置新建；写失败不影响正在跑的服务（只是运行态没落盘）。
func SetRuntime(path string, fn func(*Runtime)) error {
	fileMu.Lock()
	defer fileMu.Unlock()
	c := Default()
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, c); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	fn(&c.Runtime)
	_, err := c.saveLocked(path)
	return err
}

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

// BootFileFor 按客户端架构取启动文件，先看架构表再看全局默认值。
func (c *Config) BootFileFor(arch uint16) string {
	if f, ok := c.BootFileByArch[strconv.Itoa(int(arch))]; ok && f != "" {
		return f
	}
	return c.BootFile
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
