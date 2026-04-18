package cli

import "github.com/spf13/cobra"

func NewRootCmd(version, commit string) *cobra.Command {
	root := &cobra.Command{
		Use:   "rootfiles",
		Short: "Server bootstrapping tool for Ubuntu and DGX OS",
		Long:  "rootfiles-v2: Declarative server configuration management with modular profiles.",
	}
	root.Version = version + " (" + commit + ")"

	// Persistent flags for all subcommands
	root.PersistentFlags().Bool("yes", false, "Unattended mode (skip all prompts)")
	root.PersistentFlags().Bool("dry-run", false, "Show what would be done without executing")
	root.PersistentFlags().String("profile", "", "Profile name (base, minimal, dgx, gpu-server, full)")
	root.PersistentFlags().StringSlice("module", nil, "Run specific modules only")
	root.PersistentFlags().String("config", "", "Path to custom config YAML")

	// Flags for unattended mode
	root.PersistentFlags().String("home-base", "", "Custom home base directory (e.g., /raid/home)")
	root.PersistentFlags().String("user", "", "Username to create")
	root.PersistentFlags().String("ssh-pubkey", "", "SSH public key for the user")
	root.PersistentFlags().String("tunnel-token", "", "Cloudflare tunnel token")
	root.PersistentFlags().String("vlan-address", "", "VLAN private network address (e.g., 172.16.229.32/32)")

	root.AddCommand(newApplyCmd())
	root.AddCommand(newBackupCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newTunnelCmd())
	root.AddCommand(newUpgradeCmd(version))
	root.AddCommand(newUserCmd())
	root.AddCommand(newGPUCmd())

	return root
}

func Execute(version, commit string) error {
	return NewRootCmd(version, commit).Execute()
}
