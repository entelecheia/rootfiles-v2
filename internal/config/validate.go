package config

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
)

// Validate reports semantic errors that YAML decoding cannot catch. All
// problems are returned together so a config can be fixed in one pass.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	checkPort := func(field string, p int) {
		if p < 0 || p > 65535 {
			add("%s: port %d out of range", field, p)
		}
	}
	checkAbs := func(field, p string) {
		if p != "" && !filepath.IsAbs(p) {
			add("%s: %q must be an absolute path", field, p)
		}
	}

	checkPort("ssh.port", c.SSH.Port)
	for _, p := range c.Modules.Network.AllowedPorts {
		if p <= 0 {
			add("modules.network.allowed_ports: port %d out of range", p)
		}
		checkPort("modules.network.allowed_ports", p)
	}

	checkAbs("users.home_base", c.Users.HomeBase)
	checkAbs("users.default_shell", c.Users.DefaultShell)
	checkAbs("modules.docker.storage_dir", c.Modules.Docker.StorageDir)
	checkAbs("modules.storage.data_dir", c.Modules.Storage.DataDir)
	for link, target := range c.Modules.Storage.Symlinks {
		checkAbs("modules.storage.symlinks key", link)
		checkAbs("modules.storage.symlinks["+link+"]", target)
		if filepath.Clean(link) == "/" {
			add("modules.storage.symlinks: refusing to replace /")
		}
	}

	switch c.Modules.Nvidia.GPUAllocation.Method {
	case "", "env", "cgroup", "both":
	default:
		add("modules.nvidia.gpu_allocation.method: %q (want env, cgroup or both)", c.Modules.Nvidia.GPUAllocation.Method)
	}

	if pn := c.Modules.Cloudflared.PrivateNetwork; pn.Address != "" {
		if _, err := netip.ParsePrefix(pn.Address); err != nil {
			add("modules.cloudflared.private_network.address: %q is not a CIDR (e.g. 172.16.229.32/32)", pn.Address)
		}
	}
	if iface := c.Modules.Cloudflared.PrivateNetwork.Interface; len(iface) > 15 || strings.ContainsAny(iface, " /") {
		add("modules.cloudflared.private_network.interface: %q is not a valid interface name", iface)
	}

	if strings.ContainsAny(c.Timezone, " ") || strings.HasPrefix(c.Timezone, "/") || strings.Contains(c.Timezone, "..") {
		add("timezone: %q is not a zone name (e.g. Asia/Seoul)", c.Timezone)
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return nil
}
