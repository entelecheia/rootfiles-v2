package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/module"
)

func newBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Full system backup for OS upgrade / restore",
		Long:  "Captures system info, user metadata, config files, and a rootfiles-compatible config snapshot.",
		RunE: func(cmd *cobra.Command, args []string) error {
			outputBase, _ := cmd.Flags().GetString("output")
			skipDocker, _ := cmd.Flags().GetBool("skip-docker")
			skipEtc, _ := cmd.Flags().GetBool("skip-etc")

			rc := buildRunContext(cmd)
			ctx := context.Background()

			hostname, _ := os.Hostname()
			dirName := fmt.Sprintf("rootfiles-backup-%s-%s", hostname, time.Now().Format("20060102"))
			backupDir := filepath.Join(outputBase, dirName)

			if err := os.MkdirAll(backupDir, 0755); err != nil {
				return fmt.Errorf("creating backup directory: %w", err)
			}
			fmt.Printf("Backup directory: %s\n\n", backupDir)

			var errors []string

			// 1. system-info.json
			fmt.Print("  system-info.json ... ")
			if err := backupSystemInfo(backupDir, hostname); err != nil {
				errors = append(errors, fmt.Sprintf("system-info: %v", err))
				fmt.Println("FAIL")
			} else {
				fmt.Println("OK")
			}

			// 2. users.json
			fmt.Print("  users.json ... ")
			if err := backupUsersJSON(rc, backupDir); err != nil {
				errors = append(errors, fmt.Sprintf("users: %v", err))
				fmt.Println("SKIP (no user database)")
			} else {
				fmt.Println("OK")
			}

			// 3. etc-config.tar.gz
			if !skipEtc {
				fmt.Print("  etc-config.tar.gz ... ")
				if err := backupEtcConfig(ctx, rc, backupDir); err != nil {
					errors = append(errors, fmt.Sprintf("etc-config: %v", err))
					fmt.Println("FAIL")
				} else {
					fmt.Println("OK")
				}
			}

			// 4. crontab-root.txt
			fmt.Print("  crontab-root.txt ... ")
			if err := backupCrontab(ctx, rc, backupDir); err != nil {
				errors = append(errors, fmt.Sprintf("crontab: %v", err))
				fmt.Println("SKIP (no crontab)")
			} else {
				fmt.Println("OK")
			}

			// 5. root-ssh.tar.gz
			fmt.Print("  root-ssh.tar.gz ... ")
			if err := backupRootSSH(ctx, rc, backupDir); err != nil {
				errors = append(errors, fmt.Sprintf("root-ssh: %v", err))
				fmt.Println("SKIP")
			} else {
				fmt.Println("OK")
			}

			// 6. usr-local-bin.tar.gz
			fmt.Print("  usr-local-bin.tar.gz ... ")
			if err := backupUsrLocalBin(ctx, rc, backupDir); err != nil {
				errors = append(errors, fmt.Sprintf("usr-local-bin: %v", err))
				fmt.Println("FAIL")
			} else {
				fmt.Println("OK")
			}

			// 7. docker-images.txt
			if !skipDocker {
				fmt.Print("  docker-images.txt ... ")
				if err := backupDockerImages(ctx, rc, backupDir); err != nil {
					errors = append(errors, fmt.Sprintf("docker-images: %v", err))
					fmt.Println("SKIP (docker not available)")
				} else {
					fmt.Println("OK")
				}
			}

			// 8. config-snapshot.yaml
			fmt.Print("  config-snapshot.yaml ... ")
			if err := backupConfigSnapshot(backupDir); err != nil {
				errors = append(errors, fmt.Sprintf("config-snapshot: %v", err))
				fmt.Println("FAIL")
			} else {
				fmt.Println("OK")
			}

			fmt.Println()
			if len(errors) > 0 {
				fmt.Printf("Backup completed with %d warning(s):\n", len(errors))
				for _, e := range errors {
					fmt.Printf("  - %s\n", e)
				}
			} else {
				fmt.Println("Backup completed successfully.")
			}
			fmt.Printf("\nRestore config with:\n  rootfiles apply --config %s/config-snapshot.yaml --dry-run\n", backupDir)
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "/raid/backup", "Backup output directory")
	cmd.Flags().Bool("skip-docker", false, "Skip Docker image list")
	cmd.Flags().Bool("skip-etc", false, "Skip /etc/ config tar")
	return cmd
}

