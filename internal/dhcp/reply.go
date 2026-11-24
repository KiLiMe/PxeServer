// 拆分点：应答构造（Option 填写、Option 43 厂商扩展、NAK、额外选项覆盖）。
// 引用方向：dispatch.go → 本文件 → transport.go（发送）。本文件只造包、不发包。
// 按需阅读：客户端行为古怪（PXE-E74、iPXE 循环重载）先看这里的 BIOS/EFI 分支。

package dhcp

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// 本文件里用到但 packet.go 未定义的 option 编号。
const (
	optRootPath   = 17  // Option 17：启动文件根路径（TFTP 前缀）
	optPrivate252 = 252 // Option 252：私有选项，常见用途是 WPAD
	optMessage    = 56  // Option 56：拒绝原因，NAK 时一并回给客户端
)

func (s *Server) reply(c *net.UDPConn, req *Packet, from *net.UDPAddr, msgType byte) string {
	if msgType == Nak {
		s.sendNak(c, req, from)
		if ip := req.RequestedIP(); ip != nil {
			return fmt.Sprintf("→ NAK %s（地址不可用）", ip)
		}
		return "→ NAK"
	}
	cfg := s.cfg
	serverIP := net.ParseIP(cfg.ServerIP).To4()
	nextServer := cfg.NextServer
	if nextServer == "" {
		nextServer = cfg.ServerIP
	}
	nextIP := net.ParseIP(nextServer).To4()
	mac := req.ClientMAC().String()

	// 启动文件名：架构表 -> Option 67 覆盖值；开启"仅 gPXE/iPXE"时对传统 PXE ROM 不发文件名。
	arch, _ := req.ClientArch()
	bootFile := cfg.BootFileFor(arch)
	if cfg.BootFileOption != "" {
		bootFile = cfg.BootFileOption
	}
	// iPXE/gPXE 已经在我们下发的引导文件里跑起来了，它第二次发请求是为了拿脚本：
	// 这时要改成给菜单脚本，否则会把 ipxe.efi 再下一次，形成“无限重载”或者 iPXE 无脚本可执行。
	if req.IsIPXE() && cfg.MenuFile != "" {
		bootFile = cfg.MenuFile
		// 用「完整 URL」把菜单下发出去：iPXE 会把脚本自身的 URI 当成工作目录，
		// 于是菜单里直接写相对路径（/wimboot、/wim-boot/bcd）就能自动落到同一个
		// 服务器和端口上，菜单文件里不用再写一遍端口（端口改了也不用改菜单）。
		// 与 pxe.MenuURL 的差别是刻意的：这里必须拿到字面 IP，主机名交给固件解析不可靠；
		// 那一个用于写菜单与调试输出，允许主机名。改任一处前先看另一处。
		if cfg.HTTPEnable && cfg.HTTPPort > 0 {
			host := cfg.BootHost()
			if host != "" && net.ParseIP(host) != nil {
				menu := strings.TrimPrefix(cfg.MenuFile, "/")
				// 默认端口（80）不写出来：http://192.168.5.1/menu.ipxe 看着跟正版一样清爽。
				if cfg.HTTPPort == 80 {
					bootFile = fmt.Sprintf("http://%s/%s", host, menu)
				} else {
					bootFile = fmt.Sprintf("http://%s:%d/%s", host, cfg.HTTPPort, menu)
				}
			}
		}
	} else if cfg.FilterUserClass && !req.IsIPXE() {
		bootFile = ""
	}
	// next-server：Option 66 取「引导服务器」回落链的同一结果。
	bootServer := cfg.BootHost()
	// 只有真正的 PXE 客户端（带 option 60 或 option 93）才该收到启动文件与启动服务器。
	// 否则像 WinPE 自己的 DHCP 客户端也会被塞一个 bootfile，虽然一般被忽略，但会污染日志、
	// 也可能让某些严格的 DHCP 客户端困惑。
	if _, hasArch := req.ClientArch(); !req.IsPXEClient() && !req.IsIPXE() && !hasArch {
		bootFile = ""
		bootServer = ""
	}
	classID := cfg.ClassID
	if classID == "" {
		classID = "PXEClient"
	}

	// addr 是这次应答分配到的地址（ProxyDHCP 不分配，留空），仅用于日志摘要。
	var addr string

	// 客户端架构：< 6 是 BIOS/Legacy（0 = x86 BIOS），>= 6 是 EFI。
	isLegacy := true
	if a, ok := req.ClientArch(); ok {
		isLegacy = a < 6
	}

	r := &Packet{Op: 2, HType: 1, HLen: 6, XID: req.XID, Flags: req.Flags}
	copy(r.CHAddr[:], req.CHAddr[:])
	r.GIAddr = req.GIAddr
	// sname 写本机主机名：正版 TinyPXE 一直这么干，个别老 ROM 会读它做显示或校验。
	if hostname != "" {
		copy(r.SName[:], hostname)
	}

	// BOOTP 固定字段 file/sname 也填上：老 PXE ROM 和 Hyper-V 的 UEFI 客户端只看 file 字段，
	// 不看 option 67，两边都写才能兼容。
	if bootFile != "" {
		copy(r.File[:], bootFile)
	}
	r.SetIP(OptServerID, serverIP)
	r.SetByte(OptMessageType, msgType)

	if cfg.ProxyDHCP {
		// ProxyDHCP 不分配地址，只告诉客户端"启动文件在哪"，地址由路由器 DHCP 给。
		copy(r.SIAddr[:], nextIP)
		r.SetString(OptVendorClass, classID)
		if bootServer != "" {
			r.SetString(OptTFTPServer, bootServer)
		}
		if bootFile != "" {
			r.SetString(OptBootFile, bootFile)
		}
	} else {
		ip, ok := s.alloc(mac, req.RequestedIP())
		if !ok {
			return fmt.Sprintf("→ 地址池 %s×%d 已满，未应答", cfg.PoolStart, cfg.PoolSize)
		}
		addr = ip.String()
		copy(r.YIAddr[:], ip.To4())
		copy(r.SIAddr[:], nextIP)
		if v := net.ParseIP(cfg.SubnetMask).To4(); v != nil {
			r.SetIP(OptSubnetMask, v)
		}
		if v := net.ParseIP(cfg.Router).To4(); v != nil && !v.Equal(net.IPv4zero) {
			r.SetIP(OptRouter, v)
		}
		if v := net.ParseIP(cfg.DNS).To4(); v != nil && !v.Equal(net.IPv4zero) {
			r.SetIP(OptDNS, v)
		}
		if v := net.ParseIP(cfg.Broadcast).To4(); v != nil && !v.Equal(net.IPv4zero) {
			r.SetIP(OptBroadcast, v)
		}
		if cfg.DomainName != "" {
			r.SetString(OptDomainName, cfg.DomainName)
		}
		r.SetU32(OptLeaseTime, cfg.LeaseTime)
		// 传统 BIOS 的 PXE ROM（实测 Realtek PCIe GBE + Intel UNDI PXE-2.1 build 083）
		// 只要选项里出现 option 60 / 66 / 67 或 option 43，就会自己去做 PXE 发现，
		// 一旦厂商扩展里的菜单信息不合它意就报 “PXE-E74” 直接退出。
		// 正版 TinyPXE 对同一台机器只回 1/3/6/13/28/51/53/54 + BOOTP 的 file/siaddr、
		// sname 写主机名、外加 option 175，客户端就老老实实去 TFTP 取文件了。
		// 所以 BIOS 一律走这种最小应答，EFI 保持完整应答不变。
		if req.IsPXEClient() && !isLegacy {
			r.SetString(OptVendorClass, classID)
		}
		if bootServer != "" && !isLegacy {
			r.SetString(OptTFTPServer, bootServer)
		}
		if bootFile != "" && !isLegacy {
			r.SetString(OptBootFile, bootFile)
		}
	}
	// 高级选项：根路径与私有选项对两种模式都适用。
	if cfg.RootPath != "" {
		r.SetString(optRootPath, cfg.RootPath)
	}
	if cfg.PrivateOption != "" {
		r.SetString(optPrivate252, cfg.PrivateOption)
	}
	// Option 43（PXE 厂商扩展）的下发策略：
	//   实测两条都失败：BIOS 收到带菜单的 43 → “PXE-E74” 退出；收到不带 43 但带 66/67/60
	//   的应答 → 去做 4011 发现然后放弃。正版 TinyPXE 对同一台 BIOS 机器压根不发 43，也不发
	//   60/66/67，只靠 BOOTP 的 file 字段，客户端就正常引导。所以 BIOS 一律不发 43。
	if !isLegacy && !cfg.NoOption43 && req.IsPXEClient() && bootFile != "" && (msgType == Offer || msgType == Ack) {
		if v := s.pxeVendorOptions(bootFile); len(v) > 0 {
			r.SetOpt(OptVendorSpecific, v)
		}
	}

	s.applyExtras(r)
	s.send(c, r, from)
	return replySummary(TypeName(msgType), addr, bootFile, bootServer)
}

