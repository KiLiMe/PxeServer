//go:build linux

// Package priv 判断当前进程能不能绑定特权端口。
// Linux 上 1024 以下的端口需要 CAP_NET_BIND_SERVICE，普通用户默认没有；
// 界面与命令行据此提前提示，不必等服务绑定失败才发现。
package priv

import (
	"os"
	"strconv"
	"strings"
)

// capNetBindService 是 CAP_NET_BIND_SERVICE 在 capability 位图里的位号（见 linux/capability.h）。
const capNetBindService = 10

// CanBindPrivilegedPorts 报告当前进程能否绑定 1024 以下的端口。
// 判定以 /proc/self/status 的 CapEff 为准：root 也走这条路，容器里被降权的
// “假 root”因此不会被误判成能绑；只有读不到 /proc 时才退回按 UID 粗判。
func CanBindPrivilegedPorts() bool {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return os.Geteuid() == 0
	}
	caps, ok := parseCapEff(string(b))
	if !ok {
		return os.Geteuid() == 0
	}
	return hasCap(caps, capNetBindService)
}

// parseCapEff 从 /proc/self/status 里取出 CapEff 的位图。
// 内核绑定端口时看的是 effective 那一份：只有 Permitted/Inheritable 位（例如没设
// AmbientCapabilities 的 setcap 程序被 systemd 拉起）照样绑不上。
func parseCapEff(status string) (uint64, bool) {
	for _, line := range strings.Split(status, "\n") {
		k, v, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(k) != "CapEff" {
			continue
		}
		caps, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
		if err != nil {
			return 0, false
		}
		return caps, true
	}
	return 0, false
}

// hasCap 报告位图里第 bit 位是否有效。
func hasCap(caps uint64, bit int) bool {
	if bit < 0 || bit > 63 {
		return false
	}
	return caps&(1<<uint(bit)) != 0
}
