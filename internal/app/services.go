package app

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/dhcp"
	"pxeserv/internal/httpd"
	"pxeserv/internal/locales"
	"pxeserv/internal/netif"
	"pxeserv/internal/tftp"
)

// MenuURL 生成 iPXE 菜单的完整 URL（HTTP 启用且主机有效时返回 http://...，否则返回配置里的文件名）。
func MenuURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	menu := strings.TrimPrefix(cfg.MenuFile, "/")
	if menu == "" {
		menu = "menu.ipxe"
	}
	if !cfg.HTTPEnable || cfg.HTTPPort <= 0 {
		return menu
	}
	host := cfg.BootServerHost
	if host == "" {
		host = cfg.NextServer
	}
	if host == "" {
		host = cfg.ServerIP
	}
	if host == "" {
		return menu
	}
	return fmt.Sprintf("http://%s%s/%s", host, urlPort(cfg.HTTPPort), menu)
}

// writeHTTPHint 生成 <启动目录>/httpboot.ipxe。
// 内容只有一行 chain，指向本机 HTTP 上的菜单脚本；端口和 IP 取自当前配置，
// 所以用户改了 HTTP 端口也不需要手改任何菜单文件。
func (a *App) writeHTTPHint(cfg *config.Config) {
	if !cfg.HTTPEnable || cfg.HTTPPort <= 0 || cfg.ServerIP == "" {
		return
	}
	menu := MenuURL(cfg)
	body := fmt.Sprintf("#!ipxe\n# 由 PXE 启动服务器自动生成：把菜单换成 HTTP 拉取，后面的 initrd 才会走 HTTP。\nchain %s\n", menu)
	path := filepath.Join(cfg.TFTPRoot, "httpboot.ipxe")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		a.Errorf("启动目录：写入 %s 失败：%v", path, err)
		return
	}
	// 只写「菜单在哪」：把“为何走 HTTP”那段解释留给文档，不占每次启动的日志。
	a.Logf("菜单：%s", menu)
}

// urlPort 返回 URL 里该写的端口后缀：默认的 80 不写，其余端口写 ":端口"。
// 这样界面上看到、菜单里生成的地址都是 http://192.168.5.1/... 这种干净形式。
func urlPort(port int) string {
	if port <= 0 || port == 80 {
		return ""
	}
	return fmt.Sprintf(":%d", port)
}

// StartServices 依次拉起 DHCP、TFTP 与 HTTP，返回空串表示成功。
// 端口被占用是最常见的失败，用系统对话框提示一次。
func (a *App) StartServices() string {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return ""
	}
	cfg := a.cfg
	a.mu.Unlock()
	if cfg == nil {
		return locales.EmptyConfig()
	}
	if cfg.TFTPRoot == "" {
		msg := locales.EmptyBootRoot()
		a.Errorf("启动目录：%s", msg)
		return msg
	}
	if err := os.MkdirAll(cfg.TFTPRoot, 0o755); err != nil {
		msg := locales.BootRootUnusable(cfg.TFTPRoot, err.Error())
		a.Errorf("启动目录：%v", err)
		a.dialog(locales.BootRootDialogTitle(), msg)
		return msg
	}

	// 生成 boot/httpboot.ipxe：让 iPXE 用 HTTP 拉菜单，后面的 initrd 才会跟着走 HTTP。
	// TFTP 是停等协议（一块一个往返），731MB 的 boot.wim 走 TFTP 要好几分钟到十几分钟，
	// 走 HTTP 只要几十秒。文件很小，每次启动写一遍即可。
	a.writeHTTPHint(cfg)

	dhcpSrv := dhcp.New(cfg, a.logAt)
	tftpSrv := tftp.New(cfg.TFTPRoot, a.logAt)
	var httpSrv *httpd.Server
	if cfg.HTTPEnable {
		httpSrv = httpd.New(cfg.TFTPRoot, cfg.HTTPPort, a.logAt)
		if err := httpSrv.Listen(); err != nil {
			msg := fmt.Errorf("HTTP (tcp:%d): %w", cfg.HTTPPort, err).Error()
			a.Errorf("%s", msg)
			a.dialog(locales.AppTitle(), msg)
			return msg
		}
	}

	// DHCP binds synchronously, so its error is immediate.
	if err := dhcpSrv.Start(); err != nil {
		if httpSrv != nil {
			httpSrv.Stop()
		}
		msg := locales.DhcpStartFailed(err.Error())
		a.Errorf("DHCPd：%v", err)
		a.dialog(locales.DhcpFailDialogTitle(), msg)
		return msg
	}

	tftpErr := make(chan error, 1)
	go func() {
		if err := tftpSrv.Start(); err != nil {
			tftpErr <- fmt.Errorf("TFTP (udp:69): %w", err)
		}
		close(tftpErr)
	}()

	var httpErr chan error
	if httpSrv != nil {
		httpErr = make(chan error, 1)
		go func() {
			if err := httpSrv.Serve(); err != nil {
				httpErr <- fmt.Errorf("HTTP (tcp:%d): %w", cfg.HTTPPort, err)
			}
			close(httpErr)
		}()
	}

	// 给 TFTP 一点时间报告早期的绑定失败（如 udp:69 被占用）；
	// 通道在服务退出时会 close，保证后台监听 goroutine 不会永久阻塞泄漏。
	select {
	case err, ok := <-tftpErr:
		if ok && err != nil {
			dhcpSrv.Stop()
			tftpSrv.Stop()
			if httpSrv != nil {
				httpSrv.Stop()
			}
			a.Errorf("%v", err)
			a.dialog(locales.AppTitle(), err.Error())
			return err.Error()
		}
	case <-time.After(300 * time.Millisecond):
		go func() {
			if err, ok := <-tftpErr; ok && err != nil {
				a.Errorf("服务异常退出：%v", err)
			}
		}()
	}
	if httpErr != nil {
		go func() {
			if err, ok := <-httpErr; ok && err != nil {
				a.Errorf("服务异常退出：%v", err)
			}
		}()
	}

	a.mu.Lock()
	a.dhcpSrv, a.tftpSrv, a.httpSrv, a.running = dhcpSrv, tftpSrv, httpSrv, true
	a.mu.Unlock()
	if hint := otherSubnetHint(a.ListInterfaces(), cfg); hint != "" {
		a.Warnf("%s", hint)
	}
	return ""
}

