package app

// 本地调试接口：只能通过命令行 -debug-port <端口> 打开，只监听 127.0.0.1 的该端口，
// 实际地址写在 config.json 的 runtime.debug_addr。不传参数时完全不启动。
// 存在的意义是让脚本（包括 AI）能直接观察和驱动正在运行的程序，
// 而不必像以前那样再编一个命令行版可执行文件去抢 67/69 端口。
// 只接受本机直连：带 Origin 头的请求（浏览器跨站）与 Host 对不上监听地址的请求（DNS 重绑定）一律 403。
//
// 接口（GET 只读，/start 与 /stop 需 POST，返回 JSON）：
//   /status      运行状态 + 租约
//   /logs?n=200  最近 n 行日志
//   /config      当前配置
//   POST /start /stop 启动 / 停止服务
//   /selftest    DHCP 报文编解码自检（不联网、不占端口）
//   /decode?hex= DHCP 报文十六进制解析
//   /tftp?file=  通过 TFTP 从本机服务下载一个文件，返回大小与 SHA-256
//   /http?path=&range= 通过 HTTP 从本机服务取一个文件，返回状态码与首字节
//   /interfaces  本机网卡列表

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/dhcp"
	"pxeserv/internal/pxe"
)

type debugAPI struct {
	srv  *http.Server
	addr string
}

// debugListenAddr 把 -debug-port 的端口号换算成监听地址：
// 0 表示不开接口（返回空地址），只监听回环地址，非 1–65535 视为参数错误。
func debugListenAddr(port int) (string, error) {
	if port == 0 {
		return "", nil
	}
	if port < 0 || port > 65535 {
		return "", fmt.Errorf("端口 %d 不在 1–65535 范围内", port)
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
}

func (a *App) startDebugAPI() {
	addr, err := debugListenAddr(a.debugPort)
	if err != nil {
		a.Warnf("调试接口：%v（已跳过）", err)
		return
	}
	if addr == "" {
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		a.Warnf("调试接口：%v（已跳过）", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.Status()) })
	mux.HandleFunc("/logs", a.dbgLogs)
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.snapshot()) })
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		writeJSON(w, map[string]string{"result": orOK(a.StartServices())})
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		a.StopServices()
		writeJSON(w, map[string]string{"result": "已停止"})
	})
	mux.HandleFunc("/selftest", func(w http.ResponseWriter, r *http.Request) { a.dbgSelfTest(w) })
	mux.HandleFunc("/decode", a.dbgDecode)
	mux.HandleFunc("/tftp", a.dbgTFTP)
	mux.HandleFunc("/http", a.dbgHTTP)
	mux.HandleFunc("/interfaces", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.ListInterfaces()) })

	lnAddr := ln.Addr().String()
	d := &debugAPI{srv: &http.Server{Handler: guardLocal(lnAddr, mux)}, addr: lnAddr}
	a.mu.Lock()
	a.debug = d
	a.mu.Unlock()
	// 端口对外可见：写进 config.json 的 runtime 段，脚本读一份文件就够。
	_ = config.SetRuntime(a.cfgPath, func(r *config.Runtime) { r.DebugAddr = d.addr })
	a.Logf("调试接口：http://%s（仅本机）", d.addr)
	go func() { _ = d.srv.Serve(ln) }()
}

func (a *App) stopDebugAPI() {
	a.mu.Lock()
	d := a.debug
	a.debug = nil
	a.mu.Unlock()
	if d == nil {
		return
	}
	_ = d.srv.Close()
	_ = config.SetRuntime(a.cfgPath, func(r *config.Runtime) { r.DebugAddr = "" })
}

