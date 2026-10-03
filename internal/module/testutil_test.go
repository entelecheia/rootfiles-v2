package module

import (
	"log/slog"
	"os"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
)

// TestMain lets the test user stand in for root as the owner of custom home
// bases, which tests create under t.TempDir().
func TestMain(m *testing.M) {
	homeBaseOwner = uint32(os.Getuid())
	os.Exit(m.Run())
}

// newDryRunRC returns a RunContext with dry-run Runner and APT. Caller may
// mutate rc.Config to set module-specific toggles. Used by per-module tests
// that exercise Check/Apply without touching the real filesystem or apt-get.
func newDryRunRC(t *testing.T) *RunContext {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := exec.NewRunner(true, logger)
	return &RunContext{
		Config: &config.Config{},
		Runner: runner,
		APT:    exec.NewAPT(runner),
		DryRun: true,
	}
}
