package dhcp

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

// 本文件里用到但 packet.go 未定义的 option 编号。
const (
	optRootPath   = 17  // Option 17：启动文件根路径（TFTP 前缀）
	optPrivate252 = 252 // Option 252：私有选项，常见用途是 WPAD
	optMessage    = 56  // Option 56：拒绝原因，NAK 时一并回给客户端

	// quarantinePeriod 是地址被客户端声明冲突（DECLINE）后的隔离时长，
	// 隔离期内不再分配，避免把已知冲突的地址立刻发给下一台机器。
	quarantinePeriod = 60 * time.Second
)

// Logf 是日志回调，由调用方决定输出到控制台还是推送到界面。
type Logf = logbook.Logger

type lease struct {
	mac     string
	ip      net.IP
	expires time.Time
}

// Server 是 DHCP / ProxyDHCP 服务端。
type Server struct {
	cfg    *config.Config
	logger logbook.Logger

	mainConn *net.UDPConn // udp:67，绑在 server_ip 上（收+发都走这块网卡）
	bootConn *net.UDPConn // udp:4011，ProxyDHCP 模式下应答 PXE 启动服务请求

	extra  map[byte][]byte // 额外 DHCP 选项，来自“更多”里的自定义项
	mu     sync.Mutex
	leases map[string]*lease // MAC -> 租约
	byIP   map[string]string // IP -> MAC

	burst packetBurst // 包级日志合并，见 logPacket

	quarantine map[string]time.Time // IP -> 隔离到期时间（客户端报冲突后暂时不用）

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func New(cfg *config.Config, logf Logf) *Server {
	return &Server{
		cfg:        cfg,
		logger:     logf,
		leases:     make(map[string]*lease),
		byIP:       make(map[string]string),
		quarantine: make(map[string]time.Time),
		stop:       make(chan struct{}),
	}
}

// bindUDP 绑定一个 UDP 套接字并打开 SO_BROADCAST。
// 不要加 SO_REUSEADDR：Windows 上它会让第二个进程“静默劫持”已经在用的端口，
// 用户就看不到「udp:67 已被占用」的报错了。实测 0.0.0.0:67 与 192.168.5.1:67
// 在不加 REUSEADDR 的情况下本来就可以并存，而且两个 socket 都能收到广播。
func bindUDP(addr string) (*net.UDPConn, error) {
	a, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp4", a)
	if err != nil {
		return nil, err
	}
	if err := enableBroadcast(c); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (s *Server) Start() error {
	conn, err := bindUDP(net.JoinHostPort(s.cfg.ServerIP, "67"))
	if err != nil {
		conn, err = bindUDP(":67")
		if err != nil {
			return fmt.Errorf("DHCPd 绑定 udp:67 失败: %w", err)
		}
		s.logf(logbook.Warn, "DHCPd：%s 不是本机网卡地址，已退回监听 0.0.0.0:67；此时广播应答可能从错误网卡发出，请在界面里选对网卡", s.cfg.ServerIP)
	}
	s.mainConn = conn

	mode := "普通 DHCP"
	if s.cfg.ProxyDHCP {
		mode = "ProxyDHCP（只应答 PXE 客户端）"
	}
	// 额外选项只在启动时解析一次；写错不影响启动，只是忽略并提示。
	s.extra = make(map[byte][]byte)
	if extra, err := s.cfg.ParseExtraOptions(); err != nil {
		s.logf(logbook.Warn, "DHCPd：额外选项无效，已忽略：%v", err)
	} else if len(extra) > 0 {
		s.extra = extra
		codes := make([]string, 0, len(extra))
		for code := range extra {
			codes = append(codes, strconv.Itoa(int(code)))
		}
		sort.Strings(codes)
		s.logf(logbook.Info, "DHCPd：额外选项已生效：option %s", strings.Join(codes, ", "))
	}
	// 启动信息在 4011 绑完之后合成一行：监听地址、模式、地址池、广播目标、4011 是否就绪。
	// 这几条原本各占一行，每次启停都重复一遍，把真正有价值的包级日志挤到看不见的地方。
	dests := make([]string, 0, 2)
	for _, d := range s.broadcastDests() {
		dests = append(dests, d.IP.String()+":68")
	}

	s.wg.Add(1)
	go s.loop(conn, false)

	// 4011 必须无条件下绑定：应答里带了 Option 43（PXE 启动服务清单）之后，客户端会先向
	// 4011 发 "PXE boot server request"，没人应答它就直接报 "A boot image was not found"，
	// 连一次 TFTP 都不会发起。两种模式都绑，成本只是一行日志。
	{
		boot, err := bindUDP(net.JoinHostPort(s.cfg.ServerIP, "4011"))
		if err != nil {
			boot, err = bindUDP(":4011")
		}
		if err != nil {
			s.logf(logbook.Warn, "DHCPd：PXE 启动服务 udp:4011 绑定失败（部分客户端会改走 67 应答）: %v", err)
		} else {
			s.bootConn = boot
			s.wg.Add(1)
			go s.loop(boot, true)
		}
	}

	s.logf(logbook.Info, "%s", startupSummary(conn.LocalAddr(), mode, s.cfg.PoolStart, s.cfg.PoolSize, dests, s.bootConn != nil))
	if s.cfg.ProxyDHCP {
		s.logf(logbook.Warn, "DHCPd：ProxyDHCP 只补 PXE 启动信息、不分配 IP；本网段若没有其他 DHCP 服务器，客户端会一直停在 DHCPDISCOVER（在设置里取消 ProxyDHCP 即改为自己发 IP）")
	}
	// 后台定期清理过期租约：界面没在轮询、也没有新客户端时，过期条目不该一直占着内存。
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.mu.Lock()
				s.sweepLocked()
				s.mu.Unlock()
			}
		}
	}()
	return nil
}

