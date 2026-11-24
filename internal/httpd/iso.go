// 拆分点：/iso/ 路由——把挂载的 ISO9660 镜像按需读给客户端。
// 引用方向：server.go 的路由表 → 本文件 → internal/isorom（只读视图）。
// 按需阅读：ISO 直连相关的问题只看这里与 internal/isorom。

package httpd

import (
	"net/http"
	"path"
	"strings"

	"pxeserv/internal/isorom"
)

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
