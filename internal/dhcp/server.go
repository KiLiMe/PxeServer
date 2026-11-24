// 拆分点：DHCP 服务端的结构与生命周期（起停、读循环、连接持有）。
// 引用方向：本文件 → dispatch.go / transport.go / reply.go / pool.go / log.go，单向。
// 按需阅读：改「哪些端口绑在哪」看 bindService 与 Start；改「报文怎么处理」去 dispatch.go。

package dhcp

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

// Server 是 DHCP / ProxyDHCP 服务端。
type Server struct {
	cfg    *config.Config
	logger logbook.Logger

	network  dhcpNetwork
	mainConn *net.UDPConn // udp:67，收发使用 server_ip 对应的网卡
	bootConn *net.UDPConn // udp:4011，ProxyDHCP 模式下应答 PXE 启动服务请求

	extra  map[byte][]byte // 额外 DHCP 选项，来自主页的「额外 DHCP 选项」输入框
	mu     sync.Mutex
	leases map[string]*lease // MAC -> 租约
	byIP   map[string]string // IP -> MAC

	burst packetBurst // 包级日志合并，见 logPacket

	quarantine map[string]time.Time // IP -> 隔离到期时间（客户端报冲突后暂时不用）

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New 构造服务端。cfg 必须已经过 Normalize：NextServer 已落成实际值，应答直接取用。
// MenuFile / ClassID 例外——空串有含义（不下发菜单、用默认厂商类），判定留在应答构造里。
func New(cfg *config.Config, logger logbook.Logger) *Server {
	return &Server{
		cfg:        cfg,
		logger:     logbook.Or(logger),
		leases:     make(map[string]*lease),
		byIP:       make(map[string]string),
		quarantine: make(map[string]time.Time),
		stop:       make(chan struct{}),
	}
}

func (s *Server) Start() error {
	if err := s.setupNetwork(); err != nil {
		return err
	}
	conn, err := s.bindService("67")
	if err != nil {
		return fmt.Errorf("绑定 udp:67 失败：%w", err)
	}
	s.mainConn = conn

	mode := "普通 DHCP"
	if s.cfg.ProxyDHCP {
		mode = "ProxyDHCP（只应答 PXE 客户端）"
	}
	// 额外选项只在启动时解析一次；写错不影响启动，只是忽略并提示。
	s.extra = make(map[byte][]byte)
	if extra, err := s.cfg.ParseExtraOptions(); err != nil {
		s.logger(logbook.Warn, "DHCPd：额外选项无效，已忽略：%v", err)
	} else if len(extra) > 0 {
		s.extra = extra
		codes := make([]string, 0, len(extra))
		for code := range extra {
			codes = append(codes, strconv.Itoa(int(code)))
		}
		sort.Strings(codes)
		s.logger(logbook.Info, "DHCPd：额外选项已生效：option %s", strings.Join(codes, ", "))
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
		boot, err := s.bindService("4011")
		if err != nil {
			s.logger(logbook.Warn, "DHCPd：PXE 启动服务 udp:4011 绑定失败（部分客户端会改走 67 应答）: %v", err)
		} else {
			s.bootConn = boot
			s.wg.Add(1)
			go s.loop(boot, true)
		}
	}

	s.logger(logbook.Info, "%s", startupSummary(conn.LocalAddr(), mode, s.cfg.PoolStart, s.cfg.PoolSize, dests, s.bootConn != nil))
	if s.cfg.ProxyDHCP {
		s.logger(logbook.Warn, "DHCPd：ProxyDHCP 只补 PXE 启动信息、不分配 IP；本网段若没有其他 DHCP 服务器，客户端会一直停在 DHCPDISCOVER（在设置里取消 ProxyDHCP 即改为自己发 IP）")
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
	s.logger(logbook.Info, "DHCPd：已停止")
}

func (s *Server) loop(conn *net.UDPConn, onBootService bool) {
	defer s.wg.Done()
	buf := make([]byte, 1500)
	for {
		n, from, err := s.network.read(conn, buf)
		if err != nil {
			select {
			case <-s.stop:
				return
			default:
			}
			s.logger(logbook.Error, "DHCPd：读取失败: %v", err)
			continue
		}
		pkt, err := Parse(buf[:n])
		if err != nil || pkt.Op != 1 {
			continue
		}
		s.handle(conn, pkt, from, onBootService)
	}
}
