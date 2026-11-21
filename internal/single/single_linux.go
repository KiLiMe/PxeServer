//go:build linux

package single

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Linux 没有命名互斥体，单实例改用 flock 锁文件：
//   - 锁由内核在进程退出（含崩溃、被 kill、断电）时释放，不会留下要人手动清理的死锁；
//   - 锁文件本身故意不删——删掉它会让后来的两个进程各锁一个文件，反而失去互斥；
//   - 位置优先 $XDG_RUNTIME_DIR（登录会话私有、tmpfs，注销即清），没有时退回临时目录并按 UID 分开。
//
// 同一登录会话内的第二个实例因为拿不到锁而退出；换用户（runtime dir 不同）互不影响。

// Acquire 抢锁文件上的排他 flock（非阻塞）。
// release 解锁并关闭文件句柄；already 为真表示已有实例持着锁，此时 release 是空操作。
func Acquire(name string) (release func(), already bool, err error) {
	f, err := os.OpenFile(lockPath(name), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return func() {}, false, fmt.Errorf("打开单实例锁失败：%w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return func() {}, true, nil
		}
		return func() {}, false, fmt.Errorf("加单实例锁失败：%w", err)
	}
	// 把 PID 写进锁文件：界面上弹不出提示时，`cat` 一把就知道是谁占着。
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, false, nil
}

// Focus 在 Linux 上做不到：把已有窗口拉到前台要与那个进程通信，Wayland 还要启动方
// 给的 activation token，而这里只剩一个锁文件。调用方据此只提示一句就退出。
func Focus(string) bool { return false }

// lockPath 把互斥体名换成锁文件路径：名字按 Windows 的规矩带空格，文件名里换成分隔符。
func lockPath(name string) string {
	base := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-")) + ".lock"
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return filepath.Join(dir, base)
		}
	}
	return filepath.Join(os.TempDir(), base+"-"+strconv.Itoa(os.Getuid()))
}
