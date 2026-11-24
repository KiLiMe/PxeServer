// 拆分点：调试接口的日志与自检端点（/logs、/selftest）。

package app

import (
	"fmt"
	"net/http"

	"pxeserv/internal/dhcp"
	"pxeserv/internal/pxe"
)

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
