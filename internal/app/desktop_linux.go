package app

import (
	"os"
	"strings"
)

// ConfigureDesktop 让 GTK 文件选择器在 KDE 下通过桌面门户打开。
// GTK 会缓存门户设置，应在 mygo 初始化界面之前调用。
func ConfigureDesktop() {
	if _, set := os.LookupEnv("GTK_USE_PORTAL"); set {
		return
	}
	for desktop := range strings.SplitSeq(os.Getenv("XDG_CURRENT_DESKTOP"), ":") {
		if strings.EqualFold(desktop, "KDE") {
			_ = os.Setenv("GTK_USE_PORTAL", "1")
			return
		}
	}
}