// backupSystemInfo writes system detection results + hostname.
func backupSystemInfo(backupDir, hostname string) error {
	sysInfo, err := config.DetectSystem()
	if err != nil {
		return err
	}
	info := struct {
		Hostname string `json:"hostname"`
		*config.SystemInfo
	}{
		Hostname:   hostname,
		SystemInfo: sysInfo,
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backupDir, "system-info.json"), data, 0644)
}

// backupUsersJSON backs up user metadata (managed + system users).
func backupUsersJSON(rc *module.RunContext, backupDir string) error {
	outputPath := filepath.Join(backupDir, "users.json")
	return module.BackupUsers(rc, outputPath)
}

// backupEtcConfig creates a tar.gz of key /etc/ config files.
func backupEtcConfig(ctx context.Context, rc *module.RunContext, backupDir string) error {
	outPath := filepath.Join(backupDir, "etc-config.tar.gz")
	// Collect paths that exist
	candidates := []string{
		"/etc/ssh/sshd_config.d/",
		"/etc/docker/daemon.json",
		"/etc/systemd/network/",
		"/etc/ufw/",
		"/etc/fstab",
		"/etc/netplan/",
		"/etc/default/locale",
		"/etc/timezone",
		"/etc/default/useradd",
	}
	var existing []string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return fmt.Errorf("no /etc/ config files found")
	}
	args := append([]string{"czf", outPath}, existing...)
	_, err := rc.Runner.Run(ctx, "tar", args...)
	return err
}

// backupCrontab saves root's crontab.
func backupCrontab(ctx context.Context, rc *module.RunContext, backupDir string) error {
	result, err := rc.Runner.Run(ctx, "crontab", "-l")
	if err != nil {
		return err
	}
	if result.Stdout == "" {
		return fmt.Errorf("empty crontab")
	}
	return os.WriteFile(filepath.Join(backupDir, "crontab-root.txt"), []byte(result.Stdout), 0644)
}

// backupRootSSH archives /root/.ssh/.
func backupRootSSH(ctx context.Context, rc *module.RunContext, backupDir string) error {
	if _, err := os.Stat("/root/.ssh"); err != nil {
		return fmt.Errorf("/root/.ssh not found")
	}
	outPath := filepath.Join(backupDir, "root-ssh.tar.gz")
	_, err := rc.Runner.Run(ctx, "tar", "czf", outPath, "/root/.ssh/")
	return err
}

// backupUsrLocalBin archives /usr/local/bin/.
func backupUsrLocalBin(ctx context.Context, rc *module.RunContext, backupDir string) error {
	if _, err := os.Stat("/usr/local/bin"); err != nil {
		return fmt.Errorf("/usr/local/bin not found")
	}
	outPath := filepath.Join(backupDir, "usr-local-bin.tar.gz")
	_, err := rc.Runner.Run(ctx, "tar", "czf", outPath, "/usr/local/bin/")
	return err
}

// backupDockerImages saves a list of Docker images.
func backupDockerImages(ctx context.Context, rc *module.RunContext, backupDir string) error {
	result, err := rc.Runner.Run(ctx, "docker", "images", "--format", "{{.Repository}}:{{.Tag}}\t{{.Size}}\t{{.ID}}")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backupDir, "docker-images.txt"), []byte(result.Stdout), 0644)
}

// backupConfigSnapshot generates a rootfiles YAML config from current system state.
func backupConfigSnapshot(backupDir string) error {
	cfg, err := config.SnapshotCurrent()
	if err != nil {
		return err
	}
	data, err := config.MarshalYAML(cfg)
	if err != nil {
		return err
	}
	header := "# rootfiles config snapshot — generated by `rootfiles backup`\n# Use with: rootfiles apply --config <this-file> [--dry-run]\n\n"
	return os.WriteFile(filepath.Join(backupDir, "config-snapshot.yaml"), []byte(header+string(data)), 0644)
}
