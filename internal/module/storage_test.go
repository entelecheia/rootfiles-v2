package module

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

func TestStorageModule_Name(t *testing.T) {
	if n := NewStorageModule().Name(); n != "storage" {
		t.Errorf("Name() = %q, want storage", n)
	}
}

func TestStorageModule_CheckMissingDataDir(t *testing.T) {
	tmp := t.TempDir()
	rc := newDryRunRC(t)
	rc.Config.Modules.Storage = config.StorageConfig{DataDir: filepath.Join(tmp, "data-does-not-exist")}

	result, err := NewStorageModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Satisfied {
		t.Error("Check should not be satisfied when DataDir is missing")
	}
	if len(result.Changes) == 0 {
		t.Error("Check should report at least one change for missing DataDir")
	}
}

func TestStorageModule_CheckDefaultHomeBaseIsSatisfied(t *testing.T) {
	rc := newDryRunRC(t)
	// Default HomeBase="" and no DataDir/Symlinks → nothing to do.
	result, err := NewStorageModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Satisfied {
		t.Errorf("Check with empty storage config should be satisfied, got %+v", result.Changes)
	}
}

func TestStorageModule_ApplyDryRun(t *testing.T) {
	tmp := t.TempDir()
	rc := newDryRunRC(t)
	rc.Config.Modules.Storage = config.StorageConfig{DataDir: filepath.Join(tmp, "data")}
	result, err := NewStorageModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Changed {
		t.Error("Apply with configured DataDir should report Changed=true")
	}
}

func TestIsSymlinkTo(t *testing.T) {
	if isSymlinkTo("/path/that/does/not/exist", "/other") {
		t.Error("isSymlinkTo should return false for non-existent path")
	}
}
