package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "512M": 512 << 10, "500G": 500 << 20, "2t": 2 << 30, "100K": 100} {
		got, err := parseSize(in)
		if err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "G", "10X", "-5G", "abc"} {
		if _, err := parseSize(bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
}

func TestFindMount(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mounts")
	os.WriteFile(p, []byte(`/dev/sda1 / ext4 rw,relatime 0 0
/dev/md0 /raid xfs rw,noatime,prjquota 0 0
/dev/md1 /raidx ext4 rw 0 0
`), 0644)
	old := procMountsPath
	procMountsPath = p
	t.Cleanup(func() { procMountsPath = old })

	rc := newDryRunRC(t)
	m, err := findMount(rc, "/raid/home/alice")
	if err != nil || m.Point != "/raid" || m.FSType != "xfs" || !m.has("prjquota") {
		t.Errorf("findMount(/raid/home/alice) = %+v, %v", m, err)
	}
	if m, _ := findMount(rc, "/raidx/home"); m.Point != "/raidx" {
		t.Errorf("prefix must match whole path components, got %+v", m)
	}
	if m, _ := findMount(rc, "/home/bob"); m.Point != "/" {
		t.Errorf("fallback to / expected, got %+v", m)
	}
}
