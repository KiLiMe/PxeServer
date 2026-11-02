package tftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pin/tftp/v3"

	"pxeserv/internal/logbook"
)

// Server serves read-only TFTP out of Root.
type Server struct {
	Root   string
	logger logbook.Logger
	srv    *tftp.Server
}

func New(root string, logger logbook.Logger) *Server {
	if logger == nil {
		logger = func(logbook.Level, string, ...any) {}
	}
	s := &Server{Root: root, logger: logger}
	srv := tftp.NewServer(s.read, nil)
	// 1468 = 1500 MTU - 20 IP - 8 UDP - 4 TFTP, the usual sweet spot.
	// Without a negotiated blksize, UEFI clients crawl through multi-MB .efi files.
	srv.SetBlockSize(1468)
	srv.SetTimeout(5 * time.Second)
	s.srv = srv
	return s
}

// logf 统一出口：级别由调用点标注，落盘与筛选归上层。
func (s *Server) logf(level logbook.Level, format string, args ...any) {
	s.logger(level, format, args...)
}

// Start blocks until the server stops.
func (s *Server) Start() error {
	s.logf(logbook.Info, "TFTPd：udp:69 → %s", s.Root)
	err := s.srv.ListenAndServe(":69")
	if err != nil && !strings.Contains(err.Error(), "closed") {
		return fmt.Errorf("bind udp:69 (need admin?): %w", err)
	}
	return nil
}

func (s *Server) Stop() { s.srv.Shutdown() }

// read handles RRQ. Each transfer gets its own ephemeral socket, which pin/tftp
// manages for us - the listening socket only parses the request.
func (s *Server) read(filename string, rf io.ReaderFrom) error {
	// Sanitise: no traversal, no absolute paths.
	clean := filepath.Clean("/" + filepath.ToSlash(filename))
	full := filepath.Join(s.Root, filepath.FromSlash(clean))

	f, err := os.Open(full)
	if err != nil {
		s.logf(logbook.Warn, "TFTPd：GET %s 失败：%v", filename, err)
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		s.logf(logbook.Warn, "TFTPd：GET %s 失败：不是文件", filename)
		return fmt.Errorf("%s: not a file", filename)
	}
	// Announce the size so clients can show progress (tsize option).
	if ot, ok := rf.(tftp.OutgoingTransfer); ok {
		ot.SetSize(st.Size())
	}
	// 开始与完成合成一行：小文件（引导文件几十 KB）分两行打纯属噪声，
	// 真正有用的是「谁、多大、花了多久」。失败与中断仍然单独占一行。
	start := time.Now()
	n, err := rf.ReadFrom(f)
	if err != nil {
		s.logf(logbook.Warn, "TFTPd：%s 传输中断，已发 %d 字节（%v）", filename, n, err)
		return err
	}
	s.logf(logbook.Info, "TFTPd：GET %s 完成（%s，%.1f s）", filename, humanSize(int64(n)), time.Since(start).Seconds())
	return nil
}

// humanSize 把字节数写成 1.4 MB 这种形式：日志里没人愿意数七位数字。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	size := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB"} {
		size /= unit
		if size < unit {
			return fmt.Sprintf("%.1f %s", size, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB", size/unit)
}
