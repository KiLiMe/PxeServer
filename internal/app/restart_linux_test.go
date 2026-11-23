//go:build linux

package app

import (
	"reflect"
	"testing"
)

func TestWithoutCapture(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "普通参数", args: []string{"pxeserver"}, want: []string{"pxeserver"}},
		{name: "成对写法", args: []string{"pxeserver", "-capture", "/tmp/a.png"}, want: []string{"pxeserver"}},
		{name: "内联写法", args: []string{"pxeserver", "-capture=/tmp/a.png"}, want: []string{"pxeserver"}},
		{name: "夹在中间", args: []string{"pxeserver", "-debug-port", "0", "-capture", "/tmp/a.png"}, want: []string{"pxeserver", "-debug-port", "0"}},
		{name: "双横线", args: []string{"pxeserver", "--capture", "/tmp/a.png"}, want: []string{"pxeserver"}},
		{name: "没有参数", args: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withoutCapture(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("withoutCapture(%q) = %q，想要 %q", tt.args, got, tt.want)
			}
		})
	}
}
