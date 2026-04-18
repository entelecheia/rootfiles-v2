package module

import (
	"context"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

func TestNetworkModule_Name(t *testing.T) {
	if n := NewNetworkModule().Name(); n != "network" {
		t.Errorf("Name() = %q, want network", n)
	}
}

func TestNetworkModule_CheckDisabledIsSatisfied(t *testing.T) {
	rc := newDryRunRC(t)
	rc.Config.Modules.Network = config.NetworkConfig{UFW: false}
	result, err := NewNetworkModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Satisfied {
		t.Errorf("Check with UFW disabled should be satisfied, got %+v", result.Changes)
	}
}

func TestNetworkModule_ApplyDisabledNoChange(t *testing.T) {
	rc := newDryRunRC(t)
	rc.Config.Modules.Network = config.NetworkConfig{UFW: false}
	result, err := NewNetworkModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Changed {
		t.Error("Apply with UFW disabled should not report changes")
	}
}
