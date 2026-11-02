// Package isorom 把 ISO 文件当只读文件系统来读，用来实现“ISO 原样启动”：
// 内核/initramfs 和里面的 squashfs 都通过 HTTP 按字节范围读出来，既不用解压 ISO，
// 也不用额外占一份磁盘（SystemRescue 的 airootfs.sfs 就有 1.1GB）。
package isorom

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/kdomanski/iso9660"
)

// Image 是一张已打开的 ISO 映像。
type Image struct {
	path string
	f    *os.File
	img  *iso9660.Image
}

// Open 打开 ISO 并解析卷描述符。
func Open(p string) (*Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	im, err := iso9660.OpenImage(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("解析 ISO 失败：%w", err)
	}
	return &Image{path: p, f: f, img: im}, nil
}

// Path 返回 ISO 文件路径。
func (i *Image) Path() string { return i.path }

// Size 返回 ISO 文件大小。
func (i *Image) Size() int64 {
	if st, err := i.f.Stat(); err == nil {
		return st.Size()
	}
	return 0
}

// Label 返回卷标。
func (i *Image) Label() string {
	if s, err := i.img.Label(); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// Close 关闭底层文件。
func (i *Image) Close() error { return i.f.Close() }

// Lookup 按 ISO 内路径查找文件，返回可 Seek 的读取器和修改时间。
// 用 SectionReader 是为了让 http.ServeContent 能处理 Range 请求（断点续传/多连接下载）。
func (i *Image) Lookup(name string) (io.ReadSeeker, int64, time.Time, error) {
	clean := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(name, "\\", "/")), "/")
	if clean == "" || clean == "." {
		return nil, 0, time.Time{}, os.ErrNotExist
	}
	cur, err := i.find(clean)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	if cur.IsDir() {
		return nil, 0, time.Time{}, os.ErrNotExist
	}
	// 库把 extent 偏移包在私有字段里，只能通过 Reader() 拿到已经定位好的 SectionReader；
	// SectionReader 本身实现了 io.ReadSeeker，正好喂给 http.ServeContent 处理 Range。
	sr, ok := cur.Reader().(*io.SectionReader)
	if !ok {
		return nil, 0, time.Time{}, errors.New("ISO 条目无法定位")
	}
	return sr, sr.Size(), cur.ModTime(), nil
}

// find 沿路径逐级下钻（ISO9660 不区分大小写，这里也按忽略大小写匹配）。
func (i *Image) find(rel string) (*iso9660.File, error) {
	root, err := i.img.RootDir()
	if err != nil {
		return nil, err
	}
	cur := root
	for _, want := range strings.Split(strings.Trim(rel, "/"), "/") {
		if want == "" {
			continue
		}
		if !cur.IsDir() {
			return nil, os.ErrNotExist
		}
		children, err := cur.GetChildren()
		if err != nil {
			return nil, err
		}
		var next *iso9660.File
		for _, c := range children {
			if strings.EqualFold(c.Name(), want) {
				next = c
				break
			}
		}
		if next == nil {
			return nil, os.ErrNotExist
		}
		cur = next
	}
	return cur, nil
}

// Archiso 描述一张 ArchISO 系（SystemRescue、Arch Linux、CachyOS…）映像的启动要素。
type Archiso struct {
	BaseDir string   // 例如 sysresccd，对应内核参数 archisobasedir
	Kernel  string   // ISO 内路径，例如 sysresccd/boot/x86_64/vmlinuz
	Initrds []string // ISO 内路径，按加载顺序
}

// Archiso 在根目录下找 boot/<arch>/vmlinuz，据此判断是不是 ArchISO 结构。
// 返回 nil 表示这张 ISO 不是 ArchISO 系，需要按发行版另配启动参数。
func (i *Image) Archiso() (*Archiso, error) {
	root, err := i.img.RootDir()
	if err != nil {
		return nil, err
	}
	dirs, err := root.GetChildren()
	if err != nil {
		return nil, err
	}
	var found *Archiso
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		for _, arch := range []string{"x86_64", "i686", "i386"} {
			kernel := d.Name() + "/boot/" + arch + "/vmlinuz"
			if _, _, _, err := i.Lookup(kernel); err != nil {
				continue
			}
			a := &Archiso{BaseDir: d.Name(), Kernel: kernel}
			for _, ucode := range []string{"intel_ucode.img", "amd_ucode.img"} {
				p := d.Name() + "/boot/" + ucode
				if _, _, _, err := i.Lookup(p); err == nil {
					a.Initrds = append(a.Initrds, p)
				}
			}
			bootDir, err := i.find(d.Name() + "/boot/" + arch)
			if err != nil {
				return nil, err
			}
			files, err := bootDir.GetChildren()
			if err != nil {
				return nil, err
			}
			var imgs []string
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".img") {
					imgs = append(imgs, d.Name()+"/boot/"+arch+"/"+f.Name())
				}
			}
			sort.Strings(imgs)
			a.Initrds = append(a.Initrds, imgs...)
			found = a
			break
		}
		if found != nil {
			break
		}
	}
	return found, nil
}
