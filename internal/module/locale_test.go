package module

import (
	"context"
	"testing"
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

func TestLocaleModule_ApplyDryRun(t *testing.T) {
	rc := newDryRunRC(t)
	rc.Config.Locale = "en_US.UTF-8"
	rc.Config.Timezone = "UTC"
	result, err := NewLocaleModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Changed {
		t.Error("Apply with non-empty locale+timezone should report Changed=true")
	}
}
