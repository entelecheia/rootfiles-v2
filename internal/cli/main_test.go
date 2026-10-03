package cli

import (
	"os"
	"testing"
)

// TestMain pins the home base so config loading in these tests never runs
// home-base detection against the host, and clears the other config
// overrides so a developer's shell cannot fail a load.
func TestMain(m *testing.M) {
	os.Setenv("ROOTFILES_HOME_BASE", "/home")
	// Config selection falls back to the last applied run; keep it off the host.
	stateDir, err := os.MkdirTemp("", "rootfiles-cli-state")
	if err != nil {
		panic(err)
	}
	os.Setenv("ROOTFILES_STATE_DIR", stateDir)
	os.Setenv("ROOTFILES_LOG_FILE", stateDir+"/audit.log")
	for _, k := range []string{"ROOTFILES_PROFILE", "ROOTFILES_TIMEZONE", "ROOTFILES_TUNNEL_TOKEN",
		"ROOTFILES_VLAN_ADDRESS", "ROOTFILES_VLAN_INTERFACE", "ROOTFILES_DOCKER_ROOT", "ROOTFILES_DATA_DIR"} {
		os.Unsetenv(k)
	}
	code := m.Run()
	os.RemoveAll(stateDir)
	os.Exit(code)
}
