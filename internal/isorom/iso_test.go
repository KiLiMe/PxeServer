package isorom

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kdomanski/iso9660"
)

func buildTestISO(t *testing.T, label string, files map[string]string) string {
	t.Helper()
	w, err := iso9660.NewWriter()
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Cleanup() }()
	for p, content := range files {
		if err := w.AddFile(bytes.NewBufferString(content), p); err != nil {
			t.Fatalf("AddFile(%s): %v", p, err)
		}
	}
	isoPath := filepath.Join(t.TempDir(), "test.iso")
	f, err := os.Create(isoPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := w.WriteTo(f, label); err != nil {
		_ = f.Close()
		t.Fatalf("WriteTo: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return isoPath
}

func TestOpenLookupAndArchiso(t *testing.T) {
	isoPath := buildTestISO(t, "SYSRESCD", map[string]string{
		"sysresccd/boot/x86_64/vmlinuz":    "kernel-bytes",
		"sysresccd/boot/x86_64/initrd.img": "initrd-bytes",
		"sysresccd/boot/intel_ucode.img":   "ucode-bytes",
		"readme.txt":                       "hello iso",
	})

	img, err := Open(isoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer img.Close()

	if img.Label() != "SYSRESCD" {
		t.Fatalf("Label = %q, want SYSRESCD", img.Label())
	}
	if img.Size() <= 0 {
		t.Fatalf("Size = %d, want > 0", img.Size())
	}

	// 大小写不敏感查找与路径清理
	rs, size, _, err := img.Lookup("/README.TXT")
	if err != nil {
		t.Fatalf("Lookup README.TXT: %v", err)
	}
	got, err := io.ReadAll(rs)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello iso" || size != int64(len("hello iso")) {
		t.Fatalf("content = %q (%d), want hello iso", string(got), size)
	}

	// 目录或不存在的路径应返回错误
	if _, _, _, err := img.Lookup("sysresccd"); err == nil {
		t.Fatalf("Lookup 目录应失败")
	}
	if _, _, _, err := img.Lookup("../../etc/passwd"); err == nil {
		t.Fatalf("Lookup 不存在路径应失败")
	}

	arch, err := img.Archiso()
	if err != nil {
		t.Fatalf("Archiso: %v", err)
	}
	if arch == nil || arch.BaseDir != "sysresccd" || arch.Kernel != "sysresccd/boot/x86_64/vmlinuz" {
		t.Fatalf("Archiso = %+v", arch)
	}
	if len(arch.Initrds) != 2 || arch.Initrds[0] != "sysresccd/boot/intel_ucode.img" || arch.Initrds[1] != "sysresccd/boot/x86_64/initrd.img" {
		t.Fatalf("Archiso.Initrds = %+v", arch.Initrds)
	}
}
