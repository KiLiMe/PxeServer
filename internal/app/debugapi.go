// 拆分点：本地调试接口的框架（仅 -debug-port 开启、只监听 127.0.0.1、方法/来源守卫）。
// 引用方向：本文件 → 三个 handler 文件，单向。业务 handler 不放这里。

package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"pxeserv/internal/config"
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