// replySummary 把一次应答压成一句摘要，供 handle 打一行日志。
func replySummary(kind, addr, bootFile, bootServer string) string {
	s := "→ " + kind
	if addr != "" {
		s += " " + addr
	}
	if bootFile != "" {
		s += "，启动文件 " + bootFile
		if bootServer != "" {
			s += "（TFTP/HTTP " + bootServer + "）"
		}
	}
	return s
}

// hostname 是本机主机名，填进 DHCP 应答的 sname 字段（正版 TinyPXE 也这么做）。
var hostname = func() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}()

// applyExtras 把用户自定义的额外选项写进应答，覆盖任何同名标准选项。
func (s *Server) applyExtras(r *Packet) {
	for code, data := range s.extra {
		r.SetOpt(code, data)
	}
}

// pxeVendorOptions 组装 Option 43（PXE 厂商扩展）：
// sub 1 = 0.0.0.0（可选，见 Opt43Subopt1），sub 6 = PXE 发现控制，
// sub 8 = 启动服务器，sub 71 = 启动文件地址。
func (s *Server) pxeVendorOptions(bootFile string) []byte {
	ip := net.ParseIP(s.cfg.NextServer).To4()
	if ip == nil {
		return nil
	}
	var buf []byte
	if s.cfg.Opt43Subopt1 {
		// 部分老式 BIOS PXE 网卡缺这一项就不肯继续。
		buf = append(buf, 1, 4, 0, 0, 0, 0)
	}
	// 子项 6：发现控制 = 0x03（不广播发现、不组播发现，按子项 8 里的清单找启动服务）。
	buf = append(buf, 6, 1, 0x03)
	// 子项 8：PXE 启动服务器清单 = {类型 2 字节}{数量 1 字节}{IP 4 字节}，长度必须是 7。
	// 之前把长度写成 6，客户端解析出的 "数量" 是 IP 首字节，整个 Option 43 被丢弃。
	buf = append(buf, 8, 7, 0, 0, 1)
	buf = append(buf, ip...)

	// 子项 10 + 11：PXE 引导菜单与菜单提示。
	// 实测（Realtek PCIe GBE + Intel UNDI PXE-2.1 build 083）：客户端一旦在 Option 43 里
	// 看到 PXE 厂商标签，就要求菜单/提示信息齐全，否则直接报
	// “PXE-E74: Bad or missing PXE menu and/or prompt information” 并退出 PXE ROM。
	// 子项 10 的条目格式：{类型 2 字节}{文字长度 1 字节}{文字}；子项 11：{超时 2 字节}{提示文字}。
	label := []byte("Boot from network")
	buf = append(buf, 10, byte(len(label)+3), 0, 0, byte(len(label)))
	buf = append(buf, label...)
	prompt := []byte("Press F8 for boot menu")
	// PXE 规范里 Boot Prompt 的字段是 {超时 1 字节}{提示文字}，超时单位秒，0 表示不超时。
	// 之前多写了 1 字节，ROM 把提示文字整体读错位，就报
	// "PXE-E74: Bad or missing PXE menu and/or prompt information" 直接退出。
	buf = append(buf, 11, byte(len(prompt)+1), 0x03)
	buf = append(buf, prompt...)
	return buf
}

// sendNak 拒绝客户端的地址请求：NAK 不带 yiaddr，只告诉它那个地址不能用。
// 日志由调用方（handle → logPacket）统一打，这里不再自己输出。
func (s *Server) sendNak(c *net.UDPConn, req *Packet, from *net.UDPAddr) {
	r := &Packet{Op: 2, HType: 1, HLen: 6, XID: req.XID, Flags: req.Flags}
	r.GIAddr = req.GIAddr // 有中继时按同一路径回，否则会被当成广播发到 255.255.255.255
	copy(r.CHAddr[:], req.CHAddr[:])
	r.SetByte(OptMessageType, Nak)
	if own := net.ParseIP(s.cfg.ServerIP).To4(); own != nil {
		r.SetIP(OptServerID, own)
	}
	r.SetString(optMessage, "requested address not available")
	s.send(c, r, from)
}
