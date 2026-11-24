// 拆分点：调试接口的服务探测端点（直接读 TFTP / HTTP 各取一次，验证链路）。

package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

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
