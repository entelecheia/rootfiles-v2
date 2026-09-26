package config

// Config is the root configuration struct, mapped from profile YAML files.
type Config struct {
	Extends       string        `yaml:"extends,omitempty"`
	Locale        string        `yaml:"locale"`
	Timezone      string        `yaml:"timezone"`
	Modules       ModulesConfig `yaml:"modules"`
	Packages      []string      `yaml:"packages"`
	PackagesExtra []string      `yaml:"packages_extra"`
	Users         UsersConfig   `yaml:"users"`
	SSH           SSHConfig     `yaml:"ssh"`
	// Populated at runtime, not from YAML
	System *SystemInfo `yaml:"-"`
}

type ModulesConfig struct {
	Locale      ModuleToggle      `yaml:"locale"`
	Packages    ModuleToggle      `yaml:"packages"`
	SSH         ModuleToggle      `yaml:"ssh"`
	Users       ModuleToggle      `yaml:"users"`
	Docker      DockerConfig      `yaml:"docker"`
	Nvidia      NvidiaConfig      `yaml:"nvidia"`
	Cloudflared CloudflaredConfig `yaml:"cloudflared"`
	Storage     StorageConfig     `yaml:"storage"`
	Network     NetworkConfig     `yaml:"network"`
}

type ModuleToggle struct {
	Enabled bool `yaml:"enabled"`
}

type NvidiaConfig struct {
	Enabled       bool                `yaml:"enabled"`
	GPUAllocation GPUAllocationConfig `yaml:"gpu_allocation,omitempty"`
}

type GPUAllocationConfig struct {
	Enabled bool   `yaml:"enabled"`
	Method  string `yaml:"method,omitempty"` // "env" (default), "cgroup", "both"
}

type DockerConfig struct {
	Enabled    bool   `yaml:"enabled"`
	StorageDir string `yaml:"storage_dir,omitempty"`
}

type CloudflaredConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	TunnelToken    string               `yaml:"tunnel_token,omitempty"`
	PrivateNetwork PrivateNetworkConfig `yaml:"private_network,omitempty"`
}

type PrivateNetworkConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Interface string `yaml:"interface"`
	Address   string `yaml:"address"`
}

type StorageConfig struct {
	Enabled  bool              `yaml:"enabled"`
	DataDir  string            `yaml:"data_dir,omitempty"`
	Symlinks map[string]string `yaml:"symlinks,omitempty"`
}

type NetworkConfig struct {
	Enabled      bool  `yaml:"enabled"`
	UFW          bool  `yaml:"ufw"`
	AllowedPorts []int `yaml:"allowed_ports,omitempty"`
}

type UsersConfig struct {
	HomeBase      string   `yaml:"home_base"`
	DefaultShell  string   `yaml:"default_shell"`
	DefaultGroups []string `yaml:"default_groups"`
	SudoNopasswd  bool     `yaml:"sudo_nopasswd"`
	// Accounts are converged by the users module: created if missing,
	// missing SSH keys and group memberships added. Never removed.
	Accounts []AccountConfig `yaml:"accounts,omitempty"`
}

// AccountConfig declares a login account.
type AccountConfig struct {
	Name       string   `yaml:"name"`
	SSHPubkeys []string `yaml:"ssh_pubkeys,omitempty"`
	Groups     []string `yaml:"groups,omitempty"` // in addition to users.default_groups
}

// AddAccount merges an account into Accounts (keys and groups are unioned
// when the name already exists).
func (u *UsersConfig) AddAccount(a AccountConfig) {
	for i := range u.Accounts {
		if u.Accounts[i].Name == a.Name {
			u.Accounts[i].SSHPubkeys = appendUnique(u.Accounts[i].SSHPubkeys, a.SSHPubkeys...)
			u.Accounts[i].Groups = appendUnique(u.Accounts[i].Groups, a.Groups...)
			return
		}
	}
	u.Accounts = append(u.Accounts, a)
}

func appendUnique(list []string, items ...string) []string {
	for _, it := range items {
		found := false
		for _, l := range list {
			if l == it {
				found = true
				break
			}
		}
		if !found && it != "" {
			list = append(list, it)
		}
	}
	return list
}

type SSHConfig struct {
	DisableRootLogin    bool `yaml:"disable_root_login"`
	DisablePasswordAuth bool `yaml:"disable_password_auth"`
	Port                int  `yaml:"port,omitempty"`
}

// IsModuleEnabled returns whether a given module name is enabled in this config.
func (c *Config) IsModuleEnabled(name string) bool {
	switch name {
	case "locale":
		return c.Modules.Locale.Enabled
	case "packages":
		return c.Modules.Packages.Enabled
	case "ssh":
		return c.Modules.SSH.Enabled
	case "users":
		return c.Modules.Users.Enabled
	case "docker":
		return c.Modules.Docker.Enabled
	case "nvidia":
		return c.Modules.Nvidia.Enabled
	case "gpu":
		return c.Modules.Nvidia.GPUAllocation.Enabled
	case "cloudflared":
		return c.Modules.Cloudflared.Enabled
	case "storage":
		return c.Modules.Storage.Enabled
	case "network":
		return c.Modules.Network.Enabled
	default:
		return false
	}
}

// AllPackages returns the merged package list (base + extra).
func (c *Config) AllPackages() []string {
	seen := make(map[string]bool, len(c.Packages)+len(c.PackagesExtra))
	var result []string
	for _, p := range c.Packages {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	for _, p := range c.PackagesExtra {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
