//go:build linux

package app

import "testing"

func TestDesktopPicker(t *testing.T) {
	tests := []struct {
		name    string
		desktop string
		want    string
	}{
		{"KDE", "KDE", "kdialog"},
		{"LXQt", "LXQt", "kdialog"},
		{"DDE", "Deepin", "kdialog"},
		{"GNOME 带发行版前缀", "ubuntu:GNOME", "zenity"},
		{"XFCE", "XFCE", "zenity"},
		{"Cinnamon", "X-Cinnamon", "zenity"},
		{"大小写与空格", " kde ", "kdialog"},
		{"不认识的桌面", "sway", ""},
		{"空值", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desktopPicker(tt.desktop); got != tt.want {
				t.Fatalf("desktopPicker(%q) = %q，想要 %q", tt.desktop, got, tt.want)
			}
		})
	}
}

func TestPickerArgs(t *testing.T) {
	tests := []struct {
		name  string
		bin   string
		title string
		start string
		want  []string
	}{
		{"kdialog 带起点", "kdialog", "选目录", "/srv/pxe", []string{"--title", "选目录", "--getexistingdirectory", "/srv/pxe"}},
		{"kdialog 无起点", "kdialog", "选目录", "", []string{"--title", "选目录", "--getexistingdirectory"}},
		{"zenity 带起点", "zenity", "选目录", "/srv/pxe", []string{"--file-selection", "--directory", "--title=选目录", "--filename=/srv/pxe"}},
		{"zenity 无起点", "zenity", "选目录", "", []string{"--file-selection", "--directory", "--title=选目录"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickerArgs(tt.bin, tt.title, tt.start)
			if len(got) != len(tt.want) {
				t.Fatalf("pickerArgs(%q) = %q，想要 %q", tt.bin, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("pickerArgs(%q) = %q，想要 %q", tt.bin, got, tt.want)
				}
			}
		})
	}
}
