//go:build windows

package shell

import (
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// OpenInEditor 先用系统默认关联程序打开（ShellExecuteW "open"），
// 没有关联的程序（比如 .ipxe）时退回记事本，保证按钮永远有反应。
func OpenInEditor(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	dir, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return err
	}
	const swShownormal = 1
	ret, _, _ := procShellExecuteW.Call(
		uintptr(0),
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(0),
		uintptr(unsafe.Pointer(dir)),
		uintptr(swShownormal),
	)
	if ret > 32 {
		return nil
	}
	return exec.Command("notepad.exe", path).Start()
}
