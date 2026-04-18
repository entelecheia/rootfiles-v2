package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
	"github.com/entelecheia/rootfiles-v2/internal/module"
	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

var errAborted = errors.New("aborted by user")

func newApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Apply server configuration",
		Long:  "Apply the selected profile's configuration to the system.",
		RunE:  runApply,
	}
}

func runApply(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	yes, _ := cmd.Flags().GetBool("yes")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	profileName, _ := cmd.Flags().GetString("profile")
	moduleFilter, _ := cmd.Flags().GetStringSlice("module")
	configPath, _ := cmd.Flags().GetString("config")

	// Check ROOTFILES_YES env
	if os.Getenv("ROOTFILES_YES") == "true" {
		yes = true
	}
	// Check ROOTFILES_PROFILE env
	if profileName == "" {
		profileName = os.Getenv("ROOTFILES_PROFILE")
	}

	// Detect system
	sysInfo, err := config.DetectSystem()
	if err != nil {
		return fmt.Errorf("detecting system: %w", err)
	}

	profileName, err = selectProfile(profileName, configPath, sysInfo, yes)
	if err != nil {
		return err
	}

	// Load config
	cfg, err := config.Load(profileName, configPath, sysInfo)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply CLI flag overrides
	applyFlagOverrides(cmd, cfg)

	// Setup runner
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	runner := exec.NewRunner(dryRun, logger)
	apt := exec.NewAPT(runner)

	// Build module list
	registry := module.NewRegistry()
	modules := registry.Resolve(cfg, moduleFilter)

	if len(modules) == 0 {
		fmt.Println("No modules to apply.")
		return nil
	}

	// Show plan
	fmt.Printf("Profile: %s\n", profileName)
	if dryRun {
		fmt.Println("Mode: dry-run (no changes will be made)")
	}
	fmt.Printf("Modules: ")
	for i, m := range modules {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(m.Name())
	}
	fmt.Println()

	// Interactive configuration preview & edit
	if err := configureInteractive(cfg, yes, dryRun); err != nil {
		if errors.Is(err, errAborted) {
			fmt.Println("Aborted.")
			return nil
		}
		return err
	}

	// Execute modules
	rc := &module.RunContext{
		Config: cfg,
		Runner: runner,
		APT:    apt,
		DryRun: dryRun,
		Yes:    yes,
	}

	fmt.Println()
	return module.RunAll(ctx, modules, rc)
}

// selectProfile resolves the profile name. Priority: explicit --profile flag
// or --config path (no prompt), then system suggestion. In non-interactive
// (--yes) mode the suggestion is used silently; otherwise the user picks from
// the list of available profiles with the suggestion pre-selected.
func selectProfile(profileName, configPath string, sysInfo *config.SystemInfo, yes bool) (string, error) {
	if profileName != "" || configPath != "" {
		return profileName, nil
	}
	suggested := sysInfo.SuggestProfile()
	if yes {
		return suggested, nil
	}
	return ui.Select("Select profile", config.AvailableProfiles(), suggested, false)
}

func applyFlagOverrides(cmd *cobra.Command, cfg *config.Config) {
	if v, _ := cmd.Flags().GetString("home-base"); v != "" {
		cfg.Users.HomeBase = v
	}
	if v, _ := cmd.Flags().GetString("tunnel-token"); v != "" {
		cfg.Modules.Cloudflared.TunnelToken = v
	}
	if v, _ := cmd.Flags().GetString("vlan-address"); v != "" {
		cfg.Modules.Cloudflared.PrivateNetwork.Address = v
	}
}

