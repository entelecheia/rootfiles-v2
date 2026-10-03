package config

import (
	"path/filepath"
	"testing"
)

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
