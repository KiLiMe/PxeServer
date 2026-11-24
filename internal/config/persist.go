// 拆分点：配置文件的读写与运行态（Load / Save / runtime.* 回落）。
// 引用方向：app 层调用；本文件不碰界面，也不做语义校验（那是 normalize.go 的事）。

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// fileMu 串行化对 config.json 的整文件读写：界面保存（Save）和运行态写入
// （SetRuntime，可能来自提权后的子进程）会同时发生，必须互斥。
var fileMu sync.Mutex

// Save 把配置写到 path，并报告内容是否真的有变化。
// 内容一模一样时跳过写盘：界面每次点「保存配置」都会走到这里，
// 反复刷新 mtime 没有意义，日志里也会多出一串重复的“已保存”。
func (c *Config) Save(path string) (bool, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	// runtime 段归 SetRuntime 所有：沿用磁盘上的运行态，避免保存界面配置时
	// 把“调试接口在跑”“防火墙已放行”这类信息抹掉。
	if st, err := ReadRuntime(path); err == nil {
		c.Runtime = st
	}
	return c.saveLocked(path)
}

func (c *Config) saveLocked(path string) (bool, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return false, err
	}
	b = append(b, '\n')
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return false, nil
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// ReadRuntime 只读 runtime 段，不校验、不补默认值：
// 配置本身有错（比如地址池不在同一子网）时 Load 会报错，
// 但“防火墙放行过没有”这类运行态仍然有效。
func ReadRuntime(path string) (Runtime, error) {
	var out Runtime
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return out, err
	}
	return c.Runtime, nil
}

// SetRuntime 只修改 config.json 里的 runtime 段，其余字段原样保留。
// 文件不存在时按默认配置新建；写失败不影响正在跑的服务（只是运行态没落盘）。
func SetRuntime(path string, fn func(*Runtime)) error {
	fileMu.Lock()
	defer fileMu.Unlock()
	c := Default()
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, c); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	fn(&c.Runtime)
	_, err := c.saveLocked(path)
	return err
}
