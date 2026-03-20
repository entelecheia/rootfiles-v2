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
	listCmd.Flags().BoolP("system", "s", false, "List system users (UID 1000-65533)")
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

	// rootfiles user id USERNAME
	userCmd.AddCommand(&cobra.Command{
		Use:   "id USERNAME",
		Short: "Show UID, GID, and groups for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.ShowUserID(context.Background(), rc, args[0])
		},
	})

	// rootfiles user groups [USERNAME]
	groupsCmd := &cobra.Command{
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
	userCmd.AddCommand(groupsCmd)

	// rootfiles user group-add USERNAME --groups ...
	groupAddCmd := &cobra.Command{
		Use:   "group-add USERNAME",
		Short: "Add a user to groups",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			groups, _ := cmd.Flags().GetStringSlice("groups")
			if docker, _ := cmd.Flags().GetBool("docker"); docker {
				groups = append(groups, "docker")
			}
			if sudo, _ := cmd.Flags().GetBool("sudo"); sudo {
				groups = append(groups, "sudo")
			}
			if len(groups) == 0 {
				return fmt.Errorf("specify groups with --groups, --docker, or --sudo")
			}
			return module.AddUserToGroups(context.Background(), rc, args[0], groups)
		},
	}
	groupAddCmd.Flags().StringSlice("groups", nil, "Groups to add the user to")
	groupAddCmd.Flags().Bool("docker", false, "Add to docker group")
	groupAddCmd.Flags().Bool("sudo", false, "Add to sudo group")
	userCmd.AddCommand(groupAddCmd)

	// rootfiles user group-del USERNAME --groups ...
	groupDelCmd := &cobra.Command{
		Use:   "group-del USERNAME",
		Short: "Remove a user from groups",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			groups, _ := cmd.Flags().GetStringSlice("groups")
			if docker, _ := cmd.Flags().GetBool("docker"); docker {
				groups = append(groups, "docker")
			}
			if sudo, _ := cmd.Flags().GetBool("sudo"); sudo {
				groups = append(groups, "sudo")
			}
			if len(groups) == 0 {
				return fmt.Errorf("specify groups with --groups, --docker, or --sudo")
			}
			return module.RemoveUserFromGroups(context.Background(), rc, args[0], groups)
		},
	}
	groupDelCmd.Flags().StringSlice("groups", nil, "Groups to remove the user from")
	groupDelCmd.Flags().Bool("docker", false, "Remove from docker group")
	groupDelCmd.Flags().Bool("sudo", false, "Remove from sudo group")
	userCmd.AddCommand(groupDelCmd)

	// rootfiles user passwd [USERNAME...] --all --file --password --suffix
	passwdCmd := &cobra.Command{
		Use:   "passwd [USERNAME...]",
		Short: "Set passwords for users (batch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			suffix, _ := cmd.Flags().GetString("suffix")
			password, _ := cmd.Flags().GetString("password")
			filePath, _ := cmd.Flags().GetString("file")
			all, _ := cmd.Flags().GetBool("all")

			var entries []module.PasswordEntry

			switch {
			case filePath != "":
				var err error
				entries, err = module.LoadPasswordFile(filePath, suffix)
				if err != nil {
					return err
				}
			case all:
				users, err := module.ScanSystemUsersExported(context.Background(), rc)
				if err != nil {
					return fmt.Errorf("scanning system users: %w", err)
				}
				for _, u := range users {
					entries = append(entries, module.PasswordEntry{Username: u.Name})
				}
			case len(args) > 0:
				for _, name := range args {
					entries = append(entries, module.PasswordEntry{Username: name})
				}
			default:
				return fmt.Errorf("specify usernames, --all, or --file")
			}

			// If --password is set, override all entries
			if password != "" {
				for i := range entries {
					entries[i].Password = password
				}
			}

			if len(entries) == 0 {
				return fmt.Errorf("no users to process")
			}

			// Show target users and confirm
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
	passwdCmd.Flags().String("password", "", "Set the same password for all users")
	passwdCmd.Flags().String("suffix", "!@", "Suffix for auto-generated passwords (username+suffix)")
	passwdCmd.Flags().StringP("file", "f", "", "File with usernames (or username,password per line)")
	passwdCmd.Flags().Bool("all", false, "Set passwords for all system users")
	userCmd.AddCommand(passwdCmd)

	return userCmd
}
