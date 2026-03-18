package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/module"
	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

func newUserCmd() *cobra.Command {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Manage system users (custom home, backup, restore)",
	}

	addCmd := &cobra.Command{
		Use:   "add [USERNAME]",
		Short: "Create a user with custom home directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			username := ""
			if len(args) > 0 {
				username = args[0]
			}
			if username == "" {
				username, _ = cmd.Flags().GetString("user")
			}
			if username == "" {
				username = os.Getenv("ROOTFILES_USER")
			}
			if username == "" {
				var err error
				username, err = ui.Input("Username", "", rc.Yes)
				if err != nil || username == "" {
					return fmt.Errorf("username is required")
				}
			}

			pubkey, _ := cmd.Flags().GetString("pubkey")
			if pubkey == "" {
				pubkey, _ = cmd.Flags().GetString("ssh-pubkey")
			}
			if pubkey == "" {
				pubkey = os.Getenv("ROOTFILES_SSH_PUBKEY")
			}

			groups, _ := cmd.Flags().GetStringSlice("groups")
			noDocker, _ := cmd.Flags().GetBool("no-docker")

			return module.AddUser(context.Background(), rc, username, pubkey, groups, noDocker)
		},
	}
	addCmd.Flags().String("pubkey", "", "SSH public key")
	addCmd.Flags().StringSlice("groups", nil, "Additional groups")
	addCmd.Flags().Bool("no-docker", false, "Do not add to docker group")
	userCmd.AddCommand(addCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List managed users",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			namesOnly, _ := cmd.Flags().GetBool("names")
			if namesOnly {
				return module.ListUserNames(rc)
			}
			return module.ListUsers(rc)
		},
	}
	listCmd.Flags().Bool("names", false, "Print usernames only (one per line)")
	userCmd.AddCommand(listCmd)

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup user list and metadata to JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			output, _ := cmd.Flags().GetString("output")
			return module.BackupUsers(rc, output)
		},
	}
	backupCmd.Flags().StringP("output", "o", "", "Output path for backup file")
	userCmd.AddCommand(backupCmd)

	userCmd.AddCommand(&cobra.Command{
		Use:   "restore [BACKUP_FILE]",
		Short: "Restore users from backup (reconnect existing home dirs)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			backupPath := ""
			if len(args) > 0 {
				backupPath = args[0]
			}
			return module.RestoreUsers(context.Background(), rc, backupPath)
		},
	})

	userCmd.AddCommand(&cobra.Command{
		Use:   "rehome [USERNAME]",
		Short: "Move user home to custom home base directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.RehomeUser(context.Background(), rc, args[0])
		},
	})

	return userCmd
}
