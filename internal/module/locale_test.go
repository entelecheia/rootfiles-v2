package module

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/exec"
)

func TestLocaleModule_Name(t *testing.T) {
	if n := NewLocaleModule().Name(); n != "locale" {
		t.Errorf("Name() = %q, want locale", n)
	}
}

func TestLocaleModule_CheckEmptyConfigIsSatisfied(t *testing.T) {
	rc := newDryRunRC(t) // config has empty Locale and Timezone
	result, err := NewLocaleModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Satisfied {
		t.Errorf("Check with empty locale/timezone should be satisfied, got changes: %+v", result.Changes)
	}
}

// fakeLocaleFS redirects locale/timezone paths into a temp dir.
func fakeLocaleFS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	zi := filepath.Join(dir, "zoneinfo")
	for _, tz := range []string{"UTC", "Asia/Seoul"} {
		os.MkdirAll(filepath.Dir(filepath.Join(zi, tz)), 0755)
		os.WriteFile(filepath.Join(zi, tz), []byte("TZif"), 0644)
	}
	saved := []string{defaultLocalePath, timezonePath, localtimePath, zoneinfoDir, timedatectlBin}
	// Never touch the host clock settings: force the no-systemd fallback.
	timedatectlBin = filepath.Join(dir, "no-timedatectl")
	defaultLocalePath = filepath.Join(dir, "locale")
	timezonePath = filepath.Join(dir, "timezone")
	localtimePath = filepath.Join(dir, "localtime")
	zoneinfoDir = zi
	t.Cleanup(func() {
		defaultLocalePath, timezonePath, localtimePath, zoneinfoDir, timedatectlBin = saved[0], saved[1], saved[2], saved[3], saved[4]
	})
	return dir
}

func TestLocaleModule_TimezoneFromLocaltimeSymlink(t *testing.T) {
	dir := fakeLocaleFS(t)
	// No /etc/timezone (as on stock Ubuntu 24.04), only the symlink.
	os.Symlink(filepath.Join(dir, "zoneinfo", "Asia/Seoul"), localtimePath)
	os.WriteFile(defaultLocalePath, []byte("LANG=en_US.UTF-8\n"), 0644)

	rc := newDryRunRC(t)
	rc.Config.Locale = "en_US.UTF-8"
	rc.Config.Timezone = "Asia/Seoul"
	res, err := NewLocaleModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Satisfied {
		t.Errorf("Check should be satisfied, got %+v", res.Changes)
	}
}

func TestLocaleModule_ApplyWithoutSystemd(t *testing.T) {
	dir := fakeLocaleFS(t)
	os.WriteFile(defaultLocalePath, []byte("LANG=C\nLC_ALL=C\nLANGUAGE=en\n"), 0644)
	os.Symlink(filepath.Join(dir, "zoneinfo", "UTC"), localtimePath)

	rc := newRealRC(t)
	rc.Config.Timezone = "Asia/Seoul"
	res, err := NewLocaleModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Apply should report a timezone change")
	}
	if got := currentTimezone(rc); got != "Asia/Seoul" {
		t.Errorf("timezone = %q, want Asia/Seoul", got)
	}

	res, err = NewLocaleModule().Apply(context.Background(), rc)
	if err != nil || res.Changed {
		t.Errorf("second Apply should be a no-op, got %+v, %v", res, err)
	}
}

func TestLocaleModule_UnknownTimezone(t *testing.T) {
	fakeLocaleFS(t)
	rc := newRealRC(t)
	// Never install packages on the test host.
	rc.APT = exec.NewAPT(newDryRunRC(t).Runner)
	rc.Config.Timezone = "Mars/Olympus"
	if _, err := NewLocaleModule().Apply(context.Background(), rc); err == nil {
		t.Error("Apply should reject an unknown timezone")
	}
}

func TestSetDefaultLocale(t *testing.T) {
	fakeLocaleFS(t)
	os.WriteFile(defaultLocalePath, []byte("LANG=C\nLC_ALL=C\nLANGUAGE=en\n"), 0644)
	rc := newRealRC(t)
	if err := setDefaultLocale(rc, "ko_KR.UTF-8"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(defaultLocalePath)
	s := string(data)
	if !strings.Contains(s, "LANG=ko_KR.UTF-8") || strings.Contains(s, "LC_ALL") || !strings.Contains(s, "LANGUAGE=en") {
		t.Errorf("unexpected locale file:\n%s", s)
	}
	if currentLocale(rc) != "ko_KR.UTF-8" {
		t.Errorf("currentLocale = %q", currentLocale(rc))
	}
}
