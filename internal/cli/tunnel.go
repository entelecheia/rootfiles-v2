package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
	"github.com/entelecheia/rootfiles-v2/internal/module"
	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

func newTunnelCmd() *cobra.Command {
	tunnel := &cobra.Command{
		Use:   "tunnel",
		Short: "Manage cloudflared tunnel and VLAN private network",
	}

	tunnel.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install cloudflared binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			m := module.NewCloudflaredModule()
			_, err := m.Apply(context.Background(), rc)
			return err
		},
	})

	tunnel.AddCommand(&cobra.Command{
		Use:   "setup [TOKEN]",
		Short: "Setup tunnel + VLAN private network",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			token := ""
			if len(args) > 0 {
				token = args[0]
			}
			if token == "" {
				token, _ = cmd.Flags().GetString("tunnel-token")
			}
			if token == "" {
				token = os.Getenv("ROOTFILES_TUNNEL_TOKEN")
			}
			vlanAddr, _ := cmd.Flags().GetString("vlan-address")
			if vlanAddr == "" {
				vlanAddr = os.Getenv("ROOTFILES_VLAN_ADDRESS")
			}
			return module.TunnelSetup(context.Background(), rc, token, vlanAddr)
		},
	})

	tunnel.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show tunnel service and VLAN status",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.TunnelStatus(context.Background(), rc)
		},
	})

	tunnel.AddCommand(&cobra.Command{
		Use:   "restart",
		Short: "Restart cloudflared service",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			_, err := rc.Runner.Run(context.Background(), "systemctl", "restart", "cloudflared")
			if err != nil {
				return fmt.Errorf("restarting cloudflared: %w", err)
			}
			fmt.Println("cloudflared service restarted.")
			return nil
		},
	})

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Upgrade cloudflared binary to latest (or a pinned version)",
		Long: `Download the cloudflared binary from the upstream cloudflare/cloudflared GitHub
releases and replace /usr/local/bin/cloudflared. Uses the tagged release URL when
--version is provided, or the /latest/download/ alias otherwise. The cloudflared
systemd service is restarted when present; pure-binary refreshes on hosts that
don't run the tunnel just update the binary.

Use --check to see the current installed version vs. the latest upstream tag
without downloading or restarting anything.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			ctx := context.Background()

			checkOnly, _ := cmd.Flags().GetBool("check")
			version, _ := cmd.Flags().GetString("version")

			if checkOnly {
				return tunnelUpdateCheck(ctx, rc)
			}
			return module.TunnelUpdate(ctx, rc, version)
		},
	}
	updateCmd.Flags().Bool("check", false, "Report current and latest cloudflared version without downloading")
	updateCmd.Flags().String("version", "", "Pin to a specific cloudflared release (e.g. 2024.9.1)")
	tunnel.AddCommand(updateCmd)

	tunnel.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Remove tunnel service, VLAN, and binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.TunnelUninstall(context.Background(), rc)
		},
	})

	return tunnel
}

// tunnelUpdateCheck prints the installed and upstream cloudflared versions
// via the same ui helpers used by `rootfiles status`, without touching the
// binary. Returns a non-nil error only when the upstream lookup fails and
// we have no useful output to produce.
func tunnelUpdateCheck(ctx context.Context, rc *module.RunContext) error {
	latest, err := module.FetchLatestCloudflaredVersion(ctx)
	if err != nil {
		return fmt.Errorf("fetching latest cloudflared release: %w", err)
	}

	ui.WriteSection(os.Stdout, "Cloudflared update check")

	// currentCloudflaredVersion is package-private in module; derive from
	// `cloudflared --version` here using the same logic.
	current := ""
	if rc.Runner.FileExists(cloudflaredPath) {
		if res, err := rc.Runner.Query(ctx, cloudflaredPath, "--version"); err == nil {
			for _, tok := range strings.Fields(res.Stdout) {
				if len(tok) > 3 && tok[0] >= '0' && tok[0] <= '9' && strings.Contains(tok, ".") {
					current = tok
					break
				}
			}
		}
	}
	if current == "" {
		ui.WriteKV(os.Stdout, "Current", ui.StyleHint.Render("(not installed)"))
	} else {
		ui.WriteKV(os.Stdout, "Current", current)
	}
	ui.WriteKV(os.Stdout, "Latest", latest)

	switch {
	case current == "":
		ui.WriteHint(os.Stdout, "run `rootfiles tunnel install` to install.")
	case current == latest:
		fmt.Println("  " + ui.StyleSuccess.Render(ui.MarkOK+" up to date."))
	default:
		ui.WriteHint(os.Stdout, "run `rootfiles tunnel update` to upgrade.")
	}
	return nil
}

// cloudflaredPath mirrors internal/module/cloudflared.go's cloudflaredBinary
// constant. Duplicated here rather than exported to keep the module surface
// minimal; if it ever drifts, the `tunnel status` / `tunnel update --check`
// pair will report inconsistent paths, which scenario tests will catch.
const cloudflaredPath = "/usr/local/bin/cloudflared"

func buildRunContext(cmd *cobra.Command) *module.RunContext {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")
	if os.Getenv("ROOTFILES_YES") == "true" {
		yes = true
	}

	profileName, _ := cmd.Flags().GetString("profile")
	if profileName == "" {
		profileName = os.Getenv("ROOTFILES_PROFILE")
	}
	if profileName == "" {
		profileName = "minimal"
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	runner := exec.NewRunner(dryRun, logger)

	sysInfo, sysErr := config.DetectSystem()
	if sysErr != nil {
		logger.Warn("system detection failed, using defaults", "err", sysErr)
	}
	cfg, cfgErr := config.Load(profileName, "", sysInfo)
	if cfgErr != nil {
		// Falling back to zero-value config would surprise users by silently
		// masking corrupt YAML or a missing profile. Surface it so the subcommand
		// can still proceed (some paths like `tunnel status` tolerate a bare cfg),
		// but the user sees why their profile customizations are not being applied.
		logger.Warn("loading profile config failed, continuing with fallback", "profile", profileName, "err", cfgErr)
	}

	// Apply flag overrides
	applyFlagOverrides(cmd, cfg)

	return &module.RunContext{
		Config: cfg,
		Runner: runner,
		APT:    exec.NewAPT(runner),
		DryRun: dryRun,
		Yes:    yes,
	}
}
