package module

import (
	"context"
	"strings"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

func TestSSHModule_Name(t *testing.T) {
	if n := NewSSHModule().Name(); n != "ssh" {
		t.Errorf("Name() = %q, want ssh", n)
	}
}

func TestSSHModule_BuildConfig(t *testing.T) {
	m := NewSSHModule()
	cases := []struct {
		name    string
		cfg     config.SSHConfig
		must    []string
		mustNot []string
	}{
		{
			name: "root login disabled, password disabled, custom port",
			cfg:  config.SSHConfig{DisableRootLogin: true, DisablePasswordAuth: true, Port: 2222},
			must: []string{"PermitRootLogin no", "PasswordAuthentication no", "Port 2222"},
		},
		{
			name:    "all permissive",
			cfg:     config.SSHConfig{},
			mustNot: []string{"PermitRootLogin", "PasswordAuthentication", "Port"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := m.buildConfig(c.cfg)
			for _, s := range c.must {
				if !strings.Contains(got, s) {
					t.Errorf("config missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range c.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("config should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

func TestSSHModule_ApplyDryRun(t *testing.T) {
	rc := newDryRunRC(t)
	rc.Config.SSH = config.SSHConfig{DisableRootLogin: true, Port: 22}
	result, err := NewSSHModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Changed {
		t.Error("Apply should always mark Changed=true (config is rewritten)")
	}
}