func (s *Server) Stop() {
	first := false
	s.stopOnce.Do(func() {
		first = true
		close(s.stop)
	})
	if !first {
		return
	}
	if s.mainConn != nil {
		s.mainConn.Close()
	}
	if s.bootConn != nil {
		s.bootConn.Close()
	}
	s.wg.Wait()
	s.logf(logbook.Info, "DHCPd：已停止")
}

func (s *Server) loop(conn *net.UDPConn, onBootService bool) {
	defer s.wg.Done()
	buf := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.stop:
				return
			default:
			}
			s.logf(logbook.Error, "DHCPd：读取失败: %v", err)
			continue
		}
		pkt, err := Parse(buf[:n])
		if err != nil || pkt.Op != 1 {
			continue
		}
		s.handle(conn, pkt, from, onBootService)
	}
}

func (s *Server) handle(c *net.UDPConn, req *Packet, from *net.UDPAddr, onBootService bool) {
	mac := req.ClientMAC().String()
	if mac == "" || mac == "00:00:00:00:00:00" {
		return // 畸形或全零 MAC，分配出去只会互相覆盖租约
	}
	arch, _ := req.ClientArch()
	msg := req.MessageType()

	// ProxyDHCP 只服务 PXE 客户端，其他机器的报文一概不理，避免干扰正常 DHCP。
	if s.cfg.ProxyDHCP && !req.IsPXEClient() {
		return
	}

	// 一条报文只留一行日志：收到什么、怎么应答。应答摘要由 dispatch 回填，
	// 同一客户端 5 秒内的相同结论交给 logPacket 合并。
	s.logPacket(mac, TypeName(msg), arch, onBootService, s.dispatch(c, req, from, onBootService))
}

// dispatch 处理一条报文，并把「怎么应答」压成一句摘要（“→ OFFER 192.168.5.2，启动文件 …”）。
func (s *Server) dispatch(c *net.UDPConn, req *Packet, from *net.UDPAddr, onBootService bool) string {
	mac := req.ClientMAC().String()
	msg := req.MessageType()

	switch msg {
	case Discover, Request, Inform:
		if onBootService {
			// 4011 上的 PXE 启动服务：老式 BIOS ROM 会先发 DISCOVER，再发 REQUEST。
			// 只回 REQUEST 的话，先发 DISCOVER 的那批固件收不到 OFFER 就直接放弃了。
			switch msg {
			case Discover:
				return s.reply(c, req, from, Offer)
			default:
				return s.reply(c, req, from, Ack)
			}
		}
		if msg == Discover {
			return s.reply(c, req, from, Offer)
		}
		// 客户端在 option 54 里点名了别的 DHCP 服务器，说明它没选我们，不要插嘴。
		if sid := req.Get(OptServerID); len(sid) == 4 {
			if own := net.ParseIP(s.cfg.ServerIP).To4(); own != nil && !net.IP(sid).Equal(own) {
				return fmt.Sprintf("→ 忽略（客户端选了其他服务器 %s）", net.IP(sid))
			}
		}
		// REQUEST 里点名的地址如果已被别人占用、不在池里或刚被举报冲突，必须回 NAK，
		// 否则客户端会一直拿着一个不能用的地址（典型是换网段后拿旧地址死磕）。
		if ip := req.RequestedIP(); ip != nil && s.requestUnusable(mac, ip) {
			return s.reply(c, req, from, Nak)
		}
		return s.reply(c, req, from, Ack)
	case Release:
		s.release(mac)
		return "→ 已释放"
	case Decline:
		s.dropAddress(req.RequestedIP())
		return "→ 地址冲突，已回收"
	}
	return "→ 已忽略"
}

