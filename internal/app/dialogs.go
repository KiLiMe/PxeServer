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

func (a *App) PickFolder(current string) string {
	if a.ctx == nil {
		return current
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.window, Title: locales.PickBootRootTitle(), DefaultPath: current, Directory: true,
	})
	if err != nil || len(paths) == 0 {
		return current
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
