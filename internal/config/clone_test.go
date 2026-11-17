package config

import "testing"

func TestClone(t *testing.T) {
	cfg := Default()
	cfg.BootFileByArch = map[string]string{"7": "ipxe.efi"}
	cp := cfg.Clone()
	if cp == cfg {
		t.Fatal("Clone 应返回新对象")
	}
	cp.BootFileByArch["7"] = "other.efi"
	cp.BootFileByArch["10"] = "arm64.efi"
	if cfg.BootFileByArch["7"] != "ipxe.efi" || len(cfg.BootFileByArch) != 1 {
		t.Fatalf("修改副本影响了原配置：%v", cfg.BootFileByArch)
	}

	if got := (*Config)(nil).Clone(); got == nil || got.HTTPPort != Default().HTTPPort {
		t.Fatalf("nil 配置应回落到默认配置，实际 %+v", got)
	}
}
