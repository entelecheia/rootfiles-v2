package config

import (
	"os"
	"path/filepath"
	"strings"
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
	// Users config: home_base is left unset so Load can detect a data drive
	if cfg.Users.HomeBase != "" {
		t.Errorf("home_base = %q, want unset", cfg.Users.HomeBase)
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
	if cfg.Users.HomeBase != "" {
		t.Errorf("home_base = %q, want unset", cfg.Users.HomeBase)
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

func TestMergeTrees(t *testing.T) {
	base := map[string]any{
		"locale":         "en_US.UTF-8",
		"packages_extra": []any{"a"},
		"users":          map[string]any{"home_base": "/home", "sudo_nopasswd": true},
		"modules":        map[string]any{"docker": map[string]any{"enabled": true, "storage_dir": "/x"}},
	}
	overlay := map[string]any{
		"packages_extra": []any{"b"},
		"users":          map[string]any{"home_base": "/raid/home", "sudo_nopasswd": false},
		"modules":        map[string]any{"docker": map[string]any{"enabled": false}},
	}
	m := mergeTrees(base, overlay)
	users := m["users"].(map[string]any)
	if users["home_base"] != "/raid/home" || users["sudo_nopasswd"] != false {
		t.Errorf("users = %v", users)
	}
	docker := m["modules"].(map[string]any)["docker"].(map[string]any)
	if docker["enabled"] != false || docker["storage_dir"] != "/x" {
		t.Errorf("docker = %v (explicit false must win, other keys kept)", docker)
	}
	if extra := m["packages_extra"].([]any); len(extra) != 2 {
		t.Errorf("packages_extra = %v, want accumulated", extra)
	}
	if m["locale"] != "en_US.UTF-8" {
		t.Errorf("locale lost: %v", m["locale"])
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_CustomFileExtendsProfileAndDisables(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "site.yaml", `extends: dgx
modules:
  cloudflared:
    enabled: false
ssh:
  disable_root_login: false
users:
  sudo_nopasswd: false
`)
	cfg, err := Load("", p, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Users.HomeBase != "/raid/home" || !cfg.Modules.Docker.Enabled {
		t.Error("dgx settings should be inherited")
	}
	if cfg.Modules.Cloudflared.Enabled {
		t.Error("child must be able to disable a module")
	}
	if cfg.SSH.DisableRootLogin {
		t.Error("child must be able to set a bool back to false")
	}
	if !cfg.SSH.DisablePasswordAuth {
		t.Error("unrelated ssh settings should be kept")
	}
	if cfg.Users.SudoNopasswd {
		t.Error("sudo_nopasswd override to false ignored")
	}
	if len(cfg.PackagesExtra) == 0 || len(cfg.Packages) == 0 {
		t.Error("packages should be inherited through the chain")
	}
}

func TestLoad_CustomFileExtendsRelativeFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "common.yaml", "extends: minimal\ntimezone: UTC\n")
	os.MkdirAll(filepath.Join(dir, "hosts"), 0755)
	p := writeFile(t, filepath.Join(dir, "hosts"), "gpu01.yaml", "extends: ../common.yaml\nlocale: ko_KR.UTF-8\n")
	cfg, err := Load("", p, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Timezone != "UTC" || cfg.Locale != "ko_KR.UTF-8" || !cfg.Modules.Users.Enabled {
		t.Errorf("unexpected merge: tz=%q locale=%q users=%v", cfg.Timezone, cfg.Locale, cfg.Modules.Users.Enabled)
	}
}

func TestLoad_ExtendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "extends: b.yaml\n")
	p := writeFile(t, dir, "b.yaml", "extends: a.yaml\n")
	if _, err := Load("", p, nil); err == nil {
		t.Error("expected cycle error")
	}
}

func TestLoad_UnknownKeyRejected(t *testing.T) {
	p := writeFile(t, t.TempDir(), "typo.yaml", "extends: minimal\nssh:\n  disable_pasword_auth: true\n")
	_, err := Load("", p, nil)
	if err == nil || !strings.Contains(err.Error(), "disable_pasword_auth") {
		t.Errorf("expected unknown field error, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	cfg := &Config{
		SSH:   SSHConfig{Port: 70000},
		Users: UsersConfig{HomeBase: "raid/home"},
		Modules: ModulesConfig{
			Nvidia:      NvidiaConfig{GPUAllocation: GPUAllocationConfig{Method: "magic"}},
			Cloudflared: CloudflaredConfig{PrivateNetwork: PrivateNetworkConfig{Address: "172.16.0.1"}},
			Storage:     StorageConfig{Symlinks: map[string]string{"/": "/raid"}},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"ssh.port", "home_base", "method", "CIDR", "refusing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q:\n%v", want, err)
		}
	}
}

func TestAllProfilesValid(t *testing.T) {
	for _, name := range AvailableProfiles() {
		if _, err := Load(name, "", nil); err != nil {
			t.Errorf("profile %s: %v", name, err)
		}
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
				Enabled:       true,
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

func TestLoad_TunnelTokenFile(t *testing.T) {
	dir := t.TempDir()
	both := writeFile(t, dir, "both.yaml", "extends: minimal\nmodules:\n  cloudflared:\n    enabled: true\n    tunnel_token: inline\n    tunnel_token_file: /etc/rootfiles/tunnel-token\n")
	if _, err := Load("", both, nil); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("tunnel_token with tunnel_token_file: want 'not both' error, got %v", err)
	}

	rel := writeFile(t, dir, "rel.yaml", "extends: minimal\nmodules:\n  cloudflared:\n    enabled: true\n    tunnel_token_file: tunnel-token\n")
	if _, err := Load("", rel, nil); err == nil || !strings.Contains(err.Error(), "tunnel_token_file") {
		t.Errorf("relative tunnel_token_file: want path error, got %v", err)
	}

	// The file is read at check/apply time, so a path that only exists on
	// the server still validates on an operator laptop.
	ok := writeFile(t, dir, "ok.yaml", "extends: minimal\nmodules:\n  cloudflared:\n    enabled: true\n    tunnel_token_file: /nonexistent/rootfiles/tunnel-token\n")
	cfg, err := Load("", ok, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Modules.Cloudflared.TunnelTokenFile != "/nonexistent/rootfiles/tunnel-token" || cfg.Modules.Cloudflared.TunnelToken != "" {
		t.Errorf("unexpected cloudflared config %+v", cfg.Modules.Cloudflared)
	}

	// ROOTFILES_TUNNEL_TOKEN may still override a file-based config.
	t.Setenv("ROOTFILES_TUNNEL_TOKEN", "from-env")
	cfg, err = Load("", ok, nil)
	if err != nil || cfg.Modules.Cloudflared.TunnelToken != "from-env" {
		t.Errorf("env override with tunnel_token_file: token=%q err=%v", cfg.Modules.Cloudflared.TunnelToken, err)
	}
}

func TestValidate_PasswordAuthUsers(t *testing.T) {
	cases := map[string][]string{
		"root":      {"root"},
		"bad name":  {"Bob Smith"},
		"duplicate": {"bob", "bob"},
	}
	for name, users := range cases {
		cfg := &Config{SSH: SSHConfig{DisablePasswordAuth: true, PasswordAuthUsers: users}}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "password_auth_users") {
			t.Errorf("%s: want password_auth_users error, got %v", name, err)
		}
	}
	ok := &Config{SSH: SSHConfig{DisablePasswordAuth: true, PasswordAuthUsers: []string{"bob", "carol.k"}}}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid exception list rejected: %v", err)
	}
}

// TestMain keeps home-base detection off the host for every Load here.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rootfiles-config-test")
	if err != nil {
		panic(err)
	}
	useraddDefaultsPath = filepath.Join(dir, "useradd")
	metaRoot = filepath.Join(dir, "root")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// isolateHomeDetection points home-base detection at temp files instead of
// the host's /etc/default/useradd and <base>/.rootfiles directories.
// managed, when set, is a home base that already holds .rootfiles.
func isolateHomeDetection(t *testing.T, useradd, managed string) {
	t.Helper()
	dir := t.TempDir()
	oldUseradd, oldRoot := useraddDefaultsPath, metaRoot
	t.Cleanup(func() { useraddDefaultsPath, metaRoot = oldUseradd, oldRoot })
	useraddDefaultsPath = filepath.Join(dir, "useradd")
	metaRoot = filepath.Join(dir, "root")
	if useradd != "" {
		if err := os.WriteFile(useraddDefaultsPath, []byte(useradd), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if managed != "" {
		if err := os.MkdirAll(filepath.Join(metaRoot, managed, ".rootfiles"), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefaultHomeBase(t *testing.T) {
	mounts := func(m ...MountPoint) *SystemInfo { return &SystemInfo{OS: "ubuntu", StorageLayout: m} }
	data := MountPoint{Device: "/dev/sdb1", MountPath: "/data", FSType: "ext4"}
	cases := []struct {
		name    string
		sys     *SystemInfo
		useradd string
		managed string
		want    string
	}{
		{"no system info", nil, "", "", "/home"},
		{"no data drive", mounts(), "", "", "/home"},
		{"data drive", mounts(data), "", "", "/data/home"},
		{"raid preferred over data", mounts(data, MountPoint{"/dev/md0", "/raid", "xfs"}), "", "", "/raid/home"},
		{"network filesystem skipped", mounts(MountPoint{"nas:/x", "/data", "nfs4"}), "", "", "/home"},
		{"ephemeral /mnt skipped", mounts(MountPoint{"/dev/sdb1", "/mnt", "ext4"}), "", "", "/home"},
		{"nested mount skipped", mounts(MountPoint{"/dev/sdb1", "/data/ssd", "ext4"}), "", "", "/home"},
		{"custom useradd HOME kept", mounts(data), "SHELL=/bin/sh\nHOME=/srv/home\n", "", "/srv/home"},
		{"useradd HOME=/home kept", mounts(data), "HOME=/home\n", "", "/home"},
		{"stale custom HOME rewritten to /home kept", mounts(data), "HOME=/data/home\nHOME=/home\n", "", "/home"},
		{"commented useradd HOME ignored", mounts(data), "# HOME=/srv/home\n", "", "/data/home"},
		{"users already managed under /home", mounts(data), "", "/home", "/home"},
		{"users managed under old gpu-server pin", mounts(), "", "/data/home", "/data/home"},
		{"dgx os data drive", &SystemInfo{OS: "dgx-os", StorageLayout: []MountPoint{data}}, "", "", "/data/home"},
		{"rocky keeps /home", &SystemInfo{OS: "rocky", Version: "9.4", StorageLayout: []MountPoint{data}}, "HOME=/home\n", "", "/home"},
		{"unsupported distro keeps /home", &SystemInfo{OS: "debian", StorageLayout: []MountPoint{data}}, "", "", "/home"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHomeDetection(t, tc.useradd, tc.managed)
			if got := defaultHomeBase(tc.sys); got != tc.want {
				t.Errorf("defaultHomeBase = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_HomeBaseDetectionOnlyWhenUnset(t *testing.T) {
	isolateHomeDetection(t, "", "")
	t.Setenv("ROOTFILES_HOME_BASE", "")
	sys := &SystemInfo{OS: "ubuntu", StorageLayout: []MountPoint{{Device: "/dev/sdb1", MountPath: "/data", FSType: "xfs"}}}

	cfg, err := Load("minimal", "", sys)
	if err != nil || cfg.Users.HomeBase != "/data/home" {
		t.Fatalf("unset home_base: got %q, err=%v; want /data/home", cfg.Users.HomeBase, err)
	}

	explicit := filepath.Join(t.TempDir(), "site.yaml")
	if err := os.WriteFile(explicit, []byte("extends: minimal\nusers:\n  home_base: /home\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load("", explicit, sys); err != nil || cfg.Users.HomeBase != "/home" {
		t.Errorf("explicit home_base: got %q, err=%v; want /home", cfg.Users.HomeBase, err)
	}

	t.Setenv("ROOTFILES_HOME_BASE", "/env/home")
	if cfg, err = Load("minimal", "", sys); err != nil || cfg.Users.HomeBase != "/env/home" {
		t.Errorf("env override: got %q, err=%v; want /env/home", cfg.Users.HomeBase, err)
	}
}

func TestLoadSite_RequiresExplicitHomeBase(t *testing.T) {
	site := filepath.Join(t.TempDir(), "site.yaml")
	if err := os.WriteFile(site, []byte("extends: minimal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSite(site); err == nil || !strings.Contains(err.Error(), "users.home_base") {
		t.Errorf("site config without home_base: want users.home_base error, got %v", err)
	}
}
