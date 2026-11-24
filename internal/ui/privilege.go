// 拆分点：特权端口的界面侧动作（一键授权、复制 setcap 命令、授权后原地重启）。
// 引用方向：home.go → 本文件 → app.GrantPrivilegedPorts / app.Restart。
// 真正的判定与提权在 internal/priv，本文件只负责交互与文案。

package ui

import (
	"github.com/egoist/mygo"
	"pxeserv/internal/locales"
)

// grantPrivileges 走 pkexec 让用户在一次系统认证里完成授权：setcap 打在当前可执行文件上，
// 之后普通用户启动就能绑 67/69。当前进程的 CapEff 不会因此改变，所以只把提示切成
// 「重启后生效」，不动 privilegedOK——否则下一句「启动」就会撞回 bind 失败。
func (u *UI) grantPrivileges() {
	u.async(func() result {
		if msg := u.app.GrantPrivilegedPorts(); msg != "" {
			return failed(msg)
		}
		return notice(locales.PrivilegedGranted(), func() { u.privilegedGranted = true })
	})
}

// copySetcapHint 把手工命令放进剪贴板：没装 polkit（SSH、精简桌面）时
// 「一键授权」不可用，用户至少要能一键拿到命令去终端里贴。
func (u *UI) copySetcapHint() {
	mygo.Clipboard.WriteText(u.setcapHint())
	u.setMsg(msgSuccess, locales.PrivilegedCopyCmdDone())
}

// restartApp 先存配置再替换进程：重启会丢掉界面上没保存的编辑，
// 而 setcap 只有下一次 exec 才生效。成功后进程已被新镜像替换，不会返回。
func (u *UI) restartApp() {
	cfg := u.cfg.Clone()
	u.async(func() result {
		if msg := u.app.SaveConfig(*cfg); msg != "" {
			return failed(msg)
		}
		if err := u.app.Restart(); err != nil {
			return failed(locales.PrivilegedRestartFailed(err.Error()))
		}
		return done(nil)
	})
}

// setcapHint 返回给用户直接抄的一条命令：路径取当前可执行文件，
// 取不到时写成占位，用户至少知道该填什么。
func (u *UI) setcapHint() string {
	exe := u.app.ExecutablePath()
	if exe == "" {
		exe = "<pxeserver 的路径>"
	}
	return "sudo setcap 'cap_net_bind_service=+ep' " + exe
}