// configureInteractive shows a preview of all key settings and lets the user
// review/modify them before applying. Skipped when --yes is set.
func configureInteractive(cfg *config.Config, yes, dryRun bool) error {
	if yes {
		return nil
	}

	fmt.Println("\n=== Configuration ===")

	var err error

	// --- General ---
	cfg.Timezone, err = ui.Input("Timezone", cfg.Timezone, false)
	if err != nil {
		return err
	}

	// --- SSH ---
	if cfg.IsModuleEnabled("ssh") {
		fmt.Println("\n--- SSH ---")
		cfg.SSH.DisableRootLogin, err = ui.Confirm("Disable root login?", false)
		if err != nil {
			return err
		}
		cfg.SSH.DisablePasswordAuth, err = ui.Confirm("Disable password authentication?", false)
		if err != nil {
			return err
		}
		port := cfg.SSH.Port
		if port == 0 {
			port = 22
		}
		cfg.SSH.Port, err = ui.InputInt("SSH port", port, false)
		if err != nil {
			return err
		}
	}

	// --- Users ---
	if cfg.IsModuleEnabled("users") {
		fmt.Println("\n--- Users ---")
		if cfg.Users.HomeBase != "" {
			cfg.Users.HomeBase, err = ui.Input("Home base directory", cfg.Users.HomeBase, false)
			if err != nil {
				return err
			}
		}
		cfg.Users.SudoNopasswd, err = ui.Confirm("Sudo without password?", false)
		if err != nil {
			return err
		}
	}

	// --- Docker ---
	if cfg.IsModuleEnabled("docker") {
		fmt.Println("\n--- Docker ---")
		if cfg.Modules.Docker.StorageDir != "" {
			cfg.Modules.Docker.StorageDir, err = ui.Input("Docker storage directory", cfg.Modules.Docker.StorageDir, false)
			if err != nil {
				return err
			}
		}
	}

	// --- Cloudflared ---
	if cfg.IsModuleEnabled("cloudflared") {
		fmt.Println("\n--- Cloudflared ---")
		cfg.Modules.Cloudflared.TunnelToken, err = ui.Input("Tunnel token (empty to skip)", cfg.Modules.Cloudflared.TunnelToken, false)
		if err != nil {
			return err
		}
		cfg.Modules.Cloudflared.PrivateNetwork.Enabled, err = ui.Confirm("Enable VLAN private network?", false)
		if err != nil {
			return err
		}
		if cfg.Modules.Cloudflared.PrivateNetwork.Enabled {
			cfg.Modules.Cloudflared.PrivateNetwork.Address, err = ui.Input("VLAN address (e.g. 172.16.229.32/32)", cfg.Modules.Cloudflared.PrivateNetwork.Address, false)
			if err != nil {
				return err
			}
		}
	}

	// --- Network ---
	if cfg.IsModuleEnabled("network") {
		fmt.Println("\n--- Network ---")
		cfg.Modules.Network.UFW, err = ui.Confirm("Enable UFW firewall?", false)
		if err != nil {
			return err
		}
		if cfg.Modules.Network.UFW {
			cfg.Modules.Network.AllowedPorts, err = ui.InputIntSlice("Allowed ports (comma-separated)", cfg.Modules.Network.AllowedPorts, false)
			if err != nil {
				return err
			}
		}
	}

	// --- Storage ---
	if cfg.IsModuleEnabled("storage") {
		fmt.Println("\n--- Storage ---")
		if cfg.Modules.Storage.DataDir != "" {
			cfg.Modules.Storage.DataDir, err = ui.Input("Data directory", cfg.Modules.Storage.DataDir, false)
			if err != nil {
				return err
			}
		}
	}

	// --- Summary & confirm ---
	fmt.Println("\n=== Summary ===")
	fmt.Printf("  Timezone: %s\n", cfg.Timezone)
	if cfg.IsModuleEnabled("ssh") {
		fmt.Printf("  SSH: root_login=%v, password_auth=%v, port=%d\n",
			!cfg.SSH.DisableRootLogin, !cfg.SSH.DisablePasswordAuth, cfg.SSH.Port)
	}
	if cfg.IsModuleEnabled("users") && cfg.Users.HomeBase != "" {
		fmt.Printf("  Users: home_base=%s, sudo_nopasswd=%v\n", cfg.Users.HomeBase, cfg.Users.SudoNopasswd)
	}
	if cfg.IsModuleEnabled("docker") && cfg.Modules.Docker.StorageDir != "" {
		fmt.Printf("  Docker: storage=%s\n", cfg.Modules.Docker.StorageDir)
	}
	if cfg.IsModuleEnabled("cloudflared") {
		token := cfg.Modules.Cloudflared.TunnelToken
		if token != "" && len(token) > 8 {
			token = token[:8] + "..."
		}
		fmt.Printf("  Cloudflared: token=%s, vlan=%v", token, cfg.Modules.Cloudflared.PrivateNetwork.Enabled)
		if cfg.Modules.Cloudflared.PrivateNetwork.Enabled {
			fmt.Printf(" (%s)", cfg.Modules.Cloudflared.PrivateNetwork.Address)
		}
		fmt.Println()
	}
	if cfg.IsModuleEnabled("network") {
		fmt.Printf("  Network: ufw=%v, ports=%v\n", cfg.Modules.Network.UFW, cfg.Modules.Network.AllowedPorts)
	}
	if cfg.IsModuleEnabled("storage") && cfg.Modules.Storage.DataDir != "" {
		fmt.Printf("  Storage: data_dir=%s\n", cfg.Modules.Storage.DataDir)
	}

	if dryRun {
		return nil
	}

	confirmed, err := ui.Confirm("\nApply this configuration?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return errAborted
	}
	return nil
}
