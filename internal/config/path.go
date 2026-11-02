package config

import (
	"os"
	"path/filepath"
)

// DefaultPath 返回配置文件路径：exe 同目录下的 config.json。
// 提权后的防火墙子进程 CWD 可能不同，用相对路径会读到另一份 config.json，
// runtime 段就对不上了，所以这里一律转成绝对路径。
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}
