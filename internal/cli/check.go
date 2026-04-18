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

	out := cmd.OutOrStdout()
	verbose, _ := cmd.Flags().GetBool("verbose")

	satisfied := 0
	for _, m := range modules {
		if r := results[m.Name()]; r != nil && r.Satisfied {
			satisfied++
		}
	}

	ui.WriteHeader(out, "rootfiles check")
	ui.WriteKV(out, "Profile", profileName)
	ui.WriteSection(out, fmt.Sprintf("Modules (%d/%d satisfied)", satisfied, len(modules)))

	for _, m := range modules {
		r := results[m.Name()]
		marker := ui.OKMark()
		statusLabel := ui.StyleSuccess.Render("OK")
		changeCount := 0
		if r != nil {
			changeCount = len(r.Changes)
		}
		if r == nil || !r.Satisfied {
			marker = ui.PendingMark()
			statusLabel = ui.StyleWarning.Render("PENDING")
		}
		ui.WriteBullet(out, marker, fmt.Sprintf("%-15s %s  %d change(s)", m.Name(), statusLabel, changeCount))
		if r == nil {
			continue
		}
		for _, c := range r.Changes {
			fmt.Fprintf(out, "        %s %s\n", ui.PendingMark(), c.Description)
			if verbose && c.Command != "" {
				fmt.Fprintf(out, "          %s\n", ui.StyleHint.Render("$ "+c.Command))
			}
		}
	}

	fmt.Fprintln(out)
	if satisfied == len(modules) {
		fmt.Fprintln(out, "  "+ui.StyleSuccess.Render(ui.MarkOK+" all modules satisfied."))
	} else {
		ui.WriteHint(out, "run 'rootfiles apply' to apply pending changes.")
	}

	return nil
}
