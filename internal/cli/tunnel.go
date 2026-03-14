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

	tunnel.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update cloudflared binary to latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.TunnelUpdate(context.Background(), rc)
		},
	})

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

	sysInfo, _ := config.DetectSystem()
	cfg, _ := config.Load(profileName, "", sysInfo)

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
