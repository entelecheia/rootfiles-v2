package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSiteValidatesOptionalAlertmanagerRetention(t *testing.T) {
	if !IsValidPrometheusDuration("1y") {
		t.Fatal("Prometheus retention should continue accepting year units")
	}
	if IsValidAlertmanagerDuration("1y") {
		t.Fatal("Alertmanager retention must reject the unsupported year unit")
	}
	base := "extends: base\nmodules:\n  monitoring:\n    hub:\n      enabled: true\n      alertmanager_retention: %s\nusers:\n  home_base: /home\n"
	for _, value := range []string{"1ns", "1us", "1µs", "1μs", "1ms", "1s", "1m", "1h", "1d", "1w", "1h30m", "240h"} {
		valid := filepath.Join(t.TempDir(), "site.yaml")
		if err := os.WriteFile(valid, []byte(strings.Replace(base, "%s", value, 1)), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadSite(valid)
		if err != nil {
			t.Errorf("LoadSite with Alertmanager-supported duration %q: %v", value, err)
			continue
		}
		if got := cfg.Modules.Monitoring.Hub.AlertmanagerRetention; got != value {
			t.Errorf("alertmanager_retention = %q, want %q", got, value)
		}
	}
	for _, value := range []string{"0s", "-1h", "invalid", "1y", "999999999999999999999h"} {
		invalid := filepath.Join(t.TempDir(), "site.yaml")
		if err := os.WriteFile(invalid, []byte(strings.Replace(base, "%s", value, 1)), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSite(invalid); err == nil || !strings.Contains(err.Error(), "alertmanager_retention") {
			t.Errorf("LoadSite with invalid Alertmanager retention %q returned %v", value, err)
		}
	}
}

func TestValidateMonitoringDiscoveryPathAllowsDefaultAndDedicatedDirs(t *testing.T) {
	defaultHub := (MonitoringHubConfig{}).WithDefaults()
	if err := ValidateMonitoringDiscoveryPath(defaultHub.TargetsFile, defaultHub.DataDir, MonitoringDiscoveryDir); err != nil {
		t.Fatalf("default target path rejected: %v", err)
	}
	for _, target := range []string{
		"/etc/rootfiles/monitoring/discovery/nested/targets.json",
		"/srv/monitoring/discovery/targets.json",
		"/var/lib/foo/discovery/targets.json",
		filepath.Join(t.TempDir(), "discovery", "targets.json"),
	} {
		if err := ValidateMonitoringDiscoveryPath(target, "/data/monitoring", MonitoringDiscoveryDir); err != nil {
			t.Errorf("dedicated target path %q rejected: %v", target, err)
		}
	}
}

func TestValidateMonitoringDiscoveryPathRejectsSensitiveEtcTrees(t *testing.T) {
	for _, target := range []string{
		"/etc/ssh/ssh_host_ed25519_key",
		"/etc/ssh/discovery/targets.json",
		"/etc/sudoers.d/other",
		"/etc/sudoers.d/discovery/targets.json",
		"/etc/rootfiles/monitoring/targets.json",
	} {
		if err := ValidateMonitoringDiscoveryPath(target, "/data/monitoring", MonitoringDiscoveryDir); err == nil {
			t.Errorf("sensitive target path %q was accepted", target)
		}
	}
}

func TestValidateMonitoringDiscoveryPathRejectsSensitiveTreesAndOverlaps(t *testing.T) {
	for _, target := range []string{
		"/root/operator/discovery/targets.json",
		"/usr/local/share/discovery/targets.json",
		"/proc/rootfiles/discovery/targets.json",
		"/sys/rootfiles/discovery/targets.json",
		"/dev/rootfiles/discovery/targets.json",
		"/boot/rootfiles/discovery/targets.json",
		"/home/operator/discovery/targets.json",
		"/srv/monitoring/targets.json",
		"/data/monitoring/discovery/targets.json",
		"/srv/monitoring/discovery/targets.yaml",
	} {
		if err := ValidateMonitoringDiscoveryPath(target, "/data/monitoring", MonitoringDiscoveryDir, "/srv/secrets/receiver.yaml"); err == nil {
			t.Errorf("unsafe target path %q was accepted", target)
		}
	}
	if err := ValidateMonitoringDiscoveryPath("/srv/monitoring/discovery/targets.json", "/data/monitoring", MonitoringDiscoveryDir, "/srv/monitoring/discovery/receiver.yaml"); err == nil {
		t.Fatal("discovery directory containing a secret was accepted")
	}
}
