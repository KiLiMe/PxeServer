package app

import (
	"os"
	"path/filepath"
	"strings"

	"pxeserv/internal/config"
	"pxeserv/internal/locales"
)

var legacyRuntimeFiles = []string{"debug-port.txt", "firewall-ok.txt"}

// migrateLegacyRuntimeFiles 把旧文件的运行态并进 config.json，再删掉旧文件。
// firewall-ok.txt 里存的是上次放行成功的 exe 路径，丢了会让非管理员进程
// 把「已放行」误报成「未知」，所以先导入再删。

func migrateLegacyRuntimeFiles(cfgPath string) {
	dir := filepath.Dir(cfgPath)
	markPath := filepath.Join(dir, "firewall-ok.txt")
	if b, err := os.ReadFile(markPath); err == nil {
		if mark := strings.TrimSpace(string(b)); mark != "" {
			_ = config.SetRuntime(cfgPath, func(r *config.Runtime) {
				if r.FirewallExe == "" {
					r.FirewallExe = mark
				}
			})
		}
	}
	for _, name := range legacyRuntimeFiles {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func (a *App) GetConfig() *config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg == nil {
		return nil
	}
	c := *a.cfg
	c.BootFileByArch = make(map[string]string, len(a.cfg.BootFileByArch))
	for k, v := range a.cfg.BootFileByArch {
		c.BootFileByArch[k] = v
	}
	return &c
}

// SaveConfig validates and persists the configuration, returning "" on success.
func (a *App) SaveConfig(c config.Config) string {
	if err := c.Normalize(); err != nil {
		return err.Error()
	}
	a.mu.Lock()
	wasRunning := a.running
	old := a.cfg
	a.cfg = &c
	a.mu.Unlock()

	if wasRunning {
		// 先停旧服务再用新配置起；起不来就回滚到旧配置并恢复旧服务。
		// 此时还没落盘，磁盘上仍是旧配置，不会留下“配置已改、服务全停”的尴尬状态。
		a.StopServices()
		if err := a.StartServices(); err != "" {
			a.mu.Lock()
			a.cfg = old
			a.mu.Unlock()
			if restart := a.StartServices(); restart != "" {
				return locales.RollbackFailed(err, restart)
			}
			return locales.RollbackRestored(err)
		}
	}
	changed, err := c.Save(a.cfgPath)
	if err != nil {
		return err.Error()
	}
	if changed {
		a.Logf("配置：已保存到 %s", a.cfgPath)
	}
	return ""
}

func (a *App) snapshot() *config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}
