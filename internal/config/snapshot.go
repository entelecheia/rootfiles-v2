package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SnapshotCurrent detects the current system configuration and returns a Config
// that can be used with `rootfiles apply --config`. This works even on servers
// that were not originally configured with rootfiles.
func SnapshotCurrent() (*Config, error) {
	cfg := &Config{
		Locale:   detectLocale(),
		Timezone: detectTimezone(),
	}

	// SSH
	cfg.SSH = detectSSHConfig()
	cfg.Modules.SSH.Enabled = true

	// Docker
	cfg.Modules.Docker = detectDockerConfig()

	// Network / UFW
	cfg.Modules.Network = detectNetworkConfig()

	// Cloudflared / VLAN
	cfg.Modules.Cloudflared = detectCloudflaredConfig()

	// Users
	cfg.Users = detectUsersConfig()
	cfg.Modules.Users.Enabled = true

	// Storage
	cfg.Modules.Storage = detectStorageConfig()

	// Always enable base modules
	cfg.Modules.Locale.Enabled = true
	cfg.Modules.Packages.Enabled = true

	// Detect NVIDIA
	if _, err := exec.Command("nvidia-smi").Output(); err == nil {
		cfg.Modules.Nvidia.Enabled = true
	}

	return cfg, nil
}

// MarshalYAML returns the config as YAML bytes suitable for writing to a file.
func MarshalYAML(cfg *Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}

func detectLocale() string {
	// Try /etc/default/locale
	data, err := os.ReadFile("/etc/default/locale")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "LANG=") {
				return strings.Trim(strings.TrimPrefix(line, "LANG="), "\"")
			}
		}
	}
	// Fallback to locale command
	out, err := exec.Command("locale").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "LANG=") {
				return strings.Trim(strings.TrimPrefix(line, "LANG="), "\"")
			}
		}
	}
	return "en_US.UTF-8"
}

func detectTimezone() string {
	// Try timedatectl
	out, err := exec.Command("timedatectl", "show", "--property=Timezone", "--value").Output()
	if err == nil {
		tz := strings.TrimSpace(string(out))
		if tz != "" {
			return tz
		}
	}
	// Fallback to /etc/timezone
	data, err := os.ReadFile("/etc/timezone")
	if err == nil {
		tz := strings.TrimSpace(string(data))
		if tz != "" {
			return tz
		}
	}
	return "UTC"
}

func detectSSHConfig() SSHConfig {
	cfg := SSHConfig{}

	// Try sshd -T for merged config
	out, err := exec.Command("sshd", "-T").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(strings.ToLower(line))
			if strings.HasPrefix(line, "permitrootlogin ") {
				val := strings.TrimPrefix(line, "permitrootlogin ")
				cfg.DisableRootLogin = (val == "no")
			}
			if strings.HasPrefix(line, "passwordauthentication ") {
				val := strings.TrimPrefix(line, "passwordauthentication ")
				cfg.DisablePasswordAuth = (val == "no")
			}
			if strings.HasPrefix(line, "port ") {
				val := strings.TrimPrefix(line, "port ")
				if p, err := strconv.Atoi(val); err == nil && p != 22 {
					cfg.Port = p
				}
			}
		}
		return cfg
	}

	// Fallback: parse sshd_config.d files
	entries, _ := os.ReadDir("/etc/ssh/sshd_config.d")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile("/etc/ssh/sshd_config.d/" + e.Name())
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "permitrootlogin") {
				parts := strings.Fields(line)
				if len(parts) >= 2 && strings.EqualFold(parts[1], "no") {
					cfg.DisableRootLogin = true
				}
			}
			if strings.HasPrefix(lower, "passwordauthentication") {
				parts := strings.Fields(line)
				if len(parts) >= 2 && strings.EqualFold(parts[1], "no") {
					cfg.DisablePasswordAuth = true
				}
			}
			if strings.HasPrefix(lower, "port") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if p, err := strconv.Atoi(parts[1]); err == nil && p != 22 {
						cfg.Port = p
					}
				}
			}
		}
	}
	return cfg
}

