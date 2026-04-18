package cli

import "testing"

func TestNewBackupCmd_Basics(t *testing.T) {
	cmd := newBackupCmd()
	if cmd.Use != "backup" {
		t.Errorf("Use = %q, want backup", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("RunE is nil")
	}
}

func TestBackupCmd_Flags(t *testing.T) {
	cmd := newBackupCmd()
	for _, tc := range []struct {
		name       string
		defaultVal string
	}{
		{"output", "/raid/backup"},
		{"skip-docker", "false"},
		{"skip-etc", "false"},
	} {
		f := cmd.Flags().Lookup(tc.name)
		if f == nil {
			t.Errorf("missing flag %q", tc.name)
			continue
		}
		if f.DefValue != tc.defaultVal {
			t.Errorf("flag %q default = %q, want %q", tc.name, f.DefValue, tc.defaultVal)
		}
	}
}
