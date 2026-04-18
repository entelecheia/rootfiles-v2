package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func init() {
	// Force plain output so assertions can grep substrings without ANSI noise.
	lipgloss.SetColorProfile(0)
}

func TestNewStatusCmd_Basics(t *testing.T) {
	cmd := newStatusCmd()
	if cmd.Use != "status" {
		t.Errorf("Use = %q, want status", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("RunE is nil")
	}
}

func TestStatusCmd_RegisteredOnRoot(t *testing.T) {
	root := NewRootCmd("test", "abc")
	sub, _, err := root.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find status: %v", err)
	}
	if sub.Name() != "status" {
		t.Errorf("subcommand name = %q, want status", sub.Name())
	}
}

func TestStatusCmd_RendersAllSections(t *testing.T) {
	root := NewRootCmd("test", "abc")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"status", "--profile", "minimal"})

	if err := root.Execute(); err != nil {
		t.Fatalf("status execute: %v", err)
	}

	out := buf.String()
	// Every section header must appear so downstream scripts / readers can
	// count on a stable shape. If a section is dropped, this test fails.
	for _, section := range []string{
		"rootfiles status",
		"▸ System",
		"▸ Profile",
		"▸ Modules",
		"▸ GPU Allocations",
		"▸ Tunnel",
		"▸ Users",
	} {
		if !strings.Contains(out, section) {
			t.Errorf("status output missing section %q\n--- got ---\n%s", section, out)
		}
	}
}
