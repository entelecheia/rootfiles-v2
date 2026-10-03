package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSiteIgnoresControllerEnvironmentAndResolvesFiles(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "base.yaml")
	site := filepath.Join(dir, "site.yaml")
	if err := os.WriteFile(parent, []byte("timezone: UTC\nusers:\n  home_base: /home/test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(site, []byte("extends: base.yaml\nlocale: en_US.UTF-8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROOTFILES_HOME_BASE", "/override")
	t.Setenv("ROOTFILES_TIMEZONE", "Asia/Seoul")
	cfg, err := LoadSite(site)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Users.HomeBase != "/home/test" || cfg.Timezone != "UTC" {
		t.Fatalf("env or inheritance changed intent: %+v", cfg)
	}
}

func TestFingerprintIgnoresRuntimeAndSecretButDetectsIntent(t *testing.T) {
	cfg := &Config{Timezone: "UTC"}
	first, err := cfg.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	cfg.System = &SystemInfo{OS: "ubuntu", GPUCount: 8}
	cfg.Extends = "minimal"
	cfg.Modules.Cloudflared.TunnelToken = "secret"
	same, err := cfg.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if same != first {
		t.Fatal("runtime/inline secret affected fingerprint")
	}
	cfg.Timezone = "Asia/Seoul"
	changed, err := cfg.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("intent change did not affect fingerprint")
	}
}
