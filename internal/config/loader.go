package config

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.yaml
var embeddedProfiles embed.FS

const maxExtendsDepth = 5

// Load resolves a profile by name (or custom path, "-" for stdin), applies
// env overrides, validates the result and attaches system info.
func Load(profileName, customPath string, sysInfo *SystemInfo) (*Config, error) {
	var cfg *Config
	var err error

	if customPath != "" {
		cfg, err = resolveFile(customPath)
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
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.System = sysInfo
	return cfg, nil
}

// AvailableProfiles returns the list of built-in profile names.
func AvailableProfiles() []string {
	return []string{"base", "minimal", "dgx", "gpu-server", "full"}
}

// Profiles are merged as YAML trees before decoding: a key present in the
// child overrides the parent even when its value is false/0/"" (so a child
// can disable a module or setting), nested maps merge key by key, lists
// replace — except packages_extra, which accumulates along the chain.

func resolveProfile(name string, depth int) (*Config, error) {
	tree, err := resolveEmbeddedTree(name, depth)
	if err != nil {
		return nil, err
	}
	return decodeTree(tree, "profile "+name)
}

// resolveFile loads a custom config. Its `extends` may name a built-in
// profile or another file (relative to the including file).
func resolveFile(path string) (*Config, error) {
	tree, err := resolveFileTree(path, 0, map[string]bool{})
	if err != nil {
		return nil, err
	}
	return decodeTree(tree, "config "+path)
}

func resolveEmbeddedTree(name string, depth int) (map[string]any, error) {
	if depth > maxExtendsDepth {
		return nil, fmt.Errorf("profile extends chain too deep (max %d)", maxExtendsDepth)
	}
	data, err := embeddedProfiles.ReadFile("profiles/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("loading profile %q: profile %q not found: %w", name, name, err)
	}
	tree, err := parseTree(data, "profile "+name)
	if err != nil {
		return nil, err
	}
	parent, _ := tree["extends"].(string)
	delete(tree, "extends")
	if parent == "" {
		return tree, nil
	}
	base, err := resolveEmbeddedTree(parent, depth+1)
	if err != nil {
		return nil, err
	}
	return mergeTrees(base, tree), nil
}

func resolveFileTree(path string, depth int, seen map[string]bool) (map[string]any, error) {
	if depth > maxExtendsDepth {
		return nil, fmt.Errorf("config extends chain too deep (max %d)", maxExtendsDepth)
	}
	var data []byte
	var err error
	dir := "."
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		abs, aerr := filepath.Abs(path)
		if aerr == nil {
			if seen[abs] {
				return nil, fmt.Errorf("config extends cycle at %s", path)
			}
			seen[abs] = true
			dir = filepath.Dir(abs)
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}
	tree, err := parseTree(data, "config "+path)
	if err != nil {
		return nil, err
	}
	parent, _ := tree["extends"].(string)
	delete(tree, "extends")
	if parent == "" {
		return tree, nil
	}

	var base map[string]any
	if isProfileName(parent) {
		base, err = resolveEmbeddedTree(parent, depth+1)
	} else {
		if !filepath.IsAbs(parent) {
			parent = filepath.Join(dir, parent)
		}
		base, err = resolveFileTree(parent, depth+1, seen)
	}
	if err != nil {
		return nil, err
	}
	return mergeTrees(base, tree), nil
}

func isProfileName(s string) bool {
	for _, p := range AvailableProfiles() {
		if s == p {
			return true
		}
	}
	return false
}

func parseTree(data []byte, what string) (map[string]any, error) {
	// Strict-decode each file on its own first so unknown keys are
	// reported with that file's name and line numbers.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var probe Config
	if err := dec.Decode(&probe); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parsing %s: %w", what, err)
	}
	tree := map[string]any{}
	if err := yaml.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", what, err)
	}
	return tree, nil
}

// mergeTrees overlays overlay onto base and returns base.
func mergeTrees(base, overlay map[string]any) map[string]any {
	for k, ov := range overlay {
		if k == "packages_extra" {
			bl, _ := base[k].([]any)
			ol, _ := ov.([]any)
			base[k] = append(append([]any{}, bl...), ol...)
			continue
		}
		bm, bIsMap := base[k].(map[string]any)
		om, oIsMap := ov.(map[string]any)
		if bIsMap && oIsMap {
			base[k] = mergeTrees(bm, om)
			continue
		}
		base[k] = ov
	}
	return base
}

// decodeTree converts a merged tree into a Config, rejecting unknown keys
// so a typo (e.g. "disable_pasword_auth") fails loudly instead of being
// silently ignored.
func decodeTree(tree map[string]any, what string) (*Config, error) {
	data, err := yaml.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", what, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parsing %s: %w", what, err)
	}
	return &cfg, nil
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
