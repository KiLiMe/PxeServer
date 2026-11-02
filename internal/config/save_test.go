package config

import (
	"path/filepath"
	"testing"
)

func TestSaveReportsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Default()
	c.ServerIP = "192.168.1.10"

	changed, err := c.Save(path)
	if err != nil {
		t.Fatalf("首次保存失败：%v", err)
	}
	if !changed {
		t.Error("首次保存应报告有变化")
	}

	changed, err = c.Save(path)
	if err != nil {
		t.Fatalf("重复保存失败：%v", err)
	}
	if changed {
		t.Error("内容相同时不应重复写盘")
	}

	c.PoolSize++
	if changed, err = c.Save(path); err != nil {
		t.Fatalf("改配置后保存失败：%v", err)
	} else if !changed {
		t.Error("内容变化后应报告有变化")
	}
}
