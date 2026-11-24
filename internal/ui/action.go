// 拆分点：一次操作的抽象（result 结果类型、异步执行、脏检查、保存并启动）。
// 引用方向：各页面文件 → 本文件。本文件不画任何控件。

package ui

import (
	"encoding/json"
	"strconv"

	"pxeserv/internal/config"
	"pxeserv/internal/locales"
)

const (
	msgInfo = iota
	msgSuccess
	msgWarn
	msgError
)

// result 是一次异步任务的产物：级别、提示文案，以及回到界面线程后的收尾动作。
// silent 表示只做收尾、不弹提示（例如进入设置页时的防火墙静默刷新）。
type result struct {
	kind   int
	text   string
	silent bool
	apply  func()
}

func ok(text string, apply func()) result { return result{kind: msgSuccess, text: text, apply: apply} }

func notice(text string, apply func()) result { return result{kind: msgInfo, text: text, apply: apply} }

func failed(text string) result { return result{kind: msgError, text: text} }

func done(apply func()) result { return ok(locales.Done(), apply) }

func silent(apply func()) result { return result{silent: true, apply: apply} }

func (u *UI) async(fn func() result) {
	if u.busy {
		return
	}
	u.busy = true
	u.tasks.Add(1)
	u.Win.Invalidate()
	go func() {
		defer u.tasks.Done()
		res := fn()
		u.Win.Update(func() {
			u.busy = false
			if !res.silent {
				u.setMsg(res.kind, res.text)
			}
			if res.apply != nil {
				res.apply()
			}
			u.Win.Invalidate()
		})
	}()
}

func (u *UI) draftKey() string {
	b, _ := json.Marshal(struct {
		Config            config.Config
		Port, Pool, Lease string
	}{u.cfg, u.port, u.poolSize, u.leaseTime})
	return string(b)
}

func (u *UI) dirty() bool { return u.savedKey != u.draftKey() }

func (u *UI) saveAndStart(start bool) {
	var err error
	if u.cfg.HTTPPort, err = strconv.Atoi(u.port); err != nil {
		u.setMsg(msgError, locales.HttpPortNotInt())
		return
	}
	if u.cfg.PoolSize, err = strconv.Atoi(u.poolSize); err != nil {
		u.setMsg(msgError, locales.PoolSizeNotInt())
		return
	}
	lease, err := strconv.ParseUint(u.leaseTime, 10, 32)
	if err != nil {
		u.setMsg(msgError, locales.LeaseNotNumber())
		return
	}
	u.cfg.LeaseTime = uint32(lease)
	draft := u.cfg.Clone()
	privilegedOK, setcap := u.privilegedOK, u.setcapHint()
	u.async(func() result {
		if saveMsg := u.app.SaveConfig(*draft); saveMsg != "" {
			return failed(saveMsg)
		}
		apply := func() {
			u.adoptConfig()
			u.savedKey = u.draftKey()
			if !u.menuDirty() {
				u.menuLoaded = false
				u.menuPath = ""
				u.menuSaved = ""
			}
			u.ifaces = u.app.ListInterfaces()
			u.syncIface()
			u.Refresh()
		}
		if !start {
			return ok(locales.ConfigSavedApplied(), apply)
		}
		if !privilegedOK {
			// 配置已经存下来了，但服务一定会因为绑不上 67/69 而失败；
			// 这里直接拦下并给出可复制的 setcap 命令，比让用户看日志里的 bind 失败强。
			u.app.Warnf("服务未启动：当前进程没有绑定特权端口的权限（setcap / sudo / AmbientCapabilities）")
			return result{kind: msgError, text: locales.PrivilegedPortBlocked(setcap), apply: apply}
		}
		if startMsg := u.app.StartServices(); startMsg != "" {
			return result{kind: msgError, text: locales.SavedNotStarted(startMsg), apply: apply}
		}
		return ok(locales.SavedStarted(), apply)
	})
}
