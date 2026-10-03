package config

import (
	"strings"
	"testing"
)

func TestResolveRockyDistroMatrix(t *testing.T) {
	for _, version := range []string{"8.9", "8.10", "9.0", "9.5"} {
		d := ResolveDistro(&SystemInfo{OS: "rocky", Version: version})
		if !d.Supported || d.PackageBackend != "dnf" || d.AdminGroup != "wheel" || d.SSHService != "sshd" {
			t.Errorf("ResolveDistro(Rocky %s) = %+v", version, d)
		}
	}
	for _, version := range []string{"8.8", "8.11", "10.0"} {
		d := ResolveDistro(&SystemInfo{OS: "rocky", Version: version})
		if d.Supported {
			t.Errorf("ResolveDistro(Rocky %s) unexpectedly supported", version)
		}
	}
}

func TestValidateCapabilitiesRejectsUnsupportedRockyBeforeApply(t *testing.T) {
	cfg := &Config{}
	err := ValidateCapabilities(cfg, &SystemInfo{OS: "rocky", Version: "9.5"}, []string{"locale", "network"})
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("ValidateCapabilities error = %v, want network capability error", err)
	}
}

func TestValidateCapabilitiesRejectsRockyAPTAndEPELAssumptions(t *testing.T) {
	system := &SystemInfo{OS: "rocky", Version: "9.5"}
	cfg := &Config{}
	cfg.Modules.System.Enabled = true
	cfg.Modules.System.AptMirror = "https://mirror.example/rocky"
	if err := ValidateCapabilities(cfg, system, []string{"system"}); err == nil || !strings.Contains(err.Error(), "apt_mirror") {
		t.Errorf("APT mirror error = %v", err)
	}
	cfg.Modules.System.AptMirror = ""
	cfg.Modules.Security.Enabled = true
	cfg.Modules.Security.Fail2ban = true
	if err := ValidateCapabilities(cfg, system, []string{"security"}); err == nil || !strings.Contains(err.Error(), "EPEL") {
		t.Errorf("fail2ban error = %v", err)
	}
}

func TestValidateCapabilitiesPreservesUbuntuModuleSet(t *testing.T) {
	cfg := &Config{}
	if err := ValidateCapabilities(cfg, &SystemInfo{OS: "ubuntu", Version: "24.04"}, []string{"docker", "network", "monitoring"}); err != nil {
		t.Fatalf("ValidateCapabilities: %v", err)
	}
}