// burstWindow 是「同一客户端、同一结论」的日志合并窗口。
const burstWindow = 5 * time.Second

// logf 统一出口：级别由调用点标注，落盘与筛选归上层。
func (s *Server) logf(level logbook.Level, format string, args ...any) {
	s.logger(level, format, args...)
}

// packetLevel 给包头日志挑级别：地址池满、地址冲突这类“客户端拿不到地址”
// 的结果值得标黄，正常一轮问答只是进度。
func packetLevel(result string) logbook.Level {
	switch {
	case strings.HasPrefix(result, "→ 地址池"),
		strings.HasPrefix(result, "→ 地址冲突"):
		return logbook.Warn
	}
	return logbook.Info
}

// packetBurst 记住上一条包头日志的 key 与连续出现次数。
type packetBurst struct {
	mu   sync.Mutex
	key  string
	n    int
	last time.Time
}

// hit 记一次出现，返回是否要打日志以及当前连续次数。
func (b *packetBurst) hit(key string) (bool, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if key != b.key || now.Sub(b.last) > burstWindow {
		b.key, b.n = key, 0
	}
	b.n++
	b.last = now
	return b.n == 1 || b.n%5 == 0, b.n
}

// logPacket 打一行「收到什么 + 怎么应答」。
// 同一 MAC 在 5 秒内反复重发同样的请求时只计数（第 1 次与每 5 次各打一行），
// 否则客户端拿不到地址时会把日志刷满，真正的线索被冲到看不见的地方。
func (s *Server) logPacket(mac, msg string, arch uint16, onBootService bool, result string) {
	ok, n := s.burst.hit(mac + " " + msg + " " + result)
	if !ok {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DHCPd：MAC=%s %s", mac, msg)
	if onBootService {
		fmt.Fprintf(&b, "（4011，架构=%d）", arch)
	}
	if result != "" {
		b.WriteString(" " + result)
	}
	if n > 1 {
		fmt.Fprintf(&b, "（同 MAC 第 %d 次）", n)
	}
	s.logf(packetLevel(result), "%s", b.String())
}

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
	next := cfg.NextServer
	if next == "" {
		next = cfg.ServerIP
	}
	nextIP := net.ParseIP(next).To4()
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
		if cfg.HTTPEnable && cfg.HTTPPort > 0 {
			host := cfg.BootServerHost
			if host == "" {
				host = next
			}
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
	// next-server：Option 66 有覆盖值就优先用它。
	bootServer := cfg.BootServerHost
	if bootServer == "" {
		bootServer = next
	}
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

// requestUnusable 判断客户端在 REQUEST 里点名的地址能不能给它。
func (s *Server) requestUnusable(mac string, ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	if !s.poolContains(ip4) || s.reserved(ip4) {
		return true
	}
	if owner, taken := s.byIP[ip4.String()]; taken && owner != mac {
		return true
	}
	if until, bad := s.quarantine[ip4.String()]; bad && time.Now().Before(until) {
		return true
	}
	return false
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

// broadcastDest 计算广播应答的目的地址。
//
// 用 server_ip + 子网掩码算出的子网广播（如 192.168.5.255）比 255.255.255.255 更可靠：
// Windows 对有限广播 255.255.255.255 只有一个模糊路由，可能从错误的网卡发出去；
// 子网广播则能用明确的网段路由，从配置的那块网卡发出。
func (s *Server) broadcastDest() *net.UDPAddr {
	if ip := net.ParseIP(s.cfg.ServerIP).To4(); ip != nil {
		if m := net.ParseIP(s.cfg.SubnetMask).To4(); m != nil {
			b := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				b[i] = ip[i] | ^m[i]
			}
			return &net.UDPAddr{IP: b, Port: 68}
		}
	}
	if ip := net.ParseIP(s.cfg.Broadcast).To4(); ip != nil && !ip.Equal(net.IPv4zero) {
		return &net.UDPAddr{IP: ip, Port: 68}
	}
	return &net.UDPAddr{IP: net.IPv4(255, 255, 255, 255), Port: 68}
}

// startupSummary 生成本次监听的启动摘要行。监听地址、模式、地址池、广播目标、4011 是否就绪
// 合成一行，以免每次启停都刷六行固定台词。
func startupSummary(addr net.Addr, mode, poolStart string, poolSize int, dests []string, pxeBound bool) string {
	pxe := "PXE 4011 未绑定"
	if pxeBound {
		pxe = "PXE 4011"
	}
	return fmt.Sprintf("DHCPd：已在 %s 上监听（%s，地址池 %s×%d，广播应答 → %s，%s）",
		addr, mode, poolStart, poolSize, strings.Join(dests, "、"), pxe)
}

// broadcastDests 返回广播应答要去的全部地址：配置算出来的广播 + 有限广播 255.255.255.255。
func (s *Server) broadcastDests() []*net.UDPAddr {
	first := s.broadcastDest()
	limited := &net.UDPAddr{IP: net.IPv4(255, 255, 255, 255), Port: 68}
	if first.IP.Equal(limited.IP) {
		return []*net.UDPAddr{first}
	}
	return []*net.UDPAddr{first, limited}
}

func (s *Server) send(c *net.UDPConn, r *Packet, from *net.UDPAddr) {
	var dst *net.UDPAddr
	switch {
	case r.GIAddr != [4]byte{}:
		// 经过中继代理：回给中继的 67 端口。
		dst = &net.UDPAddr{IP: net.IP(r.GIAddr[:]), Port: 67}
	case c == s.bootConn && from != nil && r.Flags&0x8000 == 0:
		// 4011 上的 PXE 启动服务请求：客户端没要求广播时单播回请求方（它不在 68 端口等）。
		// 但 BIOS 的 PXE ROM 在阶段 2 往往还没把 IP 协议栈跑起来，收不到单播 IP 包，
		// 它会置广播标志位（flags & 0x8000），此时必须走广播。
		dst = from
	default:
		// 广播应答：子网广播（192.168.5.255）与有限广播（255.255.255.255）各发一份。
		// 不同 UEFI 固件只认其中一种 —— 有的最小实现只接受 255.255.255.255，
		// 有的只接受本网段广播。两份内容完全一样，客户端会忽略重复的那份。
		payload := r.Marshal()
		ok := false
		for _, d := range s.broadcastDests() {
			if _, err := c.WriteToUDP(payload, d); err == nil {
				ok = true
			} else {
				s.logf(logbook.Error, "DHCPd：向 %s 发送应答失败：%v", d.IP, err)
			}
		}
		if !ok {
			s.logf(logbook.Warn, "DHCPd：广播应答发送失败")
		}
		return
	}
	if _, err := c.WriteToUDP(r.Marshal(), dst); err != nil {
		s.logf(logbook.Error, "DHCPd：发送应答失败: %v", err)
	}
}

// ---------- 地址池 ----------

func (s *Server) baseIP() net.IP {
	ip := net.ParseIP(s.cfg.PoolStart).To4()
	return ip
}

func ipToU32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

func u32ToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).To4()
}

