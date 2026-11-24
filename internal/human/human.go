// Package human 把数字格式化成日志与页面里通用的可读形式。
//
// 它跨包共用：TFTP 的传输日志与 HTTP 目录页都要报文件大小，两处各写一份
// 迟早漂移成两种口径。
package human

import "fmt"

// Bytes 把字节数写成 1.4 MB 这种形式：1024 进制，保留一位小数。
func Bytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(1024), 0
	for n/div >= 1024 && exp < 3 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
