//go:build !windows

package shell

import (
	"os/exec"
	"runtime"
)

// OpenInEditor 用系统默认程序打开文件（Linux 用 xdg-open，macOS 用 open）。
func OpenInEditor(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, path).Start()
}
