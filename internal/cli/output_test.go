package cli

import (
	"path/filepath"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/state"
)

// recordProfileApply records a successful dgx apply with a --home-base
// override, keeping its resolved config copy as apply would, and returns the
// recorded fingerprint.
func recordProfileApply(t *testing.T, homeBase string) string {
	t.Helper()
	t.Setenv("ROOTFILES_STATE_DIR", t.TempDir())
	t.Setenv("ROOTFILES_HOME_BASE", "") // keep the env override off the copy and its loads
	cfg, err := config.LoadWithHomeBase("dgx", "", &config.SystemInfo{OS: "ubuntu"}, homeBase)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := cfg.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Record(state.Run{Profile: "dgx", ConfigSHA256: fingerprint, Success: true}); err != nil {
		t.Fatal(err)
	}
	if err := saveAppliedConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

// #45 AC1, AC2: reports prefer the kept applied copy when it is the recorded
// run's config and root alone controls it; otherwise they select their target
// as before, and name the recorded target when the copy is in use.
func TestResolveTarget_PrefersAppliedCopy(t *testing.T) {
	fingerprint := recordProfileApply(t, "/srv/h")
	sys := &config.SystemInfo{}
	sub, _, err := NewRootCmd("test", "abc").Find([]string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	snap := state.AppliedConfigPath()

	// The copy wins over re-resolving the recorded profile.
	if p, c, fromCopy := resolveTarget(sub, sys); p != "" || c != snap || !fromCopy {
		t.Errorf("kept copy: got %q %q %v, want the copy", p, c, fromCopy)
	}
	// Reports name the recorded profile and path, never the copy path.
	if p, c := reportedTarget("", snap, true); p != "dgx" || c != "" {
		t.Errorf("reported target: got %q %q, want the recorded profile dgx", p, c)
	}

	// An explicit --profile still wins over a usable copy.
	explicit, _, err := NewRootCmd("test", "abc").Find([]string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if err := explicit.ParseFlags([]string{"--profile", "base"}); err != nil {
		t.Fatal(err)
	}
	if p, c, fromCopy := resolveTarget(explicit, sys); p != "base" || c != "" || fromCopy {
		t.Errorf("explicit --profile: got %q %q %v, want base", p, c, fromCopy)
	}

	// A copy left by another run is not reused; the recorded profile is, as
	// before the copy existed.
	if err := state.Record(state.Run{Profile: "dgx", ConfigSHA256: "another run", Success: true}); err != nil {
		t.Fatal(err)
	}
	if p, c, fromCopy := resolveTarget(sub, sys); p != "dgx" || c != "" || fromCopy {
		t.Errorf("other run's copy: got %q %q %v, want the recorded profile", p, c, fromCopy)
	}

	// A recorded config apply also reuses its copy, and reports its recorded
	// path; without a copy the recorded path is used as before.
	site := filepath.Join(t.TempDir(), "site.yaml")
	writeFile(t, site, "extends: minimal\n")
	if err := state.Record(state.Run{ConfigPath: site, ConfigSHA256: fingerprint, Success: true}); err != nil {
		t.Fatal(err)
	}
	if p, c, fromCopy := resolveTarget(sub, sys); p != "" || c != snap || !fromCopy {
		t.Errorf("config apply with a kept copy: got %q %q %v, want the copy", p, c, fromCopy)
	}
	if p, c := reportedTarget("", snap, true); p != "" || c != site {
		t.Errorf("reported target: got %q %q, want the recorded path", p, c)
	}
	if err := state.SaveAppliedConfig(nil); err != nil {
		t.Fatal(err)
	}
	if p, c, fromCopy := resolveTarget(sub, sys); p != "" || c != site || fromCopy {
		t.Errorf("no kept copy: got %q %q %v, want the recorded path", p, c, fromCopy)
	}

	// Without any record the detection suggestion is used.
	t.Setenv("ROOTFILES_STATE_DIR", t.TempDir())
	if p, c, fromCopy := resolveTarget(sub, sys); p != sys.SuggestProfile() || c != "" || fromCopy {
		t.Errorf("no record: got %q %q %v, want the suggestion", p, c, fromCopy)
	}
}
