//go:build linux

package priv

import "testing"

func TestParseGetcap(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{name: "单个能力", out: "/opt/pxeserver/pxeserver cap_net_bind_service=ep\n", want: true},
		{name: "能力列表", out: "/opt/x cap_net_admin,cap_net_bind_service=eip\n", want: true},
		{name: "只有 permitted 不算", out: "/opt/x cap_net_bind_service=p\n", want: false},
		{name: "别的能力", out: "/opt/x cap_net_raw=ep\n", want: false},
		{name: "前后有空白", out: "\n/opt/x  cap_net_bind_service=ep\n\n", want: true},
		{name: "文件没有能力", out: "", want: false},
		{name: "报错输出", out: "getcap: /opt/x: No such file or directory\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseGetcap(tt.out); got != tt.want {
				t.Fatalf("parseGetcap(%q) = %v，想要 %v", tt.out, got, tt.want)
			}
		})
	}
}

func TestGrantRejectsEmptyPath(t *testing.T) {
	if err := Grant("  "); err == nil {
		t.Fatal("空路径应当报错")
	}
}
