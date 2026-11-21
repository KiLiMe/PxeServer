// pxed is the headless core of the PXE server: DHCP/ProxyDHCP + TFTP + HTTP.
// All logging goes through a Logf callback instead of printing directly from internal packages.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"pxeserv/internal/config"
	"pxeserv/internal/logbook"
	"pxeserv/internal/pxe"
)

// Version 由 -ldflags "-X main.Version=..." 注入（release.yml 用），未注入时标记成开发构建。
var Version = "0.0.0-dev"

func main() {
	cfgPath := flag.String("config", "config.json", "path to config.json (created if missing)")
	root := flag.String("root", "", "override boot file root directory")
	noHTTP := flag.Bool("no-http", false, "do not start the HTTP server")
	selfTest := flag.Bool("selftest", false, "run packet encode/decode self test and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("pxed %s\n", Version)
		return
	}

	if *selfTest {
		runSelfTest()
		return
	}

	// 命令行版把级别打成单字母，和界面、落盘一致，grep 起来不费劲。
	logf := func(level logbook.Level, format string, args ...any) {
		fmt.Printf("%s %s "+format+"\n", append([]any{timeNow(), level.Tag()}, args...)...)
	}

	logf(logbook.Info, "pxed v%s 启动", Version)

	cfg, err := loadOrCreate(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "配置：", err)
		os.Exit(1)
	}
	if *root != "" {
		cfg.TFTPRoot = *root
	}
	if *noHTTP {
		cfg.HTTPEnable = false
	}
	logf(logbook.Info, "配置：%s（ProxyDHCP=%v HTTP=%v 根目录=%s）", *cfgPath, cfg.ProxyDHCP, cfg.HTTPEnable, cfg.TFTPRoot)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	services, startErr := pxe.Start(cfg, logf)
	if startErr != nil {
		fmt.Fprintln(os.Stderr, startErr)
		os.Exit(1)
	}
	<-sig
	logf(logbook.Info, "正在停止服务…")
	services.Stop()
}

func loadOrCreate(path string) (*config.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := config.Default()
		if err := cfg.Normalize(); err != nil {
			return nil, err
		}
		if _, err := cfg.Save(path); err != nil {
			return nil, err
		}
		fmt.Println("已生成", path)
		return cfg, nil
	}
	return config.Load(path)
}
