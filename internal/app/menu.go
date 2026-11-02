package app

import (
	"os"
	"path/filepath"

	"pxeserv/internal/locales"
	"pxeserv/internal/shell"
)

func (a *App) menuPath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return filepath.Join(a.cfg.TFTPRoot, a.cfg.MenuFile)
}

// ReadMenuFile returns the current iPXE script, or a template when missing.

func (a *App) ReadMenuFile() string {
	p := a.menuPath()
	b, err := os.ReadFile(p)
	if err != nil {
		// 文件里的路径一律相对（以 / 开头）：iPXE 按“脚本自身 URI”解析，
		// 所以这里不写服务器地址，也不用写 HTTP 端口。
		return "#!ipxe\n" +
			locales.MenuTemplateComment(p) + "\n" +
			"menu PXE Boot Menu\n" +
			"item --gap -- ==== Network boot ====\n" +
			"item local   Boot from local disk\n" +
			"item shell   iPXE shell\n" +
			"choose --default local --timeout 15000 target && goto ${target}\n" +
			":local\nsanboot --no-describe --drive 0x80 || exit 1\n" +
			":shell\nshell\n"
	}
	return string(b)
}

func (a *App) WriteMenuFile(text string) string {
	p := a.menuPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err.Error()
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return err.Error()
	}
	a.Logf("菜单：已保存 %s（%d 字节）", p, len(text))
	return ""
}

// OpenMenuFile 用系统默认程序打开菜单文件（没有关联程序时用记事本），
// 方便直接改菜单而不用先导出再导入。文件不存在时先生成模板。

func (a *App) OpenMenuFile() string {
	p := a.menuPath()
	if _, err := os.Stat(p); err != nil {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err.Error()
		}
		if err := os.WriteFile(p, []byte(a.ReadMenuFile()), 0o644); err != nil {
			return err.Error()
		}
		a.Logf("菜单：已生成 %s", p)
	}
	if err := shell.OpenInEditor(p); err != nil {
		return locales.OpenFailed(err.Error())
	}
	a.Logf("菜单：已用编辑器打开 %s", p)
	return ""
}

// snapshot 返回当前配置指针（只读使用），配置为空时返回 nil。
