package app

import (
	"context"
	"os"
	"sync"

	"github.com/egoist/mygo"

	"pxeserv/internal/config"
	"pxeserv/internal/dhcp"
	"pxeserv/internal/firewall"
	"pxeserv/internal/httpd"
	"pxeserv/internal/locales"
	"pxeserv/internal/netif"
	"pxeserv/internal/tftp"
)

type App struct {
	ctx    context.Context
	window *mygo.Window

	mu      sync.Mutex
	cfgPath string
	cfg     *config.Config

	dhcpSrv *dhcp.Server
	tftpSrv *tftp.Server
	httpSrv *httpd.Server
	running bool
	debug   *debugAPI // 本地调试接口（127.0.0.1 随机端口）

	logs    []LogLine     // ring buffer, newest last
	logSeq  uint64        // 递增序号，供界面判断是否需要重新拷贝日志
	logFile *os.File      // 日志同时落盘，便于事后排障
	logQ    chan string   // 待落盘的日志行（异步写，不阻塞 UI 线程）
	logDone chan struct{} // 关闭 logQ 后由写线程关闭
}

// Status 在每次启停时推给界面，运行期间由界面轮询。

type Status struct {
	Running  bool         `json:"running"`
	Proxy    bool         `json:"proxy"`
	DHCP     bool         `json:"dhcp"`
	TFTP     bool         `json:"tftp"`
	HTTP     bool         `json:"http"`
	ServerIP string       `json:"serverIp"`
	Leases   []dhcp.Lease `json:"leases"`
	// DebugAddr 是本次运行中调试接口的实际地址（如 127.0.0.1:54321），
	// 端口随机、仅本机；未启用或未启动时为空。
	DebugAddr string `json:"debugAddr"`
}

func NewApp() *App {
	a := &App{cfgPath: config.DefaultPath(), cfg: config.Default(), logQ: make(chan string, 1024), logDone: make(chan struct{})}
	a.logFile = openLogFile()
	go a.writeLogs()
	return a
}

// AttachWindow 记下主窗口，弹框时作为父窗口；由 main 在界面创建后调用。
func (a *App) AttachWindow(win *mygo.Window) { a.window = win }

// SetLanguage 切换界面语言：tag 为 BCP 47 标签（如 "en"），空串表示跟随系统。
// 选中的语言由 mygo 自己记住，配置里的 language 只是让我们开机时能重新应用。
func (a *App) SetLanguage(tag string) { mygo.App.SetLocale(tag) }

// NewInMemory 返回不读写磁盘的 App：配置用给定值，日志只留在内存。
// 界面层的测试用它构造依赖，不必碰真实配置文件。
func NewInMemory(cfg *config.Config) *App {
	return &App{cfg: cfg}
}

// legacyRuntimeFiles 是运行态曾经散落在 exe 同目录的几个小文件，
// 现在并入 config.json 的 runtime 段，启动时迁移后删掉。

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	migrateLegacyRuntimeFiles(a.cfgPath)
	// 上次可能是被强杀/崩溃：runtime 里的调试地址已经失效，先清空再由
	// startDebugAPI 写真实值，避免界面显示一个连不上的端口。
	_ = config.SetRuntime(a.cfgPath, func(r *config.Runtime) { r.DebugAddr = "" })
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		a.Warnf("配置：%v（已改用默认配置）", err)
		cfg = config.Default()
		_ = cfg.Normalize()
		_, _ = cfg.Save(a.cfgPath)
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.SetLanguage(cfg.Language)
	a.Logf("就绪：%s", readySummary(cfg, ifaceName))
	a.startDebugAPI()
	if hint := firewall.Hint(); hint != "" {
		a.Warnf("%s", hint)
	}
	if cfg.AutoStart {
		if msg := a.StartServices(); msg != "" {
			a.Errorf("自动启动失败：%s", msg)
		}
	}
}

func (a *App) Shutdown(ctx context.Context) {
	a.StopServices()
	a.stopDebugAPI()
	a.mu.Lock()
	q := a.logQ
	a.logQ = nil
	a.mu.Unlock()
	if q != nil {
		close(q)
		<-a.logDone // 等写线程把缓冲刷完
	}
	a.mu.Lock()
	if a.logFile != nil {
		_ = a.logFile.Close()
		a.logFile = nil
	}
	a.mu.Unlock()
}

// Logf appends to the ring buffer, pushes the line to the UI and queues it for
// disk. The in-memory/UI copy keeps only HH:MM:SS; the file line carries the full
// date so a log that spans days stays readable.

func (a *App) DetectIP() string { return config.DetectIP() }

// AppVersion 是开发构建的版本号占位：发版由 mygo build 把 mygo.json 的 version
// 写进 exe 资源与 mygo.packageVersion，界面用 Version() 读回。

var AppVersion = "dev"

// Version 返回界面显示的版本号：发版构建取 exe 里写死的版本，go run 等开发构建回落为 AppVersion。

func Version() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return AppVersion
}

// FirewallState 返回防火墙放行状态：on / off / unknown。

func (a *App) ListInterfaces() []netif.Iface {
	return netif.List()
}

// readySummary 生成启动时的「就绪」行：本机地址（带网卡名）+ 启动目录。
// next_server 与本机相同时不重复写出，否则那半句永远是冗余的。
func readySummary(cfg *config.Config, ifaceOf func(string) string) string {
	host := cfg.ServerIP
	if name := ifaceOf(cfg.ServerIP); name != "" {
		host += "（" + name + "）"
	}
	if cfg.NextServer != "" && cfg.NextServer != cfg.ServerIP {
		return host + "，下一台服务器 " + cfg.NextServer + "，启动目录 " + cfg.TFTPRoot
	}
	return host + "，启动目录 " + cfg.TFTPRoot
}

// ifaceName 返回这个地址落在哪块网卡上（排错时最想知道的一条信息）；找不到返回 ""。
func ifaceName(ip string) string {
	if ip == "" {
		return ""
	}
	for _, it := range netif.List() {
		if it.IP == ip {
			return it.Name
		}
	}
	return ""
}

// About 返回“关于”对话框内容：版本、监听端口与使用要点。

func (a *App) About() string {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	httpPort := 0
	root := ""
	if cfg != nil {
		httpPort, root = cfg.HTTPPort, cfg.TFTPRoot
	}
	httpState := locales.Enabled()
	if cfg != nil && !cfg.HTTPEnable {
		httpState = locales.Disabled()
	}
	return locales.About(Version(), httpPort, httpState, root, a.cfgPath)
}
