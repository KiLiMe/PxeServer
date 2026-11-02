package httpd

import (
	"testing"

	"pxeserv/internal/logbook"
)

func TestStatusLevel(t *testing.T) {
	cases := []struct {
		status int
		want   logbook.Level
	}{
		{200, logbook.Info},
		{206, logbook.Info},
		{304, logbook.Info},
		{404, logbook.Warn},
		{403, logbook.Warn},
		{500, logbook.Error},
	}
	for _, c := range cases {
		if got := statusLevel(c.status); got != c.want {
			t.Errorf("statusLevel(%d) = %v，应为 %v", c.status, got, c.want)
		}
	}
}
