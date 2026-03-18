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
)

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check current system state against profile",
		Long:  "Check which modules are satisfied and which need changes.",
		RunE:  runCheck,
	}
	cmd.Flags().BoolP("verbose", "v", false, "Show commands that will be executed")
	return cmd
}

func runCheck(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	profileName, _ := cmd.Flags().GetString("profile")
	moduleFilter, _ := cmd.Flags().GetStringSlice("module")
	configPath, _ := cmd.Flags().GetString("config")

	if profileName == "" {
		profileName = os.Getenv("ROOTFILES_PROFILE")
	}

	sysInfo, err := config.DetectSystem()
	if err != nil {
		return fmt.Errorf("detecting system: %w", err)
	}

	if profileName == "" && configPath == "" {
		profileName = sysInfo.SuggestProfile()
	}

	cfg, err := config.Load(profileName, configPath, sysInfo)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	runner := exec.NewRunner(true, logger) // always dry-run for check
	apt := exec.NewAPT(runner)

	registry := module.NewRegistry()
	modules := registry.Resolve(cfg, moduleFilter)

	rc := &module.RunContext{
		Config: cfg,
		Runner: runner,
		APT:    apt,
		DryRun: true,
		Yes:    true,
	}

	results, err := module.CheckAll(ctx, modules, rc)
	if err != nil {
		return err
	}

	// Print report
	fmt.Printf("Profile: %s\n\n", profileName)
	fmt.Printf("%-15s %-10s %s\n", "MODULE", "STATUS", "CHANGES")
	fmt.Printf("%-15s %-10s %s\n", "------", "------", "-------")

	allSatisfied := true
	verbose, _ := cmd.Flags().GetBool("verbose")

	for _, m := range modules {
		r := results[m.Name()]
		status := "OK"
		if !r.Satisfied {
			status = "PENDING"
			allSatisfied = false
		}
		changeCount := len(r.Changes)
		fmt.Printf("%-15s %-10s %d change(s)\n", m.Name(), status, changeCount)
		for _, c := range r.Changes {
			fmt.Printf("  → %s\n", c.Description)
			if verbose && c.Command != "" {
				fmt.Printf("      $ %s\n", c.Command)
			}
		}
	}

	fmt.Println()
	if allSatisfied {
		fmt.Println("All modules satisfied.")
	} else {
		fmt.Println("Run 'rootfiles apply' to apply pending changes.")
	}

	return nil
}
