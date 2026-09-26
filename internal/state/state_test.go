package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordAndLast(t *testing.T) {
	t.Setenv("ROOTFILES_STATE_DIR", t.TempDir())
	if r, err := Last(); r != nil || err != nil {
		t.Fatalf("empty state: %v, %v", r, err)
	}
	for _, p := range []string{"minimal", "dgx"} {
		if err := Record(Run{Profile: p, Success: true, FinishedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Last()
	if err != nil || r.Profile != "dgx" {
		t.Fatalf("Last = %+v, %v", r, err)
	}
	hist, _ := os.ReadFile(filepath.Join(Dir(), "history.jsonl"))
	if n := strings.Count(string(hist), "\n"); n != 2 {
		t.Errorf("history has %d lines, want 2", n)
	}
}

func TestAcquireIsExclusive(t *testing.T) {
	t.Setenv("ROOTFILES_LOCK_FILE", filepath.Join(t.TempDir(), "rootfiles.lock"))
	l1, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(); err == nil || !strings.Contains(err.Error(), "another rootfiles") {
		t.Errorf("second Acquire should fail while held, got %v", err)
	}
	l1.Release()
	l2, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	l2.Release()
	var nilLock *Lock
	nilLock.Release() // must not panic
}
