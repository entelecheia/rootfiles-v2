package config

import (
	"os"
	"testing"
)

func TestResolveProfile_Base(t *testing.T) {
	cfg, err := resolveProfile("base", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Locale != "en_US.UTF-8" {
		t.Errorf("locale = %q, want en_US.UTF-8", cfg.Locale)
	}
	if cfg.Timezone != "Asia/Seoul" {
		t.Errorf("timezone = %q, want Asia/Seoul", cfg.Timezone)
	}
	if !cfg.Modules.Locale.Enabled {
		t.Error("locale module should be enabled")
	}
	if !cfg.Modules.Packages.Enabled {
		t.Error("packages module should be enabled")
	}
	if !cfg.Modules.SSH.Enabled {
		t.Error("ssh module should be enabled")
	}
	if cfg.Modules.Docker.Enabled {
		t.Error("docker module should not be enabled in base")
	}
	if len(cfg.Packages) == 0 {
		t.Error("base should have packages")
	}
}

func TestResolveProfile_Minimal(t *testing.T) {
	cfg, err := resolveProfile("minimal", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should inherit base packages
	if len(cfg.Packages) == 0 {
		t.Error("minimal should inherit base packages")
	}
	// Should have extra packages
	if len(cfg.PackagesExtra) == 0 {
		t.Error("minimal should have packages_extra")
	}
	// Users should be enabled
	if !cfg.Modules.Users.Enabled {
		t.Error("users module should be enabled in minimal")
	}
	// Cloudflared should be enabled
	if !cfg.Modules.Cloudflared.Enabled {
		t.Error("cloudflared module should be enabled in minimal")
	}
	// Users config
	if cfg.Users.HomeBase != "/home" {
		t.Errorf("home_base = %q, want /home", cfg.Users.HomeBase)
	}
	if cfg.Users.DefaultShell != "/usr/bin/zsh" {
		t.Errorf("default_shell = %q, want /usr/bin/zsh", cfg.Users.DefaultShell)
	}
}

func TestResolveProfile_DGX(t *testing.T) {
	cfg, err := resolveProfile("dgx", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should inherit minimal → base
	if len(cfg.Packages) == 0 {
		t.Error("dgx should inherit base packages")
	}
	// DGX-specific
	if cfg.Users.HomeBase != "/raid/home" {
		t.Errorf("home_base = %q, want /raid/home", cfg.Users.HomeBase)
	}
	if !cfg.Modules.Docker.Enabled {
		t.Error("docker should be enabled in dgx")
	}
	if cfg.Modules.Docker.StorageDir != "/raid/docker" {
		t.Errorf("docker storage_dir = %q, want /raid/docker", cfg.Modules.Docker.StorageDir)
	}
	if !cfg.Modules.Nvidia.Enabled {
		t.Error("nvidia should be enabled in dgx")
	}
	if !cfg.Modules.Cloudflared.PrivateNetwork.Enabled {
		t.Error("cloudflared private_network should be enabled in dgx")
	}
	if cfg.Modules.Cloudflared.PrivateNetwork.Address != "172.16.229.32/32" {
		t.Errorf("vlan address = %q, want 172.16.229.32/32", cfg.Modules.Cloudflared.PrivateNetwork.Address)
	}
	if !cfg.Modules.Storage.Enabled {
		t.Error("storage should be enabled in dgx")
	}
	if !cfg.Modules.Network.Enabled {
		t.Error("network should be enabled in dgx")
	}
	if !cfg.SSH.DisablePasswordAuth {
		t.Error("ssh password auth should be disabled in dgx")
	}
	if !cfg.Modules.Nvidia.GPUAllocation.Enabled {
		t.Error("gpu_allocation should be enabled in dgx")
	}
	if cfg.Modules.Nvidia.GPUAllocation.Method != "both" {
		t.Errorf("gpu_allocation method = %q, want both", cfg.Modules.Nvidia.GPUAllocation.Method)
	}
}

func TestResolveProfile_GPUServer(t *testing.T) {
	cfg, err := resolveProfile("gpu-server", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Users.HomeBase != "/data/home" {
		t.Errorf("home_base = %q, want /data/home", cfg.Users.HomeBase)
	}
	if !cfg.Modules.Docker.Enabled {
		t.Error("docker should be enabled")
	}
	if !cfg.Modules.Nvidia.Enabled {
		t.Error("nvidia should be enabled")
	}
	if !cfg.Modules.Nvidia.GPUAllocation.Enabled {
		t.Error("gpu_allocation should be enabled in gpu-server")
	}
	if cfg.Modules.Nvidia.GPUAllocation.Method != "env" {
		t.Errorf("gpu_allocation method = %q, want env", cfg.Modules.Nvidia.GPUAllocation.Method)
	}
}

func TestResolveProfile_Full(t *testing.T) {
	cfg, err := resolveProfile("full", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Modules.Docker.Enabled {
		t.Error("docker should be enabled in full")
	}
	if !cfg.Modules.Storage.Enabled {
		t.Error("storage should be enabled in full")
	}
	if !cfg.Modules.Network.Enabled {
		t.Error("network should be enabled in full")
	}
}

func TestResolveProfile_NotFound(t *testing.T) {
	_, err := resolveProfile("nonexistent", 0)
	if err == nil {
		t.Error("expected error for nonexistent profile")
	}
}

func TestMergeConfigs_OverlayWins(t *testing.T) {
	base := &Config{
		Locale:   "en_US.UTF-8",
		Users:    UsersConfig{HomeBase: "/home"},
	}
	overlay := &Config{
		Users: UsersConfig{HomeBase: "/raid/home"},
	}
	merged := mergeConfigs(base, overlay)
	if merged.Users.HomeBase != "/raid/home" {
		t.Errorf("home_base = %q, want /raid/home", merged.Users.HomeBase)
	}
	if merged.Locale != "en_US.UTF-8" {
		t.Errorf("locale = %q, want en_US.UTF-8 (from base)", merged.Locale)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	os.Setenv("ROOTFILES_HOME_BASE", "/custom/home")
	os.Setenv("ROOTFILES_TIMEZONE", "UTC")
	os.Setenv("ROOTFILES_TUNNEL_TOKEN", "test-token")
	os.Setenv("ROOTFILES_VLAN_ADDRESS", "10.0.0.1/32")
	defer func() {
		os.Unsetenv("ROOTFILES_HOME_BASE")
		os.Unsetenv("ROOTFILES_TIMEZONE")
		os.Unsetenv("ROOTFILES_TUNNEL_TOKEN")
		os.Unsetenv("ROOTFILES_VLAN_ADDRESS")
	}()

	cfg := &Config{}
	applyEnvOverrides(cfg)

	if cfg.Users.HomeBase != "/custom/home" {
		t.Errorf("home_base = %q, want /custom/home", cfg.Users.HomeBase)
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("timezone = %q, want UTC", cfg.Timezone)
	}
	if cfg.Modules.Cloudflared.TunnelToken != "test-token" {
		t.Errorf("tunnel_token = %q, want test-token", cfg.Modules.Cloudflared.TunnelToken)
	}
	if cfg.Modules.Cloudflared.PrivateNetwork.Address != "10.0.0.1/32" {
		t.Errorf("vlan_address = %q, want 10.0.0.1/32", cfg.Modules.Cloudflared.PrivateNetwork.Address)
	}
}

func TestAllPackages(t *testing.T) {
	cfg := &Config{
		Packages:      []string{"git", "curl"},
		PackagesExtra: []string{"docker-ce", "curl"}, // duplicate curl
	}
	all := cfg.AllPackages()
	if len(all) != 3 { // git, curl, docker-ce (deduped)
		t.Errorf("AllPackages() returned %d, want 3", len(all))
	}
}

func TestIsModuleEnabled(t *testing.T) {
	cfg := &Config{
		Modules: ModulesConfig{
			Locale: ModuleToggle{Enabled: true},
			Docker: DockerConfig{Enabled: false},
			Nvidia: NvidiaConfig{
				Enabled: true,
				GPUAllocation: GPUAllocationConfig{Enabled: true},
			},
		},
	}
	if !cfg.IsModuleEnabled("locale") {
		t.Error("locale should be enabled")
	}
	if cfg.IsModuleEnabled("docker") {
		t.Error("docker should not be enabled")
	}
	if cfg.IsModuleEnabled("unknown") {
		t.Error("unknown module should not be enabled")
	}
	if !cfg.IsModuleEnabled("nvidia") {
		t.Error("nvidia should be enabled")
	}
	if !cfg.IsModuleEnabled("gpu") {
		t.Error("gpu should be enabled when gpu_allocation is enabled")
	}

	// GPU disabled when allocation not enabled
	cfg2 := &Config{
		Modules: ModulesConfig{
			Nvidia: NvidiaConfig{Enabled: true},
		},
	}
	if cfg2.IsModuleEnabled("gpu") {
		t.Error("gpu should not be enabled when gpu_allocation is not enabled")
	}
}
