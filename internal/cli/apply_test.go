package cli

import "testing"

func TestNewApplyCmd_Basics(t *testing.T) {
	cmd := newApplyCmd()
	if cmd.Use != "apply" {
		t.Errorf("Use = %q, want apply", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("RunE is nil")
	}
}

func TestApplyCmd_InheritsRootFlags(t *testing.T) {
	root := NewRootCmd("test", "abc")
	apply, _, err := root.Find([]string{"apply"})
	if err != nil {
		t.Fatalf("find apply: %v", err)
	}
	// Apply should inherit persistent flags from root.
	for _, name := range []string{"yes", "dry-run", "profile", "module", "config"} {
		if f := apply.Flags().Lookup(name); f == nil {
			if f = apply.InheritedFlags().Lookup(name); f == nil {
				t.Errorf("apply missing persistent flag %q", name)
			}
		}
	}
}
