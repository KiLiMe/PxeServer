package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultPath 确认配置路径落在 exe 同目录且是绝对路径
// （用相对路径时提权子进程会读到另一份 config.json）。
func TestDefaultPath(t *testing.T) {
	got := DefaultPath()
	if !filepath.IsAbs(got) {
		t.Fatalf("DefaultPath = %q，期望绝对路径", got)
	}
	if !strings.HasSuffix(got, string(filepath.Separator)+"config.json") {
		t.Fatalf("DefaultPath = %q，期望以 config.json 结尾", got)
	}
}
