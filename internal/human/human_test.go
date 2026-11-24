package human

import "testing"

func TestBytes(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{"零", 0, "0 B"},
		{"不足 1K", 512, "512 B"},
		{"差 1 字节到 1K", 1023, "1023 B"},
		{"整 1K", 1024, "1.0 KB"},
		{"1.5K", 1536, "1.5 KB"},
		{"1M", 1024 * 1024, "1.0 MB"},
		{"引导文件实测值", 1468416, "1.4 MB"},
		{"引导文件", 300 * 1024, "300.0 KB"},
		{"5G", 5 * 1024 * 1024 * 1024, "5.0 GB"},
		{"1T", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"超过 1T", 4 * 1024 * 1024 * 1024 * 1024, "4.0 TB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Bytes(tc.in); got != tc.want {
				t.Errorf("Bytes(%d) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}
