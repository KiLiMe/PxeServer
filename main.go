package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo"
	mygoUI "github.com/egoist/mygo/ui"

	"pxeserv/internal/app"
	"pxeserv/internal/firewall"
	"pxeserv/internal/locales"
	"pxeserv/internal/single"
	"pxeserv/internal/ui"
)

//go:embed build/appicon.png
var appIcon []byte

func main() {
	// 提权分支（-firewall-apply/-firewall-remove）要在 mygo 初始化之前处理并直接退出。
	if firewall.HandleCLI() {
		return
	}
	// 单实例：已有窗口在跑时把它拉到前台就退出，避免两份都去抢 67/69/80 端口、
	// 互相覆盖同一个 config.json。
	release, already, err := single.Acquire("Local\\PXE Server")
	if err != nil {
		fmt.Fprintln(os.Stderr, "单实例检查：", err)
	} else {
		defer release()
		if already {
			if !single.Focus(locales.AppTitle()) {
				fmt.Fprintln(os.Stderr, "已有一个 PXE 启动服务器在运行。")
			}
			return
		}
	}
	capture := flag.String("capture", "", "把窗口截图写到指定 PNG 后退出")
	debugPort := flag.Int("debug-port", 0, "本机调试接口端口（仅 127.0.0.1；0 或不传表示不开）")
	flag.Parse()
	a := app.NewApp()
	a.SetDebugPort(*debugPort)
	done := make(chan struct{})
	var startup sync.WaitGroup
	var screen *ui.UI
	mygo.App.WhenReady(func() {
		u := ui.New(a)
		screen = u
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: locales.AppTitle(), Width: 1000, Height: 800,
			MinWidth: 760, MinHeight: 600, StateKey: "pxeserver",
			Content: mygoUI.View(u.View),
		})
		a.AttachWindow(win)
		u.Win = win
		if err := win.SetIcon(appIcon); err != nil {
			a.Logf("窗口图标：%v", err)
		}
		win.OnClosed(func() { close(done) })
		// 读配置、起服务可能卡在磁盘或端口上，别占界面线程。
		startup.Add(1)
		go func() {
			a.Startup(context.Background())
			startup.Done()
			win.Update(func() { u.Load(); u.Refresh() })
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					win.Update(func() { u.Refresh() })
				}
			}
		}()
		if *capture != "" {
			go func() {
				timer := time.NewTimer(2 * time.Second)
				defer timer.Stop()
				select {
				case <-done:
					return
				case <-timer.C:
				}
				data, err := win.CapturePage()
				if err == nil {
					err = os.WriteFile(*capture, data, 0o644)
				}
				if err != nil {
					fmt.Fprintln(os.Stderr, "截图失败：", err)
				}
				win.Close()
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	startup.Wait()
	if screen != nil {
		screen.Wait()
	}
	a.Shutdown(context.Background())
}
