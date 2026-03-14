package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
	"github.com/entelecheia/rootfiles-v2/internal/module"
	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

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

	// Select profile
	if profileName == "" && configPath == "" {
		suggested := sysInfo.SuggestProfile()
		if yes {
			profileName = suggested
		} else {
			profileName, err = ui.Select(
				"Select profile",
				config.AvailableProfiles(),
				suggested,
				false,
			)
			if err != nil {
				return err
			}
		}
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

	if !yes && !dryRun {
		confirmed, err := ui.Confirm("Apply this configuration?", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Aborted.")
			return nil
		}
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
