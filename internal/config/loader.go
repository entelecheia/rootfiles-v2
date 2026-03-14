package config

import (
	"embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.yaml
var embeddedProfiles embed.FS

const maxExtendsDepth = 5

// Load resolves a profile by name (or custom path), applies env overrides,
// and attaches system info.
func Load(profileName, customPath string, sysInfo *SystemInfo) (*Config, error) {
	var cfg *Config
	var err error

	if customPath != "" {
		cfg, err = loadFromFile(customPath)
	} else {
		if profileName == "" {
			profileName = "minimal"
		}
		cfg, err = resolveProfile(profileName, 0)
	}
	if err != nil {
		return nil, err
	}

	applyEnvOverrides(cfg)
	cfg.System = sysInfo
	return cfg, nil
}

// AvailableProfiles returns the list of built-in profile names.
func AvailableProfiles() []string {
	return []string{"base", "minimal", "dgx", "gpu-server", "full"}
}

func resolveProfile(name string, depth int) (*Config, error) {
	if depth > maxExtendsDepth {
		return nil, fmt.Errorf("profile extends chain too deep (max %d)", maxExtendsDepth)
	}

	cfg, err := loadEmbeddedProfile(name)
	if err != nil {
		return nil, fmt.Errorf("loading profile %q: %w", name, err)
	}

	if cfg.Extends == "" {
		return cfg, nil
	}

	base, err := resolveProfile(cfg.Extends, depth+1)
	if err != nil {
		return nil, err
	}

	return mergeConfigs(base, cfg), nil
}

func loadEmbeddedProfile(name string) (*Config, error) {
	data, err := embeddedProfiles.ReadFile("profiles/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", name, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing profile %q: %w", name, err)
	}
	return &cfg, nil
}

func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}
	return &cfg, nil
}

// mergeConfigs overlays child onto base. Non-zero child values win.
func mergeConfigs(base, overlay *Config) *Config {
	merged := *base

	// Scalar fields: overlay wins if non-empty
	if overlay.Locale != "" {
		merged.Locale = overlay.Locale
	}
	if overlay.Timezone != "" {
		merged.Timezone = overlay.Timezone
	}

	// Packages: keep base, append extra from overlay
	merged.Packages = base.Packages
	if len(overlay.Packages) > 0 {
		merged.Packages = overlay.Packages
	}
	merged.PackagesExtra = append(base.PackagesExtra, overlay.PackagesExtra...)

	// Modules: overlay wins per-module if explicitly set
	merged.Modules = mergeModules(base.Modules, overlay.Modules)

	// Users: overlay wins field-by-field
	merged.Users = mergeUsers(base.Users, overlay.Users)

	// SSH: overlay wins field-by-field
	merged.SSH = mergeSSH(base.SSH, overlay.SSH)

	// Clear extends (already resolved)
	merged.Extends = ""

	return &merged
}

func mergeModules(base, overlay ModulesConfig) ModulesConfig {
	m := base
	if overlay.Locale.Enabled {
		m.Locale = overlay.Locale
	}
	if overlay.Packages.Enabled {
		m.Packages = overlay.Packages
	}
	if overlay.SSH.Enabled {
		m.SSH = overlay.SSH
	}
	if overlay.Users.Enabled {
		m.Users = overlay.Users
	}
	if overlay.Docker.Enabled {
		m.Docker = overlay.Docker
	}
	if overlay.Nvidia.Enabled {
		m.Nvidia = overlay.Nvidia
	}
	if overlay.Cloudflared.Enabled {
		m.Cloudflared = overlay.Cloudflared
	}
	if overlay.Storage.Enabled {
		m.Storage = overlay.Storage
	}
	if overlay.Network.Enabled {
		m.Network = overlay.Network
	}
	return m
}

func mergeUsers(base, overlay UsersConfig) UsersConfig {
	u := base
	if overlay.HomeBase != "" {
		u.HomeBase = overlay.HomeBase
	}
	if overlay.DefaultShell != "" {
		u.DefaultShell = overlay.DefaultShell
	}
	if len(overlay.DefaultGroups) > 0 {
		u.DefaultGroups = overlay.DefaultGroups
	}
	if overlay.SudoNopasswd {
		u.SudoNopasswd = true
	}
	return u
}

func mergeSSH(base, overlay SSHConfig) SSHConfig {
	s := base
	if overlay.DisableRootLogin {
		s.DisableRootLogin = true
	}
	if overlay.DisablePasswordAuth {
		s.DisablePasswordAuth = true
	}
	if overlay.Port != 0 {
		s.Port = overlay.Port
	}
	return s
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("ROOTFILES_HOME_BASE"); v != "" {
		cfg.Users.HomeBase = v
	}
	if v := os.Getenv("ROOTFILES_TIMEZONE"); v != "" {
		cfg.Timezone = v
	}
	if v := os.Getenv("ROOTFILES_TUNNEL_TOKEN"); v != "" {
		cfg.Modules.Cloudflared.TunnelToken = v
	}
	if v := os.Getenv("ROOTFILES_VLAN_ADDRESS"); v != "" {
		cfg.Modules.Cloudflared.PrivateNetwork.Address = v
	}
	if v := os.Getenv("ROOTFILES_VLAN_INTERFACE"); v != "" {
		cfg.Modules.Cloudflared.PrivateNetwork.Interface = v
	}
	if v := os.Getenv("ROOTFILES_DOCKER_ROOT"); v != "" {
		cfg.Modules.Docker.StorageDir = v
	}
	if v := os.Getenv("ROOTFILES_DATA_DIR"); v != "" {
		cfg.Modules.Storage.DataDir = v
	}
}
