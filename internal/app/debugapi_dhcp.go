// 拆分点：调试接口的报文解码端点（把抓到的 DHCP 字节流按 Parse 结果回显）。

package app

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"pxeserv/internal/dhcp"
)

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
