package module

import (
	"context"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

func TestDockerModule_Name(t *testing.T) {
	if n := NewDockerModule().Name(); n != "docker" {
		t.Errorf("Name() = %q, want docker", n)
	}
}

func TestDockerModule_Check(t *testing.T) {
	rc := newDryRunRC(t)
	rc.Config.Modules.Docker = config.DockerConfig{Enabled: true}
	result, err := NewDockerModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestDockerModule_ApplyDryRun(t *testing.T) {
	rc := newDryRunRC(t)
	rc.Config.Modules.Docker = config.DockerConfig{Enabled: true, StorageDir: "/var/lib/docker-custom"}
	result, err := NewDockerModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result == nil {
		t.Fatal("Apply returned nil result")
	}
}
