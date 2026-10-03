package module

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskLevel(t *testing.T) {
	for pct, want := range map[float64]string{10: LevelOK, 79.9: LevelOK, 80: LevelWarn, 89: LevelWarn, 90: LevelFail, 100: LevelFail} {
		if got := diskLevel(pct); got != want {
			t.Errorf("diskLevel(%v) = %s, want %s", pct, got, want)
		}
	}
}

func TestDoctorReboot(t *testing.T) {
	dir := t.TempDir()
	saved := []string{rebootRequiredPath, rebootRequiredPkgsPath}
	rebootRequiredPath = filepath.Join(dir, "reboot-required")
	rebootRequiredPkgsPath = filepath.Join(dir, "reboot-required.pkgs")
	t.Cleanup(func() { rebootRequiredPath, rebootRequiredPkgsPath = saved[0], saved[1] })

	if f := doctorReboot(); f.Level != LevelOK {
		t.Errorf("no marker: %+v", f)
	}
	os.WriteFile(rebootRequiredPath, []byte("*** System restart required ***\n"), 0644)
	os.WriteFile(rebootRequiredPkgsPath, []byte("linux-image-6.8\nlinux-base\nlinux-base\n"), 0644)
	f := doctorReboot()
	if f.Level != LevelWarn || !strings.Contains(f.Detail, "linux-image-6.8, linux-base") {
		t.Errorf("unexpected finding: %+v", f)
	}
}

func TestDoctorDisksDedupesFilesystems(t *testing.T) {
	rc := newDryRunRC(t)
	tmp := t.TempDir()
	rc.Config.Users.HomeBase = tmp
	rc.Config.Modules.Storage.DataDir = tmp
	// Same path twice (and possibly the same fs as "/") is reported once.
	if n := len(doctorDisks(rc)); n < 1 || n > 2 {
		t.Errorf("expected / plus at most one distinct fs, got %d", n)
	}
}

// fakeSSHD puts an sshd on PATH whose `-T` prints the given effective config.
func fakeSSHD(t *testing.T, effective string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat <<'EOF'\n" + effective + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(dir, "sshd"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestDoctorSSHWarnsPasswordOnlyAccounts(t *testing.T) {
	fakePasswd(t, "alice")
	withPasswords(t, "alice", "bob")
	rc := newRealRC(t)

	fakeSSHD(t, "permitrootlogin no\npasswordauthentication yes")
	var warn *Finding
	for _, f := range doctorSSH(context.Background(), rc) {
		if f.Check == "ssh password-only" {
			f := f
			warn = &f
		}
	}
	if warn == nil || warn.Level != LevelWarn || !strings.Contains(warn.Detail, "bob") || strings.Contains(warn.Detail, "alice") {
		t.Fatalf("want a warn finding naming bob only, got %+v", warn)
	}

	fakeSSHD(t, "permitrootlogin no\npasswordauthentication no")
	for _, f := range doctorSSH(context.Background(), rc) {
		if f.Check == "ssh password-only" {
			t.Errorf("no password-only warning once password auth is off, got %+v", f)
		}
	}
}
