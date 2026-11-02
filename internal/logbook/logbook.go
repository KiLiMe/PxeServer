// Package logbook 定义日志级别与统一的日志回调签名。
//
// 内核包（dhcp / tftp / httpd）只通过 logbook.Logger 把日志交给上层，
// 不关心落盘、界面呈现或颜色；级别由写日志的一方标注，界面据此筛选。
package logbook

import "strings"

// Level 是日志级别，越往后越严重。
type Level int

const (
	// Info 是正常进度：启动摘要、一次成功应答、传输完成。
	Info Level = iota
	// Warn 是需要人留意但不致命的异常：端口没绑上、选项被忽略、客户端侧失败。
	Warn
	// Error 是本次运行真的坏了：服务异常退出、配置/日志写盘失败。
	Error
)

// Logger 是内核包向外写日志的统一签名。
type Logger func(level Level, format string, args ...any)

// Tag 返回单字符标记，落盘行与界面共用：I / W / E。
func (l Level) Tag() string {
	switch l {
	case Warn:
		return "W"
	case Error:
		return "E"
	default:
		return "I"
	}
}

// Severity 返回中文说明，仅用于人读的提示（日志正文保持中文不译）。
func (l Level) Severity() string {
	switch l {
	case Warn:
		return "警告"
	case Error:
		return "错误"
	default:
		return "信息"
	}
}

// Parse 解析单字符标记，供调试接口与测试使用；不认识的标记按 Info 处理。
func Parse(s string) (Level, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "I", "INFO":
		return Info, true
	case "W", "WARN", "WARNING":
		return Warn, true
	case "E", "ERROR":
		return Error, true
	}
	return Info, false
}