// guardLocal 只放行本机直连的请求。命令行与 curl 不会带 Origin，
// 而网页发出的跨站请求一定带（哪怕是简单的 POST），所以有 Origin 就拒；
// Host 必须是回环地址加实际端口，否则 DNS 重绑定能把外部域名解析到 127.0.0.1。
func guardLocal(addr string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" || !isLoopbackHost(r.Host, addr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost 判断请求的 Host 头是否就是本机监听地址（地址或端口不符都算不符）。
func isLoopbackHost(hostHeader, addr string) bool {
	_, wantPort, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	host, port, err := net.SplitHostPort(hostHeader)
	if err != nil || port != wantPort {
		return false
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// postOnly 只放行 POST；其余方法写 405 并返回 false。
func postOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost {
		return true
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func orOK(msg string) string {
	if msg == "" {
		return "ok"
	}
	return msg
}

func (a *App) dbgLogs(w http.ResponseWriter, r *http.Request) {
	n := 200
	fmt.Sscanf(r.URL.Query().Get("n"), "%d", &n)
	logs := a.LogText()
	if n > 0 && len(logs) > n {
		logs = logs[len(logs)-n:]
	}
	writeJSON(w, map[string]any{"count": len(logs), "lines": logs})
}

// dbgSelfTest 不需要联网、不占端口：构造一个 UEFI x64 的 DISCOVER 走一遍编解码。
func (a *App) dbgSelfTest(w http.ResponseWriter) {
	cfg := a.snapshot()
	p := &dhcp.Packet{Op: 1, HType: 1, HLen: 6, XID: 0x5eed1234, Flags: 0x8000}
	copy(p.CHAddr[:], []byte{0x52, 0x54, 0x00, 0x12, 0x34, 0x56})
	p.SetByte(dhcp.OptMessageType, dhcp.Discover)
	p.SetString(dhcp.OptVendorClass, "PXEClient:Arch:00007:UNDI:003000")
	p.SetString(dhcp.OptUserClass, "iPXE")
	p.SetOpt(dhcp.OptClientArch, []byte{0, 7})

	raw := p.Marshal()
	back, err := dhcp.Parse(raw)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := map[string]any{
		"ok":       back.MessageType() == dhcp.Discover && len(raw) >= 300,
		"bytes":    len(raw),
		"mac":      back.ClientMAC().String(),
		"arch":     mustArch(back),
		"pxe":      back.IsPXEClient(),
		"ipxe":     back.IsIPXE(),
		"bootFile": "",
		"serverIP": "",
		"httpPort": 0,
		"menuFile": "",
	}
	if cfg != nil {
		out["serverIP"] = cfg.ServerIP
		out["httpPort"] = cfg.HTTPPort
		out["menuFile"] = cfg.MenuFile
		if art, ok := back.ClientArch(); ok {
			if cfg.BootFileOption != "" && !back.IsIPXE() {
				out["bootFile"] = cfg.BootFileOption
			} else if back.IsIPXE() && cfg.MenuFile != "" {
				out["bootFile"] = pxe.MenuURL(cfg)
			} else {
				out["bootFile"] = cfg.BootFileFor(art)
			}
		}
	}
	writeJSON(w, out)
}

func mustArch(p *dhcp.Packet) int {
	if v, ok := p.ClientArch(); ok {
		return int(v)
	}
	return -1
}

func (a *App) dbgDecode(w http.ResponseWriter, r *http.Request) {
	h := strings.TrimSpace(r.URL.Query().Get("hex"))
	raw, err := hex.DecodeString(strings.ReplaceAll(h, " ", ""))
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "hex 解析失败：" + err.Error()})
		return
	}
	p, err := dhcp.Parse(raw)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	opts := map[string]string{}
	for code, v := range p.Options {
		opts[fmt.Sprintf("%d", code)] = hex.EncodeToString(v)
	}
	writeJSON(w, map[string]any{
		"ok": true, "op": p.Op, "xid": fmt.Sprintf("0x%08x", p.XID),
		"mac": p.ClientMAC().String(), "type": dhcp.TypeName(p.MessageType()),
		"yiaddr": net.IP(p.YIAddr[:]).String(), "siaddr": net.IP(p.SIAddr[:]).String(),
		"file":    strings.TrimRight(string(p.File[:]), "\x00"),
		"options": opts,
	})
}

// dbgTFTP 用最朴素的 RRQ（blksize 512）从本机 TFTP 服务拉一个文件。
func (a *App) dbgTFTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("file"))
	if name == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 file 参数"})
		return
	}
	cfg := a.snapshot()
	if cfg == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "配置未加载"})
		return
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer conn.Close()

	req := []byte{0, 1}
	req = append(req, []byte(name)...)
	req = append(req, 0)
	req = append(req, []byte("octet")...)
	req = append(req, 0)

	server := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 69}
	if _, err := conn.WriteToUDP(req, server); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	var buf [2048]byte
	var data []byte
	expect := uint16(1)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			break
		}
		if n < 4 {
			continue
		}
		if buf[1] == 5 { // ERROR
			writeJSON(w, map[string]any{"ok": false, "error": "TFTP 错误包：" + strings.TrimRight(string(buf[4:n]), "\x00")})
			return
		}
		block := uint16(buf[2])<<8 | uint16(buf[3])
		if block != expect {
			continue
		}
		data = append(data, buf[4:n]...)
		ack := []byte{0, 4, buf[2], buf[3]}
		_, _ = conn.WriteToUDP(ack, from)
		expect++
		if n-4 < 512 {
			sum := sha256.Sum256(data)
			writeJSON(w, map[string]any{"ok": true, "file": name, "bytes": len(data), "sha256": hex.EncodeToString(sum[:])})
			return
		}
	}
	writeJSON(w, map[string]any{"ok": false, "file": name, "bytes": len(data), "error": "传输未完成（超时）"})
}

// dbgHTTP 通过本机 HTTP 服务取一个文件，可选 Range。
func (a *App) dbgHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := a.snapshot()
	if cfg == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "配置未加载"})
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		p = "/menu.ipxe"
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", cfg.HTTPPort, p)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if rg := r.URL.Query().Get("range"); rg != "" {
		req.Header.Set("Range", "bytes="+rg)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "url": url, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	head := make([]byte, 16)
	n, _ := io.ReadFull(resp.Body, head)
	head = head[:n]
	writeJSON(w, map[string]any{
		"ok": resp.StatusCode < 400, "url": url, "status": resp.StatusCode,
		"contentLength": resp.ContentLength, "contentRange": resp.Header.Get("Content-Range"),
		"headHex": hex.EncodeToString(head),
	})
}
