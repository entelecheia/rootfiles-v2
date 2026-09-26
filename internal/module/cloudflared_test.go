package module

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

// fakeBin puts stub commands (exit 0, append argv to a log) first on PATH
// so tests never touch the host's systemd or network.
func fakeBin(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	for _, n := range names {
		script := "#!/bin/sh\necho \"" + n + " $*\" >> " + log + "\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return log
}

func fakeCloudflaredPaths(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	saved := []string{cloudflaredBinary, cloudflaredUnitPath, cloudflaredEnvPath, vlanNetdevPath, vlanNetworkPath}
	cloudflaredBinary = filepath.Join(dir, "bin", "cloudflared")
	cloudflaredUnitPath = filepath.Join(dir, "systemd", "cloudflared.service")
	cloudflaredEnvPath = filepath.Join(dir, "cloudflared", "tunnel.env")
	vlanNetdevPath = filepath.Join(dir, "network", "10.netdev")
	vlanNetworkPath = filepath.Join(dir, "network", "10.network")
	os.MkdirAll(filepath.Dir(cloudflaredBinary), 0755)
	os.MkdirAll(filepath.Dir(cloudflaredUnitPath), 0755)
	os.WriteFile(cloudflaredBinary, []byte("#!/bin/sh\n"), 0755)
	t.Cleanup(func() {
		cloudflaredBinary, cloudflaredUnitPath, cloudflaredEnvPath, vlanNetdevPath, vlanNetworkPath =
			saved[0], saved[1], saved[2], saved[3], saved[4]
	})
	return dir
}

func TestCloudflared_TokenServiceConvergence(t *testing.T) {
	fakeCloudflaredPaths(t)
	calls := fakeBin(t, "systemctl", "ip")

	rc := newRealRC(t)
	rc.Config.Modules.Cloudflared = config.CloudflaredConfig{Enabled: true, TunnelToken: "tok-1"}
	m := NewCloudflaredModule()

	check, _ := m.Check(context.Background(), rc)
	if check.Satisfied {
		t.Fatal("missing tunnel service should be reported")
	}
	res, err := m.Apply(context.Background(), rc)
	if err != nil || !res.Changed {
		t.Fatalf("Apply = %+v, %v", res, err)
	}

	unit, _ := os.ReadFile(cloudflaredUnitPath)
	if strings.Contains(string(unit), "tok-1") {
		t.Error("token must not appear in the world-readable unit file")
	}
	fi, err := os.Stat(cloudflaredEnvPath)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Errorf("token file mode = %v, %v; want 0600", fi, err)
	}
	if log, _ := os.ReadFile(calls); !strings.Contains(string(log), "systemctl restart cloudflared") {
		t.Errorf("service not restarted, calls:\n%s", log)
	}

	if check, _ := m.Check(context.Background(), rc); !check.Satisfied {
		t.Errorf("after Apply Check should be satisfied, got %+v", check.Changes)
	}

	// Token rotation is detected.
	rc.Config.Modules.Cloudflared.TunnelToken = "tok-2"
	if check, _ := m.Check(context.Background(), rc); check.Satisfied {
		t.Error("token change should be reported")
	}
}

func TestCloudflared_VLANAddressDrift(t *testing.T) {
	fakeCloudflaredPaths(t)
	fakeBin(t, "systemctl", "ip")

	rc := newRealRC(t)
	rc.Config.Modules.Cloudflared = config.CloudflaredConfig{Enabled: true,
		PrivateNetwork: config.PrivateNetworkConfig{Enabled: true, Address: "172.16.0.1/32"}}
	m := NewCloudflaredModule()
	if _, err := m.Apply(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if check, _ := m.Check(context.Background(), rc); !check.Satisfied {
		t.Errorf("Check after Apply = %+v", check.Changes)
	}
	rc.Config.Modules.Cloudflared.PrivateNetwork.Address = "172.16.0.2/32"
	if check, _ := m.Check(context.Background(), rc); check.Satisfied {
		t.Error("VLAN address change should be reported (was only checking file existence)")
	}
}

func TestInstallTunnelService_RejectsBadToken(t *testing.T) {
	fakeCloudflaredPaths(t)
	if _, err := installTunnelService(context.Background(), newRealRC(t), "abc\nExecStart=/bin/evil"); err == nil {
		t.Error("token with newline must be rejected")
	}
}
