package cli

import "testing"

func TestNewCheckCmd_Basics(t *testing.T) {
	cmd := newCheckCmd()
	if cmd.Use != "check" {
		t.Errorf("Use = %q, want check", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("RunE is nil")
	}
	if f := cmd.Flags().Lookup("verbose"); f == nil {
		t.Error("check is missing --verbose flag")
	} else if f.Shorthand != "v" {
		t.Errorf("verbose shorthand = %q, want v", f.Shorthand)
	}
}