// poolRange 返回地址池的有效主机范围 [startU, endU]（已排除网络号和广播地址）。
func (s *Server) poolRange() (startU, endU uint64, ok bool) {
	base := s.baseIP()
	if base == nil || s.cfg.PoolSize <= 0 {
		return 0, 0, false
	}
	maskIP := net.ParseIP(s.cfg.SubnetMask).To4()
	if maskIP == nil {
		maskIP = net.IPv4(255, 255, 255, 0).To4()
	}
	maskU := ipToU32(maskIP)
	sU := ipToU32(base)
	netU := sU & maskU
	bcastU := netU | ^maskU
	if sU <= netU || sU >= bcastU {
		return 0, 0, false
	}
	eU := uint64(sU) + uint64(s.cfg.PoolSize) - 1
	if eU >= uint64(bcastU) {
		eU = uint64(bcastU) - 1
	}
	return uint64(sU), eU, eU >= uint64(sU)
}

func (s *Server) poolContains(ip net.IP) bool {
	ip4 := ip.To4()
	startU, endU, ok := s.poolRange()
	if !ok || ip4 == nil {
		return false
	}
	v := uint64(ipToU32(ip4))
	if v < startU || v > endU {
		return false
	}
	return !s.reserved(ip4)
}

// reserved 判断该地址是否被服务器自己或网关占用，绝不会分配给客户端。
func (s *Server) reserved(ip net.IP) bool {
	for _, v := range []string{s.cfg.ServerIP, s.cfg.Router} {
		if v == "" {
			continue
		}
		if own := net.ParseIP(v).To4(); own != nil && own.Equal(ip.To4()) {
			return true
		}
	}
	return false
}

