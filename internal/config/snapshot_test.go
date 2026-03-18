package config

import (
	"strings"
	"testing"
)

func TestSnapshotCurrent_ReturnsConfig(t *testing.T) {
	cfg, err := SnapshotCurrent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config should not be nil")
	}
	// Base modules should always be enabled
	if !cfg.Modules.Locale.Enabled {
		t.Error("locale module should be enabled")
	}
	if !cfg.Modules.Packages.Enabled {
		t.Error("packages module should be enabled")
	}
	if !cfg.Modules.SSH.Enabled {
		t.Error("ssh module should be enabled")
	}
	if !cfg.Modules.Users.Enabled {
		t.Error("users module should be enabled")
	}
	// Locale and timezone should have non-empty defaults
	if cfg.Locale == "" {
		t.Error("locale should not be empty")
	}
	if cfg.Timezone == "" {
		t.Error("timezone should not be empty")
	}
}

func TestMarshalYAML_RoundTrip(t *testing.T) {
	cfg := &Config{
		Locale:   "en_US.UTF-8",
		Timezone: "Asia/Seoul",
		Modules: ModulesConfig{
			Locale:   ModuleToggle{Enabled: true},
			Packages: ModuleToggle{Enabled: true},
			SSH:      ModuleToggle{Enabled: true},
			Docker:   DockerConfig{Enabled: true, StorageDir: "/raid/docker"},
			Network:  NetworkConfig{Enabled: true, UFW: true, AllowedPorts: []int{22, 80, 443}},
		},
		SSH: SSHConfig{
			DisableRootLogin:    true,
			DisablePasswordAuth: true,
			Port:                2222,
		},
		Users: UsersConfig{
			HomeBase:     "/raid/home",
			DefaultShell: "/usr/bin/zsh",
		},
	}

	data, err := MarshalYAML(cfg)
	if err != nil {
		t.Fatalf("MarshalYAML error: %v", err)
	}
	yaml := string(data)

	// Check key fields are present in output
	checks := []string{
		"en_US.UTF-8",
		"Asia/Seoul",
		"/raid/docker",
		"/raid/home",
		"disable_root_login: true",
		"disable_password_auth: true",
		"port: 2222",
	}
	for _, c := range checks {
		if !strings.Contains(yaml, c) {
			t.Errorf("YAML output missing %q", c)
		}
	}
}

func TestDetectLocale_Fallback(t *testing.T) {
	// Should always return a non-empty string
	locale := detectLocale()
	if locale == "" {
		t.Error("detectLocale should return a fallback value")
	}
}

func TestDetectTimezone_Fallback(t *testing.T) {
	tz := detectTimezone()
	if tz == "" {
		t.Error("detectTimezone should return a fallback value")
	}
}

func TestParseUFWPorts(t *testing.T) {
	status := `Status: active

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW       Anywhere
80,443/tcp                 ALLOW       Anywhere
8080/tcp                   ALLOW       Anywhere
22/tcp (v6)                ALLOW       Anywhere (v6)
`
	ports := parseUFWPorts(status)
	if len(ports) < 3 {
		t.Errorf("expected at least 3 ports, got %d: %v", len(ports), ports)
	}
	// Check deduplication (22 appears twice in input)
	count22 := 0
	for _, p := range ports {
		if p == 22 {
			count22++
		}
	}
	if count22 != 1 {
		t.Errorf("port 22 should appear once (deduped), got %d", count22)
	}
	// Check 80 and 443 parsed from comma-separated
	has80, has443 := false, false
	for _, p := range ports {
		if p == 80 {
			has80 = true
		}
		if p == 443 {
			has443 = true
		}
	}
	if !has80 {
		t.Error("port 80 should be parsed")
	}
	if !has443 {
		t.Error("port 443 should be parsed")
	}
}
