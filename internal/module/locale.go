package module

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LocaleModule struct{}

func NewLocaleModule() *LocaleModule { return &LocaleModule{} }
func (m *LocaleModule) Name() string { return "locale" }

// Paths are overridable in tests.
var (
	defaultLocalePath = "/etc/default/locale"
	timezonePath      = "/etc/timezone"
	localtimePath     = "/etc/localtime"
	zoneinfoDir       = "/usr/share/zoneinfo"
	timedatectlBin    = "timedatectl"
)

// currentLocale returns the LANG value from /etc/default/locale.
func currentLocale(rc *RunContext) string {
	data, _ := rc.Runner.ReadFile(defaultLocalePath)
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "LANG="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// currentTimezone resolves the system timezone from the /etc/localtime
// symlink (authoritative on systemd hosts, where /etc/timezone may be
// missing), falling back to /etc/timezone.
func currentTimezone(rc *RunContext) string {
	if target, err := os.Readlink(localtimePath); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return target[i+len("zoneinfo/"):]
		}
	}
	data, _ := rc.Runner.ReadFile(timezonePath)
	return strings.TrimSpace(string(data))
}

func (m *LocaleModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config

	if cfg.Locale != "" && currentLocale(rc) != cfg.Locale {
		changes = append(changes, Change{
			Description: fmt.Sprintf("Set locale to %s", cfg.Locale),
			Command:     fmt.Sprintf("locale-gen %s && update-locale LANG=%s", cfg.Locale, cfg.Locale),
		})
	}

	if cfg.Timezone != "" && currentTimezone(rc) != cfg.Timezone {
		changes = append(changes, Change{
			Description: fmt.Sprintf("Set timezone to %s", cfg.Timezone),
			Command:     fmt.Sprintf("timedatectl set-timezone %s", cfg.Timezone),
		})
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *LocaleModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	cfg := rc.Config
	var messages, warnings []string
	changed := false

	if cfg.Locale != "" && currentLocale(rc) != cfg.Locale {
		// locale-gen comes from the locales package.
		if !rc.Runner.CommandExists("locale-gen") {
			if err := rc.APT.Update(ctx); err != nil {
				return nil, fmt.Errorf("apt update: %w", err)
			}
			if err := rc.APT.Install(ctx, []string{"locales"}); err != nil {
				return nil, fmt.Errorf("installing locales: %w", err)
			}
		}
		if _, err := rc.Runner.Run(ctx, "locale-gen", cfg.Locale); err != nil {
			warnings = append(warnings, fmt.Sprintf("locale-gen %s: %s", cfg.Locale, firstLine(err.Error())))
		}
		// Only LANG is set: forcing LC_ALL system-wide overrides every
		// per-user locale choice.
		if err := setDefaultLocale(rc, cfg.Locale); err != nil {
			return nil, fmt.Errorf("writing locale: %w", err)
		}
		messages = append(messages, fmt.Sprintf("locale set to %s", cfg.Locale))
		changed = true
	}

	if cfg.Timezone != "" && currentTimezone(rc) != cfg.Timezone {
		if !rc.DryRun && !rc.Runner.FileExists(filepath.Join(zoneinfoDir, cfg.Timezone)) {
			return nil, fmt.Errorf("unknown timezone %q (not in %s)", cfg.Timezone, zoneinfoDir)
		}
		if _, err := rc.Runner.Run(ctx, timedatectlBin, "set-timezone", cfg.Timezone); err != nil {
			// No systemd (containers): set the symlink and /etc/timezone directly.
			if err := rc.Runner.Remove(localtimePath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("replacing %s: %w", localtimePath, err)
			}
			if err := rc.Runner.Symlink(filepath.Join(zoneinfoDir, cfg.Timezone), localtimePath); err != nil {
				return nil, fmt.Errorf("linking %s: %w", localtimePath, err)
			}
		}
		if err := rc.Runner.WriteFile(timezonePath, []byte(cfg.Timezone+"\n"), 0644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", timezonePath, err)
		}
		messages = append(messages, fmt.Sprintf("timezone set to %s", cfg.Timezone))
		changed = true
	}

	return &ApplyResult{Changed: changed, Messages: messages, Warnings: warnings}, nil
}

// setDefaultLocale sets LANG in /etc/default/locale, keeping other lines
// but dropping a stale LC_ALL that earlier versions wrote.
func setDefaultLocale(rc *RunContext, locale string) error {
	data, _ := rc.Runner.ReadFile(defaultLocalePath)
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "LANG=") || strings.HasPrefix(t, "LC_ALL=") {
			continue
		}
		lines = append(lines, line)
	}
	lines = append([]string{"LANG=" + locale}, lines...)
	return rc.Runner.WriteFile(defaultLocalePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
