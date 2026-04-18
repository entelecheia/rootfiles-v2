package module

import (
	"context"
	"testing"
)

func TestPackagesModule_Name(t *testing.T) {
	if n := NewPackagesModule().Name(); n != "packages" {
		t.Errorf("Name() = %q, want packages", n)
	}
}

func TestPackagesModule_CheckNoPackagesIsSatisfied(t *testing.T) {
	rc := newDryRunRC(t) // AllPackages() returns nil with empty config
	result, err := NewPackagesModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Satisfied {
		t.Errorf("Check with no configured packages should be satisfied, got %+v", result.Changes)
	}
}

func TestPackagesModule_ApplyNoPackagesNoChange(t *testing.T) {
	rc := newDryRunRC(t)
	result, err := NewPackagesModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Changed {
		t.Error("Apply with no packages should not report changes")
	}
}
