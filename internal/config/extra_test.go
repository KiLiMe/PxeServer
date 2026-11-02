package config

import (
	"bytes"
	"testing"
)

// TestParseExtraOptions 覆盖 175 子选项（联想/惠普机型常用）、字符串与大端整数。
func TestParseExtraOptions(t *testing.T) {
	c := &Config{ExtraOptions: "175.6=ip:1.8.1.1; 175.8=ip:192.168.5.1; 93=hex:00 07; 66=str:192.168.5.1; 252=str:http://192.168.5.1/wpad.dat; 51=u32:7200"}
	m, err := c.ParseExtraOptions()
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got, want := m[175], []byte{6, 4, 1, 8, 1, 1, 8, 4, 192, 168, 5, 1}; !bytes.Equal(got, want) {
		t.Fatalf("option 175 = %v, 期望 %v", got, want)
	}
	if got, want := m[93], []byte{0x00, 0x07}; !bytes.Equal(got, want) {
		t.Fatalf("option 93 = %v, 期望 %v", got, want)
	}
	if got := string(m[252]); got != "http://192.168.5.1/wpad.dat" {
		t.Fatalf("option 252 = %q", got)
	}
	if got, want := m[51], []byte{0, 0, 0x1c, 0x20}; !bytes.Equal(got, want) {
		t.Fatalf("option 51 = %v, 期望 %v", got, want)
	}
}

// TestParseExtraOptionsRejects 确认非法输入在保存配置时就会报错。
func TestParseExtraOptionsRejects(t *testing.T) {
	for _, text := range []string{
		"175.6ip:1.8.1.1",    // 缺等号
		"300=ip:1.8.1.1",     // 选项号越界
		"53=u8:3",            // 不允许覆盖消息类型
		"175.6.2=ip:1.8.1.1", // 层级过多
		"175.6=ip:1.8.1.999", // 非法 IP
		"93=hex:zz",          // 非法十六进制
		"51=u16:999999",      // 超出 2 字节
	} {
		c := &Config{ExtraOptions: text}
		if _, err := c.ParseExtraOptions(); err == nil {
			t.Fatalf("%q 本应报错", text)
		}
	}
	if m, err := (&Config{}).ParseExtraOptions(); err != nil || len(m) != 0 {
		t.Fatalf("空配置应返回空表: %v %v", m, err)
	}
}
