package httpd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
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

// ISOPrefix 是 ISO 内文件的 URL 前缀，archiso_http_srv 要用到它。
const ISOPrefix = "/iso/"

// SetISO 挂载/卸载一张 ISO，nil 表示卸载。只挂一张时 URL 里不需要名字，
// /iso/<ISO 内路径> 直接落到它上面（保持旧写法兼容）。
func (s *Server) SetISO(img *isorom.Image) {
	if img == nil {
		s.SetISOs(nil)
		return
	}
	s.SetISOs([]Mount{{Slug: "", Img: img}})
}

// SetISOs 一次挂载多张 ISO。列表里的第一张是“主 ISO”，
// URL 里不带名字时（/iso/<ISO 内路径>）落到它上面。
func (s *Server) SetISOs(list []Mount) {
	s.mu.Lock()
	s.mounts = list
	s.mu.Unlock()
}

// ISOMounts 返回当前挂载的 ISO 名单（调试接口用）。
func (s *Server) ISOMounts() []Mount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Mount(nil), s.mounts...)
}

// serveISO 把 /iso[/<跳名>]/<ISO 内路径> 映射到对应的 ISO 映像里。
// http.ServeContent 自带 Range 支持，archiso 拉 1GB 的 airootfs.sfs 就是靠它的分段读取。
func (s *Server) serveISO(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	mounts := append([]Mount(nil), s.mounts...)
	s.mu.RUnlock()
	if len(mounts) == 0 {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, ISOPrefix)
	head, tail, hasTail := strings.Cut(rest, "/")
	for _, m := range mounts {
		if m.Slug == "" || !strings.EqualFold(m.Slug, head) {
			continue
		}
		if !hasTail {
			http.NotFound(w, r)
			return
		}
		s.serveISOFrom(w, r, m.Img, tail)
		return
	}
	// 第一段不是任何已挂 ISO 的名字：当成“主 ISO 的内部路径”，兼容 /iso/sysresccd/...
	s.serveISOFrom(w, r, mounts[0].Img, rest)
}

func (s *Server) serveISOFrom(w http.ResponseWriter, r *http.Request, img *isorom.Image, name string) {
	if img == nil {
		http.NotFound(w, r)
		return
	}
	rs, _, modTime, err := img.Lookup(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, path.Base(name), modTime, rs)
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

// serveIndex 在 / 上输出一个简单首页：当前共享了哪些启动文件、挂了哪些 ISO。
// 这不是目录列表（子目录仍然 404），只是给人一个“服务是不是活着”的落脚点。
func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Name string
		Size string
		Dir  bool
	}
	var files []entry
	if des, err := os.ReadDir(s.Root); err == nil {
		for _, de := range des {
			if strings.HasPrefix(de.Name(), ".") {
				continue
			}
			e := entry{Name: de.Name(), Dir: de.IsDir()}
			if de.IsDir() {
				e.Size = "目录"
			} else if fi, err := de.Info(); err == nil {
				e.Size = humanSize(fi.Size())
			}
			files = append(files, e)
		}
	}

	s.mu.RLock()
	mounts := append([]Mount(nil), s.mounts...)
	s.mu.RUnlock()

	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><title>PXE 启动服务器</title>")
	b.WriteString("<style>body{font-family:system-ui,'Microsoft YaHei',sans-serif;margin:24px;color:#222}" +
		"h1{font-size:20px}h2{font-size:15px;margin-top:24px;color:#444}" +
		"table{border-collapse:collapse;font-size:14px}td{padding:2px 14px 2px 0}" +
		"a{color:#0a66c2;text-decoration:none}a:hover{text-decoration:underline}" +
		".muted{color:#888;font-size:13px}</style>")
	b.WriteString("<h1>PXE 启动服务器</h1>")
	b.WriteString("<p class=\"muted\">HTTPd 正常。下面是这台服务器共享的启动文件；子目录不列举，需要时用完整路径取。</p>")

	b.WriteString("<h2>启动文件</h2><table>")
	for _, f := range files {
		if f.Dir {
			fmt.Fprintf(&b, "<tr><td>%s/</td><td class=\"muted\">%s</td></tr>", htmlEscape(f.Name), f.Size)
			continue
		}
		fmt.Fprintf(&b, "<tr><td><a href=\"/%s\">/%s</a></td><td class=\"muted\">%s</td></tr>",
			url.PathEscape(f.Name), htmlEscape(f.Name), f.Size)
	}
	b.WriteString("</table>")

	if len(mounts) > 0 {
		b.WriteString("<h2>已挂载的 ISO</h2><table>")
		for _, m := range mounts {
			if m.Img == nil {
				continue
			}
			label := m.Slug
			if label == "" {
				label = path.Base(m.Img.Path())
			}
			fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"muted\">卷标 %s，%s</td></tr>",
				htmlEscape(label), htmlEscape(m.Img.Label()), humanSize(m.Img.Size()))
		}
		b.WriteString("</table>")
	}

	b.WriteString("<p class=\"muted\" style=\"margin-top:28px\">PXE 启动服务器 · TFTP 端口 69 · 本页端口即 HTTP 端口</p>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// humanSize 把字节数换成便于阅读的单位。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")
	return r.Replace(s)
}

// noDirListing 包一层 http.FileSystem：目录请求直接 404，避免把启动目录里的
// 文件清单暴露给浏览器（PXE 客户端只会要具体文件，不需要目录列表）。
type noDirListing struct{ fs http.FileSystem }

func (n noDirListing) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		defer f.Close()
		if _, err := n.fs.Open(strings.TrimSuffix(name, "/") + "/index.html"); err != nil {
			return nil, os.ErrNotExist
		}
	}
	return f, nil
}
