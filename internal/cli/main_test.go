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
	for _, k := range []string{"ROOTFILES_PROFILE", "ROOTFILES_TIMEZONE", "ROOTFILES_TUNNEL_TOKEN",
		"ROOTFILES_VLAN_ADDRESS", "ROOTFILES_VLAN_INTERFACE", "ROOTFILES_DOCKER_ROOT", "ROOTFILES_DATA_DIR"} {
		os.Unsetenv(k)
	}
	os.Exit(m.Run())
}
