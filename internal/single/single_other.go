//go:build !windows && !linux

package single

// 其它平台（macOS/BSD）不拦多开：永远当作第一个实例，Focus 也找不到窗口。
// Linux 有自己的实现（single_linux.go，flock 锁文件），不用这里的占位。

func Acquire(string) (func(), bool, error) { return func() {}, false, nil }
func Focus(string) bool                    { return false }
