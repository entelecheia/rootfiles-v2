package cli

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
	"github.com/entelecheia/rootfiles-v2/internal/module"
	"github.com/entelecheia/rootfiles-v2/internal/state"
	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose remote access, disks, reboot, time sync, GPU and systemd health",
		Long: "Read-only health checks of the live system (as opposed to `check`, which compares against\n" +
			"the profile). Exit code: 0 = no failures, 2 = at least one failure (or warning with --strict).",
		RunE: runDoctor,
	}
	addOutputFlag(cmd)
	cmd.Flags().Bool("strict", false, "Treat warnings as failures for the exit code")
	return cmd
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	sys, _ := config.DetectSystem()
	if sys == nil {
		sys = &config.SystemInfo{}
	}
	profile, configPath := resolveTarget(cmd, sys)
	cfg, err := config.Load(profile, configPath, sys)
	if err != nil {
		cfg = &config.Config{System: sys}
	}

	runner := exec.NewRunner(true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rc := &module.RunContext{Config: cfg, Runner: runner, APT: exec.NewAPT(runner), DryRun: true, Yes: true}
	findings := module.Doctor(cmd.Context(), rc)

	if last, err := state.Last(); err == nil && last != nil && !last.Success {
		findings = append(findings, module.Finding{Check: "last apply", Level: module.LevelWarn,
			Detail: "last apply (" + last.FinishedAt.Local().Format("2006-01-02 15:04") + ") failed",
			Hint:   "rootfiles status"})
	}

	strict, _ := cmd.Flags().GetBool("strict")
	failed := 0
	for _, f := range findings {
		if f.Level == module.LevelFail || (strict && f.Level == module.LevelWarn) {
			failed++
		}
	}

	out := cmd.OutOrStdout()
	if format == "json" {
		if err := writeJSON(out, map[string]any{"findings": findings, "failed": failed}); err != nil {
			return err
		}
	} else {
		ui.WriteHeader(out, "rootfiles doctor")
		for _, f := range findings {
			marker := ui.OKMark()
			switch f.Level {
			case module.LevelWarn:
				marker = ui.WarnMark()
			case module.LevelFail:
				marker = ui.StyleError.Render(ui.MarkFail)
			case module.LevelSkip:
				marker = ui.StyleHint.Render("-")
			}
			ui.WriteBullet(out, marker, fmt.Sprintf("%-16s %s", f.Check, f.Detail))
			if f.Hint != "" && f.Level != module.LevelOK {
				fmt.Fprintf(out, "      %s\n", ui.StyleHint.Render("→ "+f.Hint))
			}
		}
		fmt.Fprintln(out)
	}
	if failed > 0 {
		return &ExitError{Code: exitDrift}
	}
	return nil
}
