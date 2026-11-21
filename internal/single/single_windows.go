//go:build windows

package single

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 单实例靠命名互斥体实现：名字在 Local 命名空间里，同一登录会话内唯一。
// 提权后的副本仍属于同一会话，因此也能被挡住。

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

const swRestore = 9

// Acquire 抢一个命名互斥体（名字进 Local 命名空间，即“本登录会话”，提权后的副本也能被挡住）。
// release 关闭句柄；already 为真表示已有实例持着它，此时 release 是空操作。
func Acquire(name string) (release func(), already bool, err error) {
	namep, err := windows.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return func() {}, false, fmt.Errorf("互斥体名不合法：%w", err)
	}
	h, err := windows.CreateMutex(nil, false, namep)
	already = err == windows.ERROR_ALREADY_EXISTS
	if err != nil && !already {
		return func() {}, false, fmt.Errorf("创建互斥体失败：%w", err)
	}
	if already {
		// 句柄仍然有效，但不属于我们，关掉就退出。
		_ = windows.CloseHandle(h)
		return func() {}, true, nil
	}
	return func() { _ = windows.CloseHandle(h) }, false, nil
}

// Focus 把标题匹配的窗口拉到前台；找不到窗口时返回 false。
func Focus(title string) bool {
	titlep, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlep)))
	if hwnd == 0 {
		return false
	}
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWindow.Call(hwnd)
	return true
}