func detectDockerConfig() DockerConfig {
	cfg := DockerConfig{}

	// Check if docker is installed
	if _, err := exec.LookPath("docker"); err != nil {
		return cfg
	}
	cfg.Enabled = true

	// Parse daemon.json for data-root
	data, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil {
		return cfg
	}
	var daemon map[string]interface{}
	if err := json.Unmarshal(data, &daemon); err != nil {
		return cfg
	}
	if root, ok := daemon["data-root"].(string); ok && root != "/var/lib/docker" {
		cfg.StorageDir = root
	}
	return cfg
}

func detectNetworkConfig() NetworkConfig {
	cfg := NetworkConfig{}

	out, err := exec.Command("ufw", "status").Output()
	if err != nil {
		return cfg
	}
	status := string(out)
	if strings.Contains(status, "Status: active") {
		cfg.Enabled = true
		cfg.UFW = true
		cfg.AllowedPorts = parseUFWPorts(status)
	}
	return cfg
}

func parseUFWPorts(status string) []int {
	var ports []int
	seen := make(map[int]bool)
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Status:") || strings.HasPrefix(line, "To") || strings.HasPrefix(line, "--") {
			continue
		}
		// Lines like: "22/tcp                     ALLOW       Anywhere"
		// or:         "80,443/tcp                 ALLOW       Anywhere"
		if !strings.Contains(line, "ALLOW") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		portSpec := parts[0]
		// Strip protocol suffix
		portSpec = strings.TrimSuffix(portSpec, "/tcp")
		portSpec = strings.TrimSuffix(portSpec, "/udp")
		// Handle comma-separated ports
		for _, ps := range strings.Split(portSpec, ",") {
			if p, err := strconv.Atoi(strings.TrimSpace(ps)); err == nil {
				if !seen[p] {
					seen[p] = true
					ports = append(ports, p)
				}
			}
		}
	}
	return ports
}

func detectCloudflaredConfig() CloudflaredConfig {
	cfg := CloudflaredConfig{}

	if _, err := os.Stat("/usr/local/bin/cloudflared"); err != nil {
		return cfg
	}
	cfg.Enabled = true

	// Detect tunnel token from systemd service
	out, err := exec.Command("systemctl", "show", "cloudflared", "--property=ExecStart").Output()
	if err == nil {
		s := string(out)
		if idx := strings.Index(s, "tunnel run --token "); idx >= 0 {
			token := s[idx+len("tunnel run --token "):]
			// Token ends at whitespace, semicolon, or quote
			token = strings.Fields(token)[0]
			token = strings.Trim(token, "\"';")
			if token != "" {
				cfg.TunnelToken = token
			}
		}
	}

	// Detect VLAN address
	vlanOut, err := exec.Command("ip", "addr", "show", "vlan0").Output()
	if err == nil {
		for _, line := range strings.Split(string(vlanOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "inet ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					cfg.PrivateNetwork = PrivateNetworkConfig{
						Enabled:   true,
						Interface: "vlan0",
						Address:   parts[1],
					}
				}
				break
			}
		}
	}
	return cfg
}

func detectUsersConfig() UsersConfig {
	cfg := UsersConfig{
		DefaultShell: "/usr/bin/zsh",
	}

	// Detect home_base from /etc/default/useradd or common paths
	data, err := os.ReadFile("/etc/default/useradd")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "HOME=") {
				hb := strings.TrimPrefix(line, "HOME=")
				if hb != "/home" && hb != "" {
					cfg.HomeBase = hb
				}
			}
		}
	}

	// Fallback: check common custom home paths
	if cfg.HomeBase == "" {
		for _, candidate := range []string{"/raid/home", "/data/home"} {
			if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
				cfg.HomeBase = candidate
				break
			}
		}
	}
	if cfg.HomeBase == "" {
		cfg.HomeBase = "/home"
	}

	return cfg
}

func detectStorageConfig() StorageConfig {
	cfg := StorageConfig{}

	// Check for common data directories
	for _, candidate := range []string{"/raid/data", "/data"} {
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			cfg.Enabled = true
			cfg.DataDir = candidate
			break
		}
	}

	// Detect symlinks
	symlinks := map[string]string{}
	for _, link := range []string{"/data"} {
		target, err := os.Readlink(link)
		if err == nil {
			symlinks[link] = target
		}
	}
	if len(symlinks) > 0 {
		cfg.Enabled = true
		cfg.Symlinks = symlinks
	}

	return cfg
}
