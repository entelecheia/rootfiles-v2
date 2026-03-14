package module

import (
	"context"
	"fmt"
	"strings"
)

type LocaleModule struct{}

func NewLocaleModule() *LocaleModule { return &LocaleModule{} }
func (m *LocaleModule) Name() string { return "locale" }

func (m *LocaleModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config

	// Check locale
	if cfg.Locale != "" {
		out, _ := rc.Runner.ReadFile("/etc/default/locale")
		if !strings.Contains(string(out), cfg.Locale) {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Set locale to %s", cfg.Locale),
				Command:     fmt.Sprintf("locale-gen %s && update-locale LANG=%s", cfg.Locale, cfg.Locale),
			})
		}
	}

	// Check timezone
	if cfg.Timezone != "" {
		out, _ := rc.Runner.ReadFile("/etc/timezone")
		current := strings.TrimSpace(string(out))
		if current != cfg.Timezone {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Set timezone to %s", cfg.Timezone),
				Command:     fmt.Sprintf("timedatectl set-timezone %s", cfg.Timezone),
			})
		}
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *LocaleModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	cfg := rc.Config
	var messages []string
	changed := false

	if cfg.Locale != "" {
		// Ensure locales package is installed (provides locale-gen)
		if !rc.Runner.CommandExists("locale-gen") {
			rc.APT.Update(ctx)
			rc.APT.Install(ctx, []string{"locales"})
		}
		// Generate locale (best-effort — may fail in minimal containers)
		rc.Runner.Run(ctx, "locale-gen", cfg.Locale)
		// Set default locale
		content := fmt.Sprintf("LANG=%s\nLC_ALL=%s\n", cfg.Locale, cfg.Locale)
		if err := rc.Runner.WriteFile("/etc/default/locale", []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("writing locale: %w", err)
		}
		messages = append(messages, fmt.Sprintf("locale set to %s", cfg.Locale))
		changed = true
	}

	if cfg.Timezone != "" {
		if _, err := rc.Runner.Run(ctx, "timedatectl", "set-timezone", cfg.Timezone); err != nil {
			// Fallback: write directly (for containers without systemd)
			if err2 := rc.Runner.WriteFile("/etc/timezone", []byte(cfg.Timezone+"\n"), 0644); err2 != nil {
				return nil, fmt.Errorf("setting timezone: %w", err)
			}
			rc.Runner.Run(ctx, "dpkg-reconfigure", "-f", "noninteractive", "tzdata")
		}
		messages = append(messages, fmt.Sprintf("timezone set to %s", cfg.Timezone))
		changed = true
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}
