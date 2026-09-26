package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkMutating(t *testing.T) {
	root := NewRootCmd("test", "abc")
	for _, path := range [][]string{{"apply"}, {"user", "add"}, {"gpu", "assign"}, {"tunnel", "setup"}} {
		c, _, err := root.Find(path)
		if err != nil || c.Annotations[annotMutates] != "true" {
			t.Errorf("%v should be marked mutating", path)
		}
	}
	for _, path := range [][]string{{"check"}, {"status"}, {"user", "list"}, {"gpu", "list"}, {"tunnel", "status"}} {
		c, _, _ := root.Find(path)
		if c.Annotations[annotMutates] == "true" {
			t.Errorf("%v should not be marked mutating", path)
		}
	}
}

func TestPreflight(t *testing.T) {
	t.Setenv("ROOTFILES_LOCK_FILE", filepath.Join(t.TempDir(), "lock"))
	t.Setenv("ROOTFILES_LOG_FILE", filepath.Join(t.TempDir(), "audit.log"))
	old := geteuid
	t.Cleanup(func() { geteuid = old; heldLock.Release(); heldLock = nil })

	root := NewRootCmd("test", "abc")
	apply, _, _ := root.Find([]string{"apply"})
	status, _, _ := root.Find([]string{"status"})

	geteuid = func() int { return 1000 }
	if err := preflight(apply, nil); err == nil || !strings.Contains(err.Error(), "root") {
		t.Errorf("non-root apply should be refused, got %v", err)
	}
	if err := preflight(status, nil); err != nil {
		t.Errorf("read-only command must not need root: %v", err)
	}
	root.PersistentFlags().Set("dry-run", "true")
	if err := preflight(apply, nil); err != nil {
		t.Errorf("dry-run must not need root: %v", err)
	}
	root.PersistentFlags().Set("dry-run", "false")

	geteuid = func() int { return 0 }
	if err := preflight(apply, nil); err != nil {
		t.Fatalf("root apply: %v", err)
	}
	if err := preflight(apply, nil); err == nil {
		t.Error("second concurrent mutating command should fail on the lock")
	}
}
