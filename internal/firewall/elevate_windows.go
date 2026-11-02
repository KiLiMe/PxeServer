package firewall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 提权后的自己靠这两个参数区分要做的事；结果通过同目录的结果文件回传。
const (
	flagApply  = "-firewall-apply"
	flagRemove = "-firewall-remove"
	resultFile = "firewall-result.txt"
)

func resultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return resultFile
	}
	return filepath.Join(filepath.Dir(exe), resultFile)
}

func writeResult(msg string) {
	_ = os.WriteFile(resultPath(), []byte(msg), 0o644)
}

// elevate 用 UAC 提权重跑自己执行指定规则操作，等结果文件。
func elevate(flag string) error {
	exe, err := program()
	if err != nil {
		return err
	}
	result := resultPath()
	_ = os.Remove(result)

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(flag)
	if err != nil {
		return err
	}
	shellExecute := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	r, _, callErr := shellExecute.Call(0,
		uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)), 0, uintptr(windows.SW_HIDE))
	if int(r) <= 32 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return fmt.Errorf("提权失败（返回码 %d）：%v", int(r), callErr)
		}
		return fmt.Errorf("提权被取消或失败（返回码 %d）", int(r))
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(result); readErr == nil {
			_ = os.Remove(result)
			msg := strings.TrimSpace(string(data))
			if msg == "" || msg == "ok" {
				return nil
			}
			return fmt.Errorf("%s", msg)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("超时：没有等到提权进程的结果")
}
