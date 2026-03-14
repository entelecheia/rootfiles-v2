package module

import (
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

func TestRegistry_ResolveAll(t *testing.T) {
	reg := NewRegistry()
	cfg := &config.Config{
		Modules: config.ModulesConfig{
			Locale:      config.ModuleToggle{Enabled: true},
			Packages:    config.ModuleToggle{Enabled: true},
			SSH:         config.ModuleToggle{Enabled: true},
			Users:       config.ModuleToggle{Enabled: true},
			Docker:      config.DockerConfig{Enabled: true},
			Nvidia:      config.ModuleToggle{Enabled: true},
			Cloudflared: config.CloudflaredConfig{Enabled: true},
			Storage:     config.StorageConfig{Enabled: true},
			Network:     config.NetworkConfig{Enabled: true},
		},
	}
	modules := reg.Resolve(cfg, nil)
	if len(modules) != 9 {
		t.Errorf("expected 9 modules, got %d", len(modules))
	}
	// Verify order
	expected := []string{"locale", "packages", "ssh", "users", "docker", "nvidia", "cloudflared", "storage", "network"}
	for i, m := range modules {
		if m.Name() != expected[i] {
			t.Errorf("module[%d] = %q, want %q", i, m.Name(), expected[i])
		}
	}
}

func TestRegistry_ResolveFiltered(t *testing.T) {
	reg := NewRegistry()
	cfg := &config.Config{
		Modules: config.ModulesConfig{
			Locale:      config.ModuleToggle{Enabled: true},
			Packages:    config.ModuleToggle{Enabled: true},
			SSH:         config.ModuleToggle{Enabled: true},
			Users:       config.ModuleToggle{Enabled: true},
			Docker:      config.DockerConfig{Enabled: true},
			Cloudflared: config.CloudflaredConfig{Enabled: true},
		},
	}
	modules := reg.Resolve(cfg, []string{"docker", "ssh"})
	if len(modules) != 2 {
		t.Errorf("expected 2 modules, got %d", len(modules))
	}
	if modules[0].Name() != "ssh" {
		t.Errorf("first module = %q, want ssh (should respect order)", modules[0].Name())
	}
	if modules[1].Name() != "docker" {
		t.Errorf("second module = %q, want docker", modules[1].Name())
	}
}

func TestRegistry_ResolveDisabledSkipped(t *testing.T) {
	reg := NewRegistry()
	cfg := &config.Config{
		Modules: config.ModulesConfig{
			Locale:   config.ModuleToggle{Enabled: true},
			Packages: config.ModuleToggle{Enabled: true},
			// Docker NOT enabled
		},
	}
	modules := reg.Resolve(cfg, nil)
	for _, m := range modules {
		if m.Name() == "docker" {
			t.Error("docker should not be in resolved modules (not enabled)")
		}
	}
}

func TestRegistry_ResolveEmpty(t *testing.T) {
	reg := NewRegistry()
	cfg := &config.Config{} // nothing enabled
	modules := reg.Resolve(cfg, nil)
	if len(modules) != 0 {
		t.Errorf("expected 0 modules, got %d", len(modules))
	}
}
