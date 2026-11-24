package pxe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
)

// MenuURL 生成 iPXE 菜单的完整 URL（HTTP 启用且主机有效时返回 http://...，否则返回配置里的文件名）。
// 与应答路径（internal/dhcp/reply.go）不同，这里不要求主机是字面 IP：它只用于写菜单与调试输出。
func MenuURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	menu := strings.TrimPrefix(cfg.MenuFile, "/")
	if menu == "" {
		menu = "menu.ipxe"
	}
	if !cfg.HTTPEnable || cfg.HTTPPort <= 0 {
		return menu
	}
	host := cfg.BootHost()
	if host == "" {
		return menu
	}
	if cfg.HTTPPort == 80 {
		return fmt.Sprintf("http://%s/%s", host, menu)
	}
	return fmt.Sprintf("http://%s:%d/%s", host, cfg.HTTPPort, menu)
}

// writeHTTPHint 生成 <启动目录>/httpboot.ipxe。
// 内容只有一行 chain，指向本机 HTTP 上的菜单脚本；端口和 IP 取自当前配置，
// 所以用户改了 HTTP 端口也不需要手改任何菜单文件。
func writeHTTPHint(cfg *config.Config, logf logbook.Logger) {
	if !cfg.HTTPEnable || cfg.HTTPPort <= 0 || cfg.ServerIP == "" {
		return
	}
	menu := MenuURL(cfg)
	body := fmt.Sprintf("#!ipxe\n# 由 PXE 启动服务器自动生成：把菜单换成 HTTP 拉取，后面的 initrd 才会走 HTTP。\nchain %s\n", menu)
	path := filepath.Join(cfg.TFTPRoot, "httpboot.ipxe")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		logf(logbook.Error, "启动目录：写入 %s 失败：%v", path, err)
		return
	}
	// 只写「菜单在哪」：把“为何走 HTTP”那段解释留给文档，不占每次启动的日志。
	logf(logbook.Info, "菜单：%s", menu)
}