// alloc 返回应当分配给该 MAC 的地址；失败说明池子已满。
func (s *Server) alloc(mac string, requested net.IP) (net.IP, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()

	ttl := time.Duration(s.cfg.LeaseTime) * time.Second
	now := time.Now()

	if l, ok := s.leases[mac]; ok && (requested == nil || requested.Equal(l.ip)) {
		l.expires = now.Add(ttl)
		return l.ip, true
	}
	if requested != nil && s.poolContains(requested) {
		owner, taken := s.byIP[requested.String()]
		until, bad := s.quarantine[requested.String()]
		if (!taken || owner == mac) && (!bad || now.After(until)) && !s.reserved(requested) {
			s.bindLocked(mac, requested, now.Add(ttl))
			return requested, true
		}
	}
	if l, ok := s.leases[mac]; ok {
		l.expires = now.Add(ttl)
		return l.ip, true
	}
	startU, endU, ok := s.poolRange()
	if !ok {
		return nil, false
	}
	for v := startU; v <= endU; v++ {
		ip := u32ToIP(uint32(v))
		if s.reserved(ip) {
			continue
		}
		if _, taken := s.byIP[ip.String()]; taken {
			continue
		}
		if until, bad := s.quarantine[ip.String()]; bad && now.Before(until) {
			continue
		}
		s.bindLocked(mac, ip, now.Add(ttl))
		return ip, true
	}
	return nil, false
}

func (s *Server) bindLocked(mac string, ip net.IP, expires time.Time) {
	if old, ok := s.leases[mac]; ok {
		delete(s.byIP, old.ip.String())
	}
	s.leases[mac] = &lease{mac: mac, ip: ip, expires: expires}
	s.byIP[ip.String()] = mac
}

func (s *Server) release(mac string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.leases[mac]; ok {
		delete(s.byIP, l.ip.String())
		delete(s.leases, mac)
	}
}

func (s *Server) dropAddress(ip net.IP) {
	if ip == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	s.quarantine[ip4.String()] = time.Now().Add(quarantinePeriod)
	if mac, ok := s.byIP[ip4.String()]; ok {
		if l, ok2 := s.leases[mac]; ok2 && l.ip.Equal(ip4) {
			delete(s.leases, mac)
		}
		delete(s.byIP, ip4.String())
	}
}

func (s *Server) sweepLocked() {
	now := time.Now()
	for mac, l := range s.leases {
		if now.After(l.expires) {
			delete(s.byIP, l.ip.String())
			delete(s.leases, mac)
		}
	}
}

// Lease 是暴露给界面的一条租约。
type Lease struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	ExpiresAt string `json:"expiresAt"`
	Remaining int    `json:"remainingSec"`
}

func (s *Server) Snapshots() []Lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	out := make([]Lease, 0, len(s.leases))
	now := time.Now()
	for _, l := range s.leases {
		left := int(l.expires.Sub(now).Seconds())
		if left < 0 {
			left = 0
		}
		out = append(out, Lease{
			MAC:       l.mac,
			IP:        l.ip.String(),
			ExpiresAt: l.expires.Format("15:04:05"),
			Remaining: left,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

func (s *Server) ReleaseLease(mac string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leases[mac]
	if !ok {
		return false
	}
	delete(s.byIP, l.ip.String())
	delete(s.leases, mac)
	return true
}
