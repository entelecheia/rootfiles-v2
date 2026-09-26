package cli

import (
	"fmt"
	"io"
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
		Long: "Check which modules are satisfied and which need changes.\n" +
			"Exit code: 0 = all satisfied, 2 = pending changes, 1 = error. Without --profile/--config,\n" +
			"the last applied profile (or the detected suggestion) is used.",
		RunE: runCheck,
	}
	cmd.Flags().BoolP("verbose", "v", false, "Show commands that will be executed")
	addOutputFlag(cmd)
	return cmd
}

func runCheck(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	moduleFilter, _ := cmd.Flags().GetStringSlice("module")
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}

	sysInfo, err := config.DetectSystem()
	if err != nil {
		return fmt.Errorf("detecting system: %w", err)
	}
	profileName, configPath := resolveTarget(cmd, sysInfo)

	cfg, err := config.Load(profileName, configPath, sysInfo)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	applyFlagOverrides(cmd, cfg)
	applyAccountFlags(cmd, cfg)
	if err := cfg.Validate(); err != nil {
		return err
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
	allOK := satisfied == len(modules)

	if format == "json" {
		report := checkReport{Profile: profileName, ConfigPath: configPath, Satisfied: allOK}
		for _, m := range modules {
			mr := checkModule{Name: m.Name(), Satisfied: true, Changes: []module.Change{}}
			if r := results[m.Name()]; r != nil {
				mr.Satisfied = r.Satisfied
				if r.Changes != nil {
					mr.Changes = r.Changes
				}
			}
			report.Modules = append(report.Modules, mr)
		}
		if err := writeJSON(out, report); err != nil {
			return err
		}
	} else {
		renderCheckText(out, profileName, configPath, modules, results, satisfied, verbose)
	}

	if !allOK {
		return &ExitError{Code: exitDrift}
	}
	return nil
}

type checkModule struct {
	Name      string          `json:"name"`
	Satisfied bool            `json:"satisfied"`
	Changes   []module.Change `json:"changes"`
}

type checkReport struct {
	Profile    string        `json:"profile,omitempty"`
	ConfigPath string        `json:"config_path,omitempty"`
	Satisfied  bool          `json:"satisfied"`
	Modules    []checkModule `json:"modules"`
}

func renderCheckText(out io.Writer, profileName, configPath string, modules []module.Module, results map[string]*module.CheckResult, satisfied int, verbose bool) {
	ui.WriteHeader(out, "rootfiles check")
	if configPath != "" {
		ui.WriteKV(out, "Config", configPath)
	} else {
		ui.WriteKV(out, "Profile", profileName)
	}
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
		ui.WriteHint(out, "run 'rootfiles apply' to apply pending changes (exit code 2 = pending).")
	}
}
