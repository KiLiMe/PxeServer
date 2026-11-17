//go:build !windows

package single

// 非 Windows 平台不拦多开：永远当作第一个实例，Focus 也找不到窗口。

func Acquire(string) (func(), bool, error) { return func() {}, false, nil }
func Focus(string) bool                    { return false }
