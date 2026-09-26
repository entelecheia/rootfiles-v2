package module

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// fakePasswd points the lockout guard at a temp passwd file whose accounts
// live under dir. keyed lists accounts that get a non-empty authorized_keys.
func fakePasswd(t *testing.T, keyed ...string) {
	t.Helper()
	dir := t.TempDir()
	accounts := []struct {
		name string
		uid  int
	}{{"root", 0}, {"daemon", 1}, {"alice", 1000}}
	var b strings.Builder
	for _, a := range accounts {
		home := filepath.Join(dir, a.name)
		os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		fmt.Fprintf(&b, "%s:x:%d:%d::%s:/bin/bash\n", a.name, a.uid, a.uid, home)
		for _, k := range keyed {
			if k == a.name {
				os.WriteFile(filepath.Join(home, ".ssh", "authorized_keys"), []byte("ssh-ed25519 AAAA test\n"), 0600)
			}
		}
	}
	p := filepath.Join(dir, "passwd")
	os.WriteFile(p, []byte(b.String()), 0644)
	old := passwdPath
	passwdPath = p
	t.Cleanup(func() { passwdPath = old })
}

func TestSSHModule_LockoutGuard(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.SSHConfig
		keyed   []string
		pending []string
		wantErr bool
	}{
		{"password auth kept", config.SSHConfig{DisablePasswordAuth: false, DisableRootLogin: true}, nil, nil, false},
		{"no keys anywhere", config.SSHConfig{DisablePasswordAuth: true}, nil, nil, true},
		{"root key, root login allowed", config.SSHConfig{DisablePasswordAuth: true}, []string{"root"}, nil, false},
		{"root key only, root login disabled", config.SSHConfig{DisablePasswordAuth: true, DisableRootLogin: true}, []string{"root"}, nil, true},
		{"user key", config.SSHConfig{DisablePasswordAuth: true, DisableRootLogin: true}, []string{"alice"}, nil, false},
		{"system account key ignored", config.SSHConfig{DisablePasswordAuth: true, DisableRootLogin: true}, []string{"daemon"}, nil, true},
		{"declared account with key", config.SSHConfig{DisablePasswordAuth: true, DisableRootLogin: true}, nil, []string{"bob"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakePasswd(t, c.keyed...)
			rc := newDryRunRC(t)
			rc.Config.SSH = c.cfg
			rc.Config.Modules.Users.Enabled = true
			for _, n := range c.pending {
				rc.Config.Users.Accounts = append(rc.Config.Users.Accounts, config.AccountConfig{Name: n, SSHPubkeys: []string{"ssh-ed25519 AAAA k"}})
			}
			err := NewSSHModule().lockoutGuard(rc)
			if (err != nil) != c.wantErr {
				t.Errorf("lockoutGuard() err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestSSHModule_ApplyRefusesLockoutWithoutForce(t *testing.T) {
	fakePasswd(t)
	dropIn := filepath.Join(t.TempDir(), "00-rootfiles.conf")
	old := sshdDropInPath
	sshdDropInPath = dropIn
	t.Cleanup(func() { sshdDropInPath = old })

	rc := newDryRunRC(t)
	rc.Config.SSH = config.SSHConfig{DisablePasswordAuth: true, DisableRootLogin: true}
	if _, err := NewSSHModule().Apply(context.Background(), rc); err == nil {
		t.Fatal("Apply should refuse to disable password auth with no key login")
	}

	check, _ := NewSSHModule().Check(context.Background(), rc)
	if check.Satisfied || !strings.Contains(check.Changes[0].Description, "blocked") {
		t.Errorf("Check should flag the blocked change, got %+v", check.Changes)
	}

	rc.Force = true
	if _, err := NewSSHModule().Apply(context.Background(), rc); err != nil {
		t.Fatalf("Apply with Force: %v", err)
	}
}

func TestSSHModule_ApplyNoopWhenUnchanged(t *testing.T) {
	dropIn := filepath.Join(t.TempDir(), "00-rootfiles.conf")
	old := sshdDropInPath
	sshdDropInPath = dropIn
	t.Cleanup(func() { sshdDropInPath = old })

	m := NewSSHModule()
	cfg := config.SSHConfig{DisableRootLogin: true}
	os.WriteFile(dropIn, []byte(m.buildConfig(cfg)), 0644)

	rc := newDryRunRC(t)
	rc.Config.SSH = cfg
	res, err := m.Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Changed {
		t.Error("Apply should be a no-op when the drop-in already matches")
	}
}

func TestPortLine(t *testing.T) {
	if portLine("# x\nPermitRootLogin no\nPort 2222\n") != "Port 2222" {
		t.Error("portLine should find Port directive")
	}
	if portLine("# x\n") != "" {
		t.Error("portLine should be empty without Port")
	}
}

func sshCfg(noPassword bool, maxTries int) config.SSHConfig {
	return config.SSHConfig{DisablePasswordAuth: noPassword, MaxAuthTries: maxTries}
}