// otherSubnetHint 在启动完成后提醒「本机还有别的网段」。
// 客户端接在别的网卡上时，它的广播到不了绑在 server_ip 上的 socket，
// 日志里一个包都不会出现——这是「之前好好的，突然启动不了」最常见的原因。
func otherSubnetHint(list []netif.Iface, cfg *config.Config) string {
	own := net.ParseIP(cfg.ServerIP).To4()
	mask := net.ParseIP(cfg.SubnetMask).To4()
	if own == nil || mask == nil {
		return ""
	}
	var others []string
	for _, it := range list {
		ip := net.ParseIP(it.IP).To4()
		m := net.ParseIP(it.Mask).To4()
		if ip == nil || m == nil {
			continue
		}
		if ip.Mask(net.IPMask(m)).Equal(own.Mask(net.IPMask(mask))) {
			continue
		}
		others = append(others, fmt.Sprintf("%s=%s", it.Name, it.IP))
	}
	if len(others) == 0 {
		return ""
	}
	return locales.OtherSubnetHint(strings.Join(others, "、"))
}

func (a *App) StopServices() {
	a.mu.Lock()
	d, t, h := a.dhcpSrv, a.tftpSrv, a.httpSrv
	a.dhcpSrv, a.tftpSrv, a.httpSrv, a.running = nil, nil, nil, false
	a.mu.Unlock()
	if d != nil {
		d.Stop()
	}
	if t != nil {
		t.Stop()
	}
	if h != nil {
		h.Stop()
	}
	if d != nil || t != nil || h != nil {
		a.Logf("服务已全部停止")
	}
}

func (a *App) Status() Status {
	a.mu.Lock()
	srv := a.dhcpSrv
	st := Status{
		Running: a.running,
		DHCP:    a.dhcpSrv != nil,
		TFTP:    a.tftpSrv != nil,
		HTTP:    a.httpSrv != nil,
		Leases:  []dhcp.Lease{},
	}
	if a.cfg != nil {
		st.Proxy = a.cfg.ProxyDHCP
		st.ServerIP = a.cfg.ServerIP
	}
	if a.debug != nil {
		st.DebugAddr = a.debug.addr
	}
	a.mu.Unlock()
	// 注意：Snapshots 会取 DHCP 自己的锁，必须在放开 a.mu 之后调用，
	// 否则一旦 DHCP 侧在持锁时回调 Logf（会拿 a.mu）就会死锁。
	if srv != nil {
		st.Leases = srv.Snapshots()
	}
	return st
}

// ReleaseLease 按 MAC 释放一条租约，让地址可以再次分配。
func (a *App) ReleaseLease(mac string) string {
	a.mu.Lock()
	srv := a.dhcpSrv
	a.mu.Unlock()
	if srv == nil {
		return locales.ServicesNotStarted()
	}
	if !srv.ReleaseLease(mac) {
		return locales.LeaseNotFound(mac)
	}
	a.Logf("DHCPd：已释放租约 %s", mac)
	return ""
}
