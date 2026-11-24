package tftp

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pin/tftp/v3"

	"pxeserv/internal/human"
	"pxeserv/internal/logbook"
)

// Server serves read-only TFTP out of Root.
type Server struct {
	Root   string
	logger logbook.Logger
	srv    *tftp.Server
	conn   *net.UDPConn
}

func New(root string, logger logbook.Logger) *Server {
	s := &Server{Root: root, logger: logbook.Or(logger)}
	srv := tftp.NewServer(s.read, nil)
	// 1468 = 1500 MTU - 20 IP - 8 UDP - 4 TFTP, the usual sweet spot.
	// Without a negotiated blksize, UEFI clients crawl through multi-MB .efi files.
	srv.SetBlockSize(1468)
	srv.SetTimeout(5 * time.Second)
	s.srv = srv
	return s
}

// Listen 同步绑定 TFTP 端口。
func (s *Server) Listen() error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 69})
	if err != nil {
		return err
	}
	s.conn = conn
	return nil
}

// Serve 使用 Listen 已绑定的套接字，直到服务停止。
func (s *Server) Serve() error {
	s.logger(logbook.Info, "TFTPd：udp:69 → %s", s.Root)
	return s.srv.Serve(s.conn)
}

func (s *Server) Stop() {
	s.srv.Shutdown()
	if s.conn != nil {
		// 若启动回滚发生在 Serve 之前，第三方服务器还没有登记这个套接字。
		_ = s.conn.Close()
	}
}

// read handles RRQ. Each transfer gets its own ephemeral socket, which pin/tftp
// manages for us - the listening socket only parses the request.
func (s *Server) read(filename string, rf io.ReaderFrom) error {
	// Sanitise: no traversal, no absolute paths.
	clean := filepath.Clean("/" + filepath.ToSlash(filename))
	full := filepath.Join(s.Root, filepath.FromSlash(clean))

	f, err := os.Open(full)
	if err != nil {
		s.logger(logbook.Warn, "TFTPd：GET %s 失败：%v", filename, err)
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		s.logger(logbook.Warn, "TFTPd：GET %s 失败：不是文件", filename)
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
		s.logger(logbook.Warn, "TFTPd：%s 传输中断，已发 %d 字节（%v）", filename, n, err)
		return err
	}
	s.logger(logbook.Info, "TFTPd：GET %s 完成（%s，%.1f s）", filename, human.Bytes(n), time.Since(start).Seconds())
	return nil
}
