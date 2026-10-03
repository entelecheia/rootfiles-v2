package cli

import (
	"os"
	"testing"
)

// TestMain pins the home base so config loading in these tests never runs
// home-base detection against the host.
func TestMain(m *testing.M) {
	os.Setenv("ROOTFILES_HOME_BASE", "/home")
	os.Exit(m.Run())
}
