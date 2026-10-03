package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/entelecheia/rootfiles-v2/internal/config"
	"testing"
)

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

func TestCheck_JSONOutputAndExitCode(t *testing.T) {
	previous := detectSystem
	detectSystem = func() (*config.SystemInfo, error) { return &config.SystemInfo{OS: "ubuntu", Version: "22.04"}, nil }
	t.Cleanup(func() { detectSystem = previous })
	t.Setenv("ROOTFILES_STATE_DIR", t.TempDir())
	root := NewRootCmd("test", "abc")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"check", "--profile", "base", "-o", "json"})
	err := root.Execute()

	var report checkReport
	if jerr := json.Unmarshal(buf.Bytes(), &report); jerr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jerr, buf.String())
	}
	if report.Profile != "base" || len(report.Modules) != 3 {
		t.Errorf("unexpected report: %+v", report)
	}
	var exitErr *ExitError
	switch {
	case report.Satisfied && err != nil:
		t.Errorf("satisfied check should exit 0, got %v", err)
	case !report.Satisfied && (!errors.As(err, &exitErr) || exitErr.Code != exitDrift):
		t.Errorf("drift should exit %d, got %v", exitDrift, err)
	}
}

func TestCheck_RejectsUnknownOutput(t *testing.T) {
	root := NewRootCmd("test", "abc")
	root.SetArgs([]string{"check", "--profile", "base", "-o", "yaml"})
	if err := root.Execute(); err == nil {
		t.Error("unknown output format should fail")
	}
}
