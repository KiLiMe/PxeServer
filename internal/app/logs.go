package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"pxeserv/internal/locales"
	"pxeserv/internal/logbook"
)

func logFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "PXE-server.log"
	}
	return filepath.Join(filepath.Dir(exe), "PXE-server.log")
}

// openLogFile 在 exe 同目录追加写 PXE-server.log，超过 2MB 先轮转成 .old。
// 每次启动先写一行会话头，方便在长日志里定位“这次运行”。
// 失败不影响运行（只是没有文件日志）。

func openLogFile() *os.File {
	path := logFilePath()
	if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	// 会话头带完整日期：日志文件跨天时只有 HH:MM:SS 是分不清的。
	_, _ = fmt.Fprintf(f, "======== %s 启动 v%s ========\n", time.Now().Format("2006-01-02 15:04:05"), Version())
	return f
}

// writeLogs 单线程消费 logQ：落盘 I/O 不再占用 a.mu，也不卡界面。

func (a *App) writeLogs() {
	defer close(a.logDone)
	for line := range a.logQ {
		if a.logFile != nil {
			_, _ = fmt.Fprintln(a.logFile, line)
		}
	}
}

// LogLine 是界面与调试接口看到的一行日志：级别供筛选，Text 已带时刻与级别标记。
type LogLine struct {
	Level logbook.Level
	Text  string
}

// Logf 写一条信息级日志。级别只影响界面筛选与落盘标记，不影响格式化内容。
func (a *App) Logf(format string, args ...any) {
	a.logAt(logbook.Info, format, args...)
}

// Warnf 写一条警告：客户端侧失败、端口没绑上这类“能用但要留意”的情况。
func (a *App) Warnf(format string, args ...any) {
	a.logAt(logbook.Warn, format, args...)
}

// Errorf 写一条错误：本次运行已经坏了（服务异常退出、写盘失败）。
func (a *App) Errorf(format string, args ...any) {
	a.logAt(logbook.Error, format, args...)
}

// logAt 是所有日志的唯一落点：内存环形缓冲给界面，logQ 交给写线程落盘。
func (a *App) logAt(level logbook.Level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	now := time.Now()
	line := now.Format("15:04:05") + " " + level.Tag() + " " + msg
	fileLine := now.Format("2006-01-02 15:04:05") + " " + level.Tag() + " " + msg
	a.mu.Lock()
	a.logs = append(a.logs, LogLine{Level: level, Text: line})
	if len(a.logs) > 500 {
		a.logs = a.logs[len(a.logs)-500:]
	}
	a.logSeq++
	q := a.logQ
	if q != nil {
		select {
		case q <- fileLine:
		default: // 队列满就丢文件副本，绝不阻塞服务线程
		}
	}
	a.mu.Unlock()
}

// dialog reports a fatal-ish error both in the log panel and as a native
// message box (port already in use etc. is easy to miss in a log).

// Logs 返回当前缓冲的日志（旧到新）。
func (a *App) Logs() []LogLine {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]LogLine(nil), a.logs...)
}

// LogsSince 在序号变化时返回最新日志及新序号；无变化时返回 ok=false，避免 UI 无谓拷贝。
func (a *App) LogsSince(seq uint64) ([]LogLine, uint64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.logSeq == seq && seq != 0 {
		return nil, seq, false
	}
	return append([]LogLine(nil), a.logs...), a.logSeq, true
}

// LogText 返回纯文本行，供调试接口与导出使用。
func (a *App) LogText() []string {
	lines := a.Logs()
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return out
}

func (a *App) ClearLogs() {
	a.mu.Lock()
	a.logs = nil
	a.logSeq++
	a.mu.Unlock()
}

// SaveLogFile 把当前缓冲的日志导出成 txt（默认文件名带日期），
// 返回空串表示成功，否则是错误信息；用户取消时返回 ""。

func (a *App) SaveLogFile() string {
	lines := a.LogText()
	if a.ctx == nil {
		return locales.UiNotReady()
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: a.window, Title: locales.ExportLogTitle(),
		DefaultPath: "PXE-server-" + time.Now().Format("20060102-1504") + ".log",
		Filters:     []mygo.FileFilter{{Name: locales.FilterLogFiles(), Extensions: []string{"log"}}, {Name: locales.FilterTextFiles(), Extensions: []string{"txt"}}},
	})
	if err != nil {
		return locales.ExportFailed(err.Error())
	}
	if path == "" {
		return ""
	}
	body := strings.Join(lines, "\r\n")
	if body != "" {
		body += "\r\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return locales.ExportFailed(err.Error())
	}
	a.Logf("日志：已导出 %d 行到 %s", len(lines), path)
	return ""
}

// The native view polls Status and Logs on its UI thread.
