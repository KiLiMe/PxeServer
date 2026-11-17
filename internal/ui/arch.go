package ui

import (
	"sort"
	"strconv"
	"strings"

	ui "github.com/egoist/mygo/ui"

	"pxeserv/internal/locales"
)

// addArchMapping 校验输入并把「架构号 → 启动文件」记进草稿配置。
func (u *UI) addArchMapping() {
	key := strings.TrimSpace(u.archKey)
	val := strings.TrimSpace(u.archValue)
	n, err := strconv.Atoi(key)
	if key == "" || err != nil || n < 0 {
		u.setMsg(msgError, locales.ArchKeyInvalid())
		return
	}
	if val == "" {
		u.setMsg(msgError, locales.ArchFileEmpty())
		return
	}
	if u.cfg.BootFileByArch == nil {
		u.cfg.BootFileByArch = map[string]string{}
	}
	u.cfg.BootFileByArch[strconv.Itoa(n)] = val
	u.archKey = ""
	u.archValue = ""
}

func (u *UI) archView(c *ui.Context) {
	section(c, locales.SectionArch(), func() {
		keys := make([]string, 0, len(u.cfg.BootFileByArch))
		for k := range u.cfg.BootFileByArch {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { a, _ := strconv.Atoi(keys[i]); b, _ := strconv.Atoi(keys[j]); return a < b })
		for _, k := range keys {
			v := u.cfg.BootFileByArch[k]
			row(c, func() { ui.Text(c, k+" · "+archName(k)).Height(32) }, func() { ui.TextInput(c, &v).Height(32).OnChange(func() { u.cfg.BootFileByArch[k] = v }) }, func() { ui.Button(c, locales.Remove()).OnClick(func() { delete(u.cfg.BootFileByArch, k) }) })
		}
		row(c, sf(c, locales.CustomArch(), &u.archKey), sf(c, locales.BootFileLabel(), &u.archValue))
		ui.Button(c, locales.AddMapping()).OnClick(u.addArchMapping)
		ui.Text(c, locales.ArchFallback(u.cfg.BootFile))
		ui.Button(c, locales.SaveArch()).OnClick(func() { u.saveAndStart(false) })
	})
}

func archName(k string) string {
	switch k {
	case "0":
		return locales.ArchBiosX86()
	case "6":
		return locales.ArchUefiX86()
	case "7":
		return locales.ArchUefiX64()
	case "9":
		return locales.ArchUefiX64Bc()
	case "10":
		return locales.ArchArm64()
	case "11":
		return locales.ArchArm64Bc()
	}
	return locales.ArchCustom()
}
