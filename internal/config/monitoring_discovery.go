package config

import (
	"errors"
	"path/filepath"
	"strings"
)

// ValidateMonitoringDiscoveryPath validates a file_sd target file and its
// bind-mounted directory. Custom targets live in a dedicated discovery
// directory; only rootfiles' own discovery subtree may be placed under /etc.
func ValidateMonitoringDiscoveryPath(target, dataDir, managedDiscoveryDir string, secretPaths ...string) error {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target || filepath.Ext(target) != ".json" || strings.ContainsAny(target, "\r\n\x00") {
		return errors.New("monitoring targets_file must be a canonical absolute .json path")
	}
	dir := filepath.Dir(target)
	if !filepath.IsAbs(managedDiscoveryDir) || filepath.Clean(managedDiscoveryDir) != managedDiscoveryDir {
		return errors.New("managed monitoring discovery directory must be an absolute canonical path")
	}
	for _, ancestor := range []string{"/", "/etc", "/root", "/home", "/var", "/var/tmp", "/tmp", "/run", "/usr", "/opt", "/data", "/etc/rootfiles", "/etc/rootfiles/monitoring"} {
		if monitoringPathContains(dir, ancestor) {
			return errors.New("monitoring discovery directory cannot be a broad system directory")
		}
	}
	if monitoringPathWithin(dir, "/etc") && !monitoringPathWithin(dir, managedDiscoveryDir) {
		return errors.New("monitoring discovery directory is not allowed under /etc")
	}
	for _, tree := range []string{"/root", "/usr", "/proc", "/sys", "/dev", "/boot", "/home"} {
		if monitoringPathWithin(dir, tree) {
			return errors.New("monitoring discovery directory is inside a sensitive system tree")
		}
	}
	if !monitoringPathWithin(dir, managedDiscoveryDir) && filepath.Base(dir) != "discovery" {
		return errors.New("custom monitoring targets must use a dedicated discovery directory")
	}
	for _, secret := range secretPaths {
		if secret != "" && (filepath.Clean(secret) == target || monitoringPathContains(dir, secret)) {
			return errors.New("monitoring discovery directory cannot contain configured secret files")
		}
	}
	if dataDir != "" {
		dataDir = filepath.Clean(dataDir)
		if monitoringPathContains(dir, dataDir) || monitoringPathContains(dataDir, dir) {
			return errors.New("monitoring discovery directory must be separate from hub data_dir")
		}
	}
	return nil
}

// monitoringPathContains reports whether path is parented by parent, or is
// equal to it. In particular, / contains every absolute path.
func monitoringPathContains(parent, path string) bool {
	return monitoringPathWithin(filepath.Clean(path), filepath.Clean(parent))
}

func monitoringPathWithin(path, parent string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
