// 拆分点：日志级别判定与「同一客户端 5 秒合并」策略。
// 引用方向：全包 → 本文件（s.logf 是唯一出口）。本文件不依赖其它 dhcp 文件。
// 按需阅读：日志刷屏或级别标错，只看这里 + internal/logbook。

package dhcp

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"pxeserv/internal/logbook"
)

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
