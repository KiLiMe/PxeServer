// 拆分点：配置数据结构本身（字段 = 界面控件）与默认值。
// 依赖边界：本包只依赖 internal/locales（中文报错文案），绝不能被 app / ui 反向依赖。
// 按需阅读：新增配置项改这里，再同步 internal/ui 对应页面与 normalize.go 的校验。

package config

import (
	"strconv"
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

// BootFileFor 按客户端架构取启动文件，先看架构表再看全局默认值。
func (c *Config) BootFileFor(arch uint16) string {
	if f, ok := c.BootFileByArch[strconv.Itoa(int(arch))]; ok && f != "" {
		return f
	}
	return c.BootFile
}
