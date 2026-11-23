package app

import (
	"github.com/egoist/mygo"

	"pxeserv/internal/locales"
)

func (a *App) dialog(title, msg string) {
	if a.ctx == nil {
		return
	}
	go func() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window,
			Type: mygo.MessageError, Title: title, Message: msg})
	}()
}

// PickFolder 让用户选启动目录；取消或出错时返回空串，由界面保持原值。
// Linux 上优先用桌面原生的外部选择器（KDE→kdialog，GTK 桌面→zenity），
// 装不上或桌面不认识才回落到内建的 GTK 对话框。
func (a *App) PickFolder(current string) string {
	if a.ctx == nil {
		return ""
	}
	title := locales.PickBootRootTitle()
	if path, handled := pickFolderExternal(title, current); handled {
		return path
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.window, Title: title, DefaultPath: current, Directory: true,
	})
	if err != nil || len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func (a *App) OpenBootFolder() string {
	root := a.GetConfig().TFTPRoot
	if err := mygo.Shell.OpenPath(root); err != nil {
		return err.Error()
	}
	return root
}
