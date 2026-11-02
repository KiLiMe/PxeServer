package logbook

import "testing"

func TestLevelTag(t *testing.T) {
	cases := []struct {
		level Level
		tag   string
		sev   string
	}{
		{Info, "I", "信息"},
		{Warn, "W", "警告"},
		{Error, "E", "错误"},
		{Level(42), "I", "信息"},
	}
	for _, c := range cases {
		if got := c.level.Tag(); got != c.tag {
			t.Errorf("Level(%d).Tag() = %q，应为 %q", c.level, got, c.tag)
		}
		if got := c.level.Severity(); got != c.sev {
			t.Errorf("Level(%d).Severity() = %q，应为 %q", c.level, got, c.sev)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Level
		ok   bool
	}{
		{"I", Info, true},
		{"info", Info, true},
		{" W ", Warn, true},
		{"warning", Warn, true},
		{"E", Error, true},
		{"Error", Error, true},
		{"X", Info, false},
		{"", Info, false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Parse(%q) = (%d, %v)，应为 (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
