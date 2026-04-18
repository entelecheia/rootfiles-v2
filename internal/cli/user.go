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

	userCmd.AddCommand(
		newUserAddCmd(),
		newUserListCmd(),
		newUserBackupCmd(),
		newUserRestoreCmd(),
		newUserRehomeCmd(),
		newUserIDCmd(),
		newUserGroupsCmd(),
		newUserGroupAddCmd(),
		newUserGroupDelCmd(),
		newUserPasswdCmd(),
	)

	return userCmd
}

func newUserAddCmd() *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.Flags().String("pubkey", "", "SSH public key")
	cmd.Flags().StringSlice("groups", nil, "Additional groups")
	cmd.Flags().Bool("no-docker", false, "Do not add to docker group")
	return cmd
}

func newUserListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed users (or system users with --system)",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			system, _ := cmd.Flags().GetBool("system")
			namesOnly, _ := cmd.Flags().GetBool("names")
			if system {
				if namesOnly {
					return module.ListSystemUserNames(context.Background(), rc)
				}
				return module.ListSystemUsers(context.Background(), rc)
			}
			if namesOnly {
				return module.ListUserNames(rc)
			}
			return module.ListUsers(rc)
		},
	}
	cmd.Flags().BoolP("system", "s", false, "List system users (UID 1000-65533)")
	cmd.Flags().Bool("names", false, "Print usernames only (one per line)")
	return cmd
}

func newUserBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup user list and metadata to JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			output, _ := cmd.Flags().GetString("output")
			return module.BackupUsers(rc, output)
		},
	}
	cmd.Flags().StringP("output", "o", "", "Output path for backup file")
	return cmd
}

func newUserRestoreCmd() *cobra.Command {
	return &cobra.Command{
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
	}
}

func newUserRehomeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rehome [USERNAME]",
		Short: "Move user home to custom home base directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.RehomeUser(context.Background(), rc, args[0])
		},
	}
}

func newUserIDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "id USERNAME",
		Short: "Show UID, GID, and groups for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.ShowUserID(context.Background(), rc, args[0])
		},
	}
}

func newUserGroupsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "groups [USERNAME]",
		Short: "List groups (all groups or for a specific user)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			if len(args) > 0 {
				return module.ListUserGroups(context.Background(), rc, args[0])
			}
			return module.ListGroups(context.Background(), rc)
		},
	}
}

func newUserGroupAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group-add USERNAME",
		Short: "Add a user to groups",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			groups := collectGroupFlags(cmd)
			if len(groups) == 0 {
				return fmt.Errorf("specify groups with --groups, --docker, or --sudo")
			}
			return module.AddUserToGroups(context.Background(), rc, args[0], groups)
		},
	}
	addGroupSelectionFlags(cmd)
	return cmd
}

func newUserGroupDelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group-del USERNAME",
		Short: "Remove a user from groups",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			groups := collectGroupFlags(cmd)
			if len(groups) == 0 {
				return fmt.Errorf("specify groups with --groups, --docker, or --sudo")
			}
			return module.RemoveUserFromGroups(context.Background(), rc, args[0], groups)
		},
	}
	addGroupSelectionFlags(cmd)
	return cmd
}

// addGroupSelectionFlags registers the --groups / --docker / --sudo flag trio
// shared by group-add and group-del.
func addGroupSelectionFlags(cmd *cobra.Command) {
	cmd.Flags().StringSlice("groups", nil, "Groups to target")
	cmd.Flags().Bool("docker", false, "Include the docker group")
	cmd.Flags().Bool("sudo", false, "Include the sudo group")
}

// collectGroupFlags expands the --groups / --docker / --sudo flags into a single slice.
func collectGroupFlags(cmd *cobra.Command) []string {
	groups, _ := cmd.Flags().GetStringSlice("groups")
	if docker, _ := cmd.Flags().GetBool("docker"); docker {
		groups = append(groups, "docker")
	}
	if sudo, _ := cmd.Flags().GetBool("sudo"); sudo {
		groups = append(groups, "sudo")
	}
	return groups
}

func newUserPasswdCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "passwd [USERNAME...]",
		Short: "Set passwords for users (batch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			suffix, _ := cmd.Flags().GetString("suffix")
			password, _ := cmd.Flags().GetString("password")
			filePath, _ := cmd.Flags().GetString("file")
			all, _ := cmd.Flags().GetBool("all")

			entries, err := resolvePasswordTargets(context.Background(), rc, args, filePath, suffix, all)
			if err != nil {
				return err
			}

			if password != "" {
				for i := range entries {
					entries[i].Password = password
				}
			}

			if len(entries) == 0 {
				return fmt.Errorf("no users to process")
			}

			fmt.Printf("Target users (%d):\n", len(entries))
			for _, e := range entries {
				fmt.Printf("  - %s\n", e.Username)
			}
			ok, err := ui.Confirm("Set passwords for these users?", rc.Yes)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Cancelled.")
				return nil
			}

			return module.SetPasswords(context.Background(), rc, entries, suffix)
		},
	}
	cmd.Flags().String("password", "", "Set the same password for all users")
	cmd.Flags().String("suffix", "!@", "Suffix for auto-generated passwords (username+suffix)")
	cmd.Flags().StringP("file", "f", "", "File with usernames (or username,password per line)")
	cmd.Flags().Bool("all", false, "Set passwords for all system users")
	return cmd
}

// resolvePasswordTargets decides which users the passwd subcommand should
// operate on. Sources (in priority): --file, --all, positional args.
func resolvePasswordTargets(ctx context.Context, rc *module.RunContext, args []string, filePath, suffix string, all bool) ([]module.PasswordEntry, error) {
	switch {
	case filePath != "":
		return module.LoadPasswordFile(filePath, suffix)
	case all:
		users, err := module.ScanSystemUsersExported(ctx, rc)
		if err != nil {
			return nil, fmt.Errorf("scanning system users: %w", err)
		}
		entries := make([]module.PasswordEntry, 0, len(users))
		for _, u := range users {
			entries = append(entries, module.PasswordEntry{Username: u.Name})
		}
		return entries, nil
	case len(args) > 0:
		entries := make([]module.PasswordEntry, 0, len(args))
		for _, name := range args {
			entries = append(entries, module.PasswordEntry{Username: name})
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("specify usernames, --all, or --file")
	}
}
