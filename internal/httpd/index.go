// 拆分点：/ 路由——目录索引页与体积格式化，禁止列目录的包装。
// 引用方向：server.go 的路由表 → 本文件。本文件不碰 ISO。

package httpd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

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
