package cli

import (
	"io"
	"strings"
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

// noHostCommands empties PATH so system detection in buildRunContext cannot
// run host binaries such as nvidia-smi.
func noHostCommands(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
}

func TestBuildRunContext_DefaultsToMinimalProfile(t *testing.T) {
	noHostCommands(t)
	// Build a root command so buildRunContext can resolve persistent flags.
	root := NewRootCmd("test", "abc")
	// Find the tunnel status subcommand (simple, no args)
	sub, _, err := root.Find([]string{"tunnel", "status"})
	if err != nil {
		t.Fatalf("find tunnel status: %v", err)
	}
	// Set required flag values to their defaults.
	rc, err := buildRunContext(sub)
	if err != nil {
		t.Fatal(err)
	}
	if rc == nil {
		t.Fatal("buildRunContext returned nil")
	}
	if rc.Config == nil {
		t.Fatal("rc.Config is nil")
	}
	if rc.Runner == nil {
		t.Error("rc.Runner is nil")
	}
	if rc.APT == nil {
		t.Error("rc.APT is nil")
	}
}

// AC1: commands that read or write user or GPU state stop on a config load
// error instead of falling back to /home and an empty profile.
func TestBuildRunContext_LoadErrorStopsStatefulCommands(t *testing.T) {
	cases := []struct {
		name, homeBase string
		args           []string
		want           string
	}{
		{"user add, mistyped profile", "/home", []string{"user", "add", "alice", "--profile", "nope"}, `profile "nope" not found`},
		{"gpu assign, mistyped profile", "/home", []string{"gpu", "assign", "alice", "--gpus", "0", "--profile", "nope"}, `profile "nope" not found`},
		{"user add, relative ROOTFILES_HOME_BASE", "home", []string{"user", "add", "alice"}, "users.home_base"},
		{"gpu assign, relative ROOTFILES_HOME_BASE", "home", []string{"gpu", "assign", "alice", "--gpus", "0"}, "users.home_base"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			noHostCommands(t)
			t.Setenv("ROOTFILES_HOME_BASE", tc.homeBase)
			root := NewRootCmd("test", "abc")
			root.SetArgs(append(tc.args, "--dry-run"))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// AC2: config-free commands such as `tunnel status` still run.
func TestBuildConfigFreeRunContext_ToleratesLoadError(t *testing.T) {
	noHostCommands(t)
	t.Setenv("ROOTFILES_HOME_BASE", "home")
	root := NewRootCmd("test", "abc")
	sub, _, err := root.Find([]string{"tunnel", "status"})
	if err != nil {
		t.Fatalf("find tunnel status: %v", err)
	}
	if err := sub.ParseFlags([]string{"--profile", "nope"}); err != nil {
		t.Fatal(err)
	}
	if _, err := buildRunContext(sub); err == nil {
		t.Fatal("buildRunContext accepted a mistyped profile")
	}
	rc, err := buildConfigFreeRunContext(sub)
	if err != nil || rc == nil || rc.Config == nil {
		t.Fatalf("buildConfigFreeRunContext = %v, %v; want a run context", rc, err)
	}
}

// AC2 wiring: config-free commands run their action without a loadable
// profile. Dry runs keep them off the host.
func TestConfigFreeCommandsRunWithoutProfile(t *testing.T) {
	for _, args := range [][]string{{"tunnel", "restart"}, {"schedule", "disable"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			noHostCommands(t)
			root := NewRootCmd("test", "abc")
			root.SetArgs(append(args, "--profile", "nope", "--dry-run"))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.Execute(); err != nil {
				t.Errorf("err = %v, want the command to run without a profile", err)
			}
		})
	}
}
