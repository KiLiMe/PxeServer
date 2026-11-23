//go:build linux

package priv

import "testing"

func TestParseCapEff(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   uint64
		ok     bool
	}{
		{
			name:   "root 全 1",
			status: "Name:\tpxeserver\nCapInh:\t0000000000000000\nCapEff:\t000001ffffffffff\nCapBnd:\t000001ffffffffff\n",
			want:   0x1ffffffffff,
			ok:     true,
		},
		{
			name:   "普通用户全 0",
			status: "Name:\tpxeserver\nCapInh:\t0000000000000000\nCapEff:\t0000000000000000\nCapBnd:\t000001ffffffffff\n",
			want:   0,
			ok:     true,
		},
		{
			name:   "只有 setcap 给的那一位",
			status: "CapEff:\t0000000000000400\n",
			want:   1 << 10,
			ok:     true,
		},
		{
			name:   "没有 CapEff 行",
			status: "Name:\tpxeserver\nCapPrm:\t0000000000000400\n",
			want:   0,
			ok:     false,
		},
		{
			name:   "值不是十六进制",
			status: "CapEff:\tnot-a-number\n",
			want:   0,
			ok:     false,
		},
	}
	for _, c := range cases {
		got, ok := parseCapEff(c.status)
		if got != c.want || ok != c.ok {
			t.Errorf("%s：得到 (0x%x, %v)，应为 (0x%x, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestHasCap(t *testing.T) {
	cases := []struct {
		name string
		caps uint64
		bit  int
		want bool
	}{
		{"有 CAP_NET_BIND_SERVICE", 1 << 10, 10, true},
		{"只有 CAP_NET_RAW", 1 << 13, 10, false},
		{"全 0", 0, 10, false},
		{"位号越界", ^uint64(0), 64, false},
		{"负位号", ^uint64(0), -1, false},
	}
	for _, c := range cases {
		if got := hasCap(c.caps, c.bit); got != c.want {
			t.Errorf("%s：得到 %v，应为 %v", c.name, got, c.want)
		}
	}
}

func TestGrantRejectsEmptyExe(t *testing.T) {
	if err := Grant("  "); err == nil {
		t.Fatal("空路径应当直接报错，而不是去跑 pkexec")
	}
}
