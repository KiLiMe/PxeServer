package firewall

import "pxeserv/internal/config"

// 「上次放行成功过」的标记存在 config.json 的 runtime.firewall_exe 里，
// 非管理员进程查不到防火墙规则时也能直接读到，避免把「已放行」误报成「未知」。

func mark(exe string) error {
	return config.SetRuntime(config.DefaultPath(), func(r *config.Runtime) { r.FirewallExe = exe })
}

func unmark() error {
	return config.SetRuntime(config.DefaultPath(), func(r *config.Runtime) { r.FirewallExe = "" })
}

// markerMatches 判断「本路径的程序上次放行成功过」。
func markerMatches() bool {
	exe, err := program()
	if err != nil {
		return false
	}
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return false
	}
	return cfg.Runtime.FirewallExe == exe
}
