package module

import (
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
