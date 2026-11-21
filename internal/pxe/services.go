// Package pxe 编排桌面端与命令行共用的 PXE 服务。
package pxe

import (
	"fmt"
	"os"

	"pxeserv/internal/config"
	"pxeserv/internal/dhcp"
	"pxeserv/internal/httpd"
	"pxeserv/internal/logbook"
	"pxeserv/internal/tftp"
)

type Stage string

const (
	BootRoot Stage = "启动目录"
	DHCP     Stage = "DHCPd"
	TFTP     Stage = "TFTPd"
	HTTP     Stage = "HTTPd"
)

type StartError struct {
	Stage Stage
	Err   error
}

func (e *StartError) Error() string { return fmt.Sprintf("%s：%v", e.Stage, e.Err) }

type Services struct {
	DHCP *dhcp.Server
	tftp *tftp.Server
	HTTP *httpd.Server
}

// Start 在 DHCP 开始应答前绑定文件服务的端口并准备菜单。
func Start(cfg *config.Config, logf logbook.Logger) (*Services, *StartError) {
	if logf == nil {
		logf = func(logbook.Level, string, ...any) {}
	}
	if err := os.MkdirAll(cfg.TFTPRoot, 0o755); err != nil {
		return nil, &StartError{BootRoot, err}
	}
	services := &Services{DHCP: dhcp.New(cfg, logf), tftp: tftp.New(cfg.TFTPRoot, logf)}
	if cfg.HTTPEnable {
		services.HTTP = httpd.New(cfg.TFTPRoot, cfg.HTTPPort, logf)
		if err := services.HTTP.Listen(); err != nil {
			return nil, &StartError{HTTP, err}
		}
	}
	if err := services.tftp.Listen(); err != nil {
		services.Stop()
		return nil, &StartError{TFTP, err}
	}
	writeHTTPHint(cfg, logf)
	if err := services.DHCP.Start(); err != nil {
		services.Stop()
		return nil, &StartError{DHCP, err}
	}
	go func() {
		if err := services.tftp.Serve(); err != nil {
			logf(logbook.Error, "TFTPd：服务异常退出：%v", err)
		}
	}()
	if services.HTTP != nil {
		go func() {
			if err := services.HTTP.Serve(); err != nil {
				logf(logbook.Error, "HTTPd：服务异常退出：%v", err)
			}
		}()
	}
	return services, nil
}

func (s *Services) Stop() {
	s.DHCP.Stop()
	s.tftp.Stop()
	if s.HTTP != nil {
		s.HTTP.Stop()
	}
}
