package cli

import (
	"testing"
)

func TestNewTunnelCmd_Subcommands(t *testing.T) {
	cmd := newTunnelCmd()
	if cmd.Use != "tunnel" {
		t.Errorf("Use = %q, want tunnel", cmd.Use)
	}

	want := map[string]bool{
		"install":   false,
		"setup":     false,
		"status":    false,
		"restart":   false,
		"update":    false,
		"uninstall": false,
	}
	for _, sub := range cmd.Commands() {
		name := sub.Name()
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tunnel missing subcommand %q", name)
		}
	}
}

func TestBuildRunContext_DefaultsToMinimalProfile(t *testing.T) {
	// Build a root command so buildRunContext can resolve persistent flags.
	root := NewRootCmd("test", "abc")
	// Find the tunnel status subcommand (simple, no args)
	sub, _, err := root.Find([]string{"tunnel", "status"})
	if err != nil {
		t.Fatalf("find tunnel status: %v", err)
	}
	// Set required flag values to their defaults.
	rc := buildRunContext(sub)
	if rc == nil {
		t.Fatal("buildRunContext returned nil")
	}
	if rc.Config == nil {
		t.Fatal("rc.Config is nil — config.Load failure was silently swallowed without producing a non-nil fallback")
	}
	if rc.Runner == nil {
		t.Error("rc.Runner is nil")
	}
	if rc.APT == nil {
		t.Error("rc.APT is nil")
	}
}
