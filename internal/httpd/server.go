// 拆分点：HTTP 静态服务的生命周期（监听、超时、状态码日志、关闭）。
// 引用方向：本文件 → iso.go / index.go（两个路由处理器），单向。

package httpd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"pxeserv/internal/isorom"
	"pxeserv/internal/logbook"
)

// Server serves the boot file tree over HTTP (needed for wimboot/winpe and for
// iPXE kernels, since TFTP is painfully slow for anything above a few MB).
type Server struct {
	Root   string
	Port   int
	logger logbook.Logger
	srv    *http.Server
	ln     net.Listener

	mu     sync.RWMutex
	mounts []Mount // 非空时可直接按 http://host:port/iso/<跳名>/<ISO 内路径> 读 ISO 内容
}

// Mount 是一张挂在 HTTP 服务上的 ISO。Slug 是 URL 里的第一段（例如
// systemrescue-13-02-amd64），这样不同路径、不同名字的 ISO 可以同时挂载，
// 谁的路径就归谁，互不影响。
type Mount struct {
	Slug string
	Img  *isorom.Image
}

func New(root string, port int, logger logbook.Logger) *Server {
	if logger == nil {
		logger = func(logbook.Level, string, ...any) {}
	}
	s := &Server{Root: root, Port: port, logger: logger}
	fs := http.FileServer(noDirListing{http.Dir(root)})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		switch {
		case strings.HasPrefix(r.URL.Path, ISOPrefix):
			s.serveISO(rec, r)
		case r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			// 根目录给一个人能看的首页（正版 TinyPXE 也是这么做的），
			// 子目录仍然 404，不做可枚举的文件服务器。
			s.serveIndex(rec, r)
		default:
			fs.ServeHTTP(rec, r)
		}
		// 只写路径会让“文件不存在”和“传输成功”在日志里长得一模一样，所以带上状态码。
		s.logf(statusLevel(rec.status), "HTTPd：%s %s -> %d（%s，%s）", r.Method, r.URL.Path, rec.status,
			time.Since(start).Round(time.Millisecond), r.RemoteAddr)
	})
	s.srv = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: h, ReadHeaderTimeout: 10 * time.Second}
	return s
}

// logf 统一出口：级别由调用点标注，落盘与筛选归上层。
func (s *Server) logf(level logbook.Level, format string, args ...any) {
	s.logger(level, format, args...)
}

// statusLevel 把 HTTP 状态码映射成级别：4xx/5xx 值得一看，其余只是进度。
func statusLevel(status int) logbook.Level {
	switch {
	case status >= 500:
		return logbook.Error
	case status >= 400:
		return logbook.Warn
	}
	return logbook.Info
}

// Listen 同步绑定 HTTP 监听端口，使调用方能立即感知端口占用。
func (s *Server) Listen() error {
	if s.ln != nil {
		return nil
	}
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Serve 在已绑定的监听套接字上提供 HTTP 服务，直到 Stop 被调用。
func (s *Server) Serve() error {
	if err := s.Listen(); err != nil {
		return err
	}
	s.logf(logbook.Info, "HTTPd：http://0.0.0.0:%d → %s", s.Port, s.Root)
	err := s.srv.Serve(s.ln)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Start() error {
	return s.Serve()
}

// statusRecorder 记住 http.FileServer 真正写出的状态码，供访问日志使用。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		_ = s.srv.Close()
	}
	if s.ln != nil {
		// 若启动回滚发生在 Serve 之前，http.Server 还没有登记监听器。
		_ = s.ln.Close()
	}
}
