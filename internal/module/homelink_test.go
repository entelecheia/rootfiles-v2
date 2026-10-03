package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #37: a configured base below a symlinked /home is accepted when the link
// target passes the root-only walk; other symlinks stay refused.
func TestCheckHomeBase_SymlinkedHome(t *testing.T) {
	mkdirs := func(t *testing.T, root string, rel ...string) {
		t.Helper()
		for _, r := range rel {
			p := filepath.Join(root, r)
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	cases := []struct {
		name   string
		target string // link target of <root>/home
		setup  func(t *testing.T, root string)
		base   string
		want   string // "" means accepted
	}{
		{"relative target", "data/home", nil, "home/users", ""},
		{"absolute target under the root", "/data/home", nil, "home/users", ""},
		{"restored home parent", "data/home", nil, "home/team", ""},
		{"user-controlled target", "data/home", func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "data"), 0o777); err != nil {
				t.Fatal(err)
			}
		}, "home/users", "points to"},
		{"other symlink", "data/home", func(t *testing.T, root string) {
			if err := os.Symlink("data", filepath.Join(root, "srv")); err != nil {
				t.Fatal(err)
			}
		}, "srv/home", "is a symlink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mkdirs(t, root, ".", "data/home")
			old := homeBaseRoot
			homeBaseRoot = root
			t.Cleanup(func() { homeBaseRoot = old })
			if err := os.Symlink(tc.target, filepath.Join(root, "home")); err != nil {
				t.Fatal(err)
			}
			if tc.setup != nil {
				tc.setup(t, root)
			}
			err := checkHomeBase(filepath.Join(root, tc.base))
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("checkHomeBase: %v, want accepted", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("checkHomeBase error = %v, want %q", err, tc.want)
			}
		})
	}
}
