package cli

import (
	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
	"github.com/entelecheia/rootfiles-v2/internal/state"
)

// appliedFingerprint only claims provenance for a successful apply of the
// same target. Legacy state without a stored digest remains unverified.
func appliedFingerprint(cfg *config.Config, profile, path string) (string, error) {
	last, err := state.Last()
	if err != nil {
		return "", err
	}
	if last == nil || !last.Success || last.ConfigSHA256 == "" {
		return "", nil
	}
	if last.Profile != profile || last.ConfigPath != path {
		return "", nil
	}
	current, err := cfg.Fingerprint()
	if err != nil {
		return "", err
	}
	if current != last.ConfigSHA256 {
		return "", nil
	}
	return current, nil
}

// Reporting still works on hosts with unsupported configuration: capability
// errors are returned as report findings, never as package mutations.
func statusPackageManager(runner *exec.Runner, sys *config.SystemInfo) exec.PackageManager {
	pm, err := exec.NewPackageManager(runner, sys)
	if err != nil {
		if sys != nil && sys.OS == "rocky" {
			return exec.NewRPM(runner)
		}
		return exec.NewAPT(runner)
	}
	return pm
}
