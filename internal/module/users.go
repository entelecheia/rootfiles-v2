package module

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type UsersModule struct{}

func NewUsersModule() *UsersModule { return &UsersModule{} }
func (m *UsersModule) Name() string { return "users" }

// UserMeta stores metadata for a managed user.
type UserMeta struct {
	Name         string   `json:"name"`
	UID          int      `json:"uid"`
	GID          int      `json:"gid"`
	Shell        string   `json:"shell"`
	Groups       []string `json:"groups"`
	SudoNopasswd bool     `json:"sudo_nopasswd"`
	SSHPubkeys   []string `json:"ssh_pubkeys,omitempty"`
	CreatedAt    string   `json:"created_at"`
	Home         string   `json:"home"`
}

// UsersDB is the metadata file format.
type UsersDB struct {
	Version   int        `json:"version"`
	HomeBase  string     `json:"home_base"`
	CreatedBy string     `json:"created_by"`
	Users     []UserMeta `json:"users"`
}

func (m *UsersModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config.Users

	if cfg.HomeBase != "" && cfg.HomeBase != "/home" {
		if !rc.Runner.FileExists(cfg.HomeBase) {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Create custom home base directory %s", cfg.HomeBase),
				Command:     fmt.Sprintf("mkdir -p %s", cfg.HomeBase),
			})
		}
		metaDir := filepath.Join(cfg.HomeBase, ".rootfiles")
		if !rc.Runner.FileExists(metaDir) {
			changes = append(changes, Change{
				Description: "Create rootfiles metadata directory",
				Command:     fmt.Sprintf("mkdir -p %s", metaDir),
			})
		}
	}

	// Check /etc/default/useradd HOME setting
	if cfg.HomeBase != "" && cfg.HomeBase != "/home" {
		data, _ := rc.Runner.ReadFile("/etc/default/useradd")
		if !strings.Contains(string(data), "HOME="+cfg.HomeBase) {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Set default useradd HOME to %s", cfg.HomeBase),
			})
		}
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *UsersModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	cfg := rc.Config.Users
	var messages []string
	changed := false

	// Create custom home base
	if cfg.HomeBase != "" && cfg.HomeBase != "/home" {
		if err := rc.Runner.MkdirAll(cfg.HomeBase, 0755); err != nil {
			return nil, fmt.Errorf("creating home base: %w", err)
		}
		metaDir := filepath.Join(cfg.HomeBase, ".rootfiles")
		if err := rc.Runner.MkdirAll(metaDir, 0755); err != nil {
			return nil, fmt.Errorf("creating metadata dir: %w", err)
		}
		messages = append(messages, fmt.Sprintf("home base %s ready", cfg.HomeBase))
		changed = true
	}

	// Update /etc/default/useradd
	if cfg.HomeBase != "" && cfg.HomeBase != "/home" {
		content := fmt.Sprintf("HOME=%s\n", cfg.HomeBase)
		data, _ := rc.Runner.ReadFile("/etc/default/useradd")
		if !strings.Contains(string(data), "HOME="+cfg.HomeBase) {
			// Replace or append HOME= line
			lines := strings.Split(string(data), "\n")
			var newLines []string
			found := false
			for _, line := range lines {
				if strings.HasPrefix(line, "HOME=") {
					newLines = append(newLines, "HOME="+cfg.HomeBase)
					found = true
				} else {
					newLines = append(newLines, line)
				}
			}
			if !found {
				newLines = append(newLines, "HOME="+cfg.HomeBase)
			}
			content = strings.Join(newLines, "\n")
			rc.Runner.WriteFile("/etc/default/useradd", []byte(content), 0644)
			messages = append(messages, "updated /etc/default/useradd")
		}
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}

// AddUser creates a user with the given config. Called from CLI `rootfiles user add`.
func AddUser(ctx context.Context, rc *RunContext, username, pubkey string, extraGroups []string, noDocker bool) error {
	cfg := rc.Config.Users
	homeBase := cfg.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}
	homeDir := filepath.Join(homeBase, username)
	shell := cfg.DefaultShell
	if shell == "" {
		shell = "/usr/bin/zsh"
	}

	// Check if user exists
	if _, err := user.Lookup(username); err == nil {
		return fmt.Errorf("user %s already exists", username)
	}

	// Create user
	args := []string{
		"--home-dir", homeDir,
		"--create-home",
		"--shell", shell,
	}
	if _, err := rc.Runner.Run(ctx, "useradd", append(args, username)...); err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	// Build group list
	groups := cfg.DefaultGroups
	if len(extraGroups) > 0 {
		groups = append(groups, extraGroups...)
	}
	if noDocker {
		var filtered []string
		for _, g := range groups {
			if g != "docker" {
				filtered = append(filtered, g)
			}
		}
		groups = filtered
	}

	// Add to groups
	if len(groups) > 0 {
		rc.Runner.Run(ctx, "usermod", "-aG", strings.Join(groups, ","), username)
	}

	// Sudoers
	if cfg.SudoNopasswd {
		sudoContent := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL\n", username)
		sudoPath := fmt.Sprintf("/etc/sudoers.d/%s", username)
		rc.Runner.WriteFile(sudoPath, []byte(sudoContent), 0440)
	}

	// SSH key
	if pubkey != "" {
		sshDir := filepath.Join(homeDir, ".ssh")
		rc.Runner.MkdirAll(sshDir, 0700)
		authKeys := filepath.Join(sshDir, "authorized_keys")
		rc.Runner.WriteFile(authKeys, []byte(pubkey+"\n"), 0600)
		// Fix ownership
		u, _ := user.Lookup(username)
		if u != nil {
			rc.Runner.Run(ctx, "chown", "-R", u.Uid+":"+u.Gid, sshDir)
		}
	}

	// Save to metadata
	saveUserMeta(rc, username, homeDir, shell, groups, cfg.SudoNopasswd, pubkey)

	fmt.Printf("User %s created (home: %s, shell: %s, groups: %v)\n", username, homeDir, shell, groups)
	return nil
}

// BackupUsers exports user metadata to JSON.
func BackupUsers(rc *RunContext, outputPath string) error {
	cfg := rc.Config.Users
	homeBase := cfg.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}

	dbPath := filepath.Join(homeBase, ".rootfiles", "users.json")
	data, err := rc.Runner.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("no user database found at %s", dbPath)
	}

	if outputPath == "" {
		hostname, _ := os.Hostname()
		outputPath = fmt.Sprintf("rootfiles-users-%s-%s.json", hostname, time.Now().Format("20060102"))
	}

	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("writing backup: %w", err)
	}

	fmt.Printf("User backup saved to %s\n", outputPath)
	return nil
}

// RestoreUsers restores users from a backup JSON file.
func RestoreUsers(ctx context.Context, rc *RunContext, backupPath string) error {
	cfg := rc.Config.Users
	homeBase := cfg.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}

	// Auto-detect backup path
	if backupPath == "" {
		backupPath = filepath.Join(homeBase, ".rootfiles", "users.json")
	}

	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("reading backup: %w", err)
	}

	var db UsersDB
	if err := json.Unmarshal(data, &db); err != nil {
		return fmt.Errorf("parsing backup: %w", err)
	}

	for _, u := range db.Users {
		// Check if user already exists
		if _, err := user.Lookup(u.Name); err == nil {
			fmt.Printf("  User %s already exists, skipping\n", u.Name)
			continue
		}

		// Check if home dir exists (preserved from previous install)
		homeExists := rc.Runner.FileExists(u.Home)

		args := []string{
			"--home-dir", u.Home,
			"--shell", u.Shell,
			"--uid", strconv.Itoa(u.UID),
		}
		if homeExists {
			args = append(args, "--no-create-home")
		} else {
			args = append(args, "--create-home")
		}
		args = append(args, u.Name)

		if _, err := rc.Runner.Run(ctx, "useradd", args...); err != nil {
			fmt.Printf("  Warning: failed to create user %s: %v\n", u.Name, err)
			continue
		}

		// Restore groups
		if len(u.Groups) > 0 {
			rc.Runner.Run(ctx, "usermod", "-aG", strings.Join(u.Groups, ","), u.Name)
		}

		// Restore sudoers
		if u.SudoNopasswd {
			sudoContent := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL\n", u.Name)
			rc.Runner.WriteFile(fmt.Sprintf("/etc/sudoers.d/%s", u.Name), []byte(sudoContent), 0440)
		}

		// Fix ownership if home preserved
		if homeExists {
			rc.Runner.Run(ctx, "chown", "-R",
				strconv.Itoa(u.UID)+":"+strconv.Itoa(u.GID), u.Home)
		}

		status := "created"
		if homeExists {
			status = "restored (home preserved)"
		}
		fmt.Printf("  User %s %s\n", u.Name, status)
	}

	return nil
}

// RehomeUser moves a user's home from /home/<name> to the custom home base.
func RehomeUser(ctx context.Context, rc *RunContext, username string) error {
	cfg := rc.Config.Users
	homeBase := cfg.HomeBase
	if homeBase == "" {
		return fmt.Errorf("home_base not configured")
	}

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user %s not found: %w", username, err)
	}

	oldHome := u.HomeDir
	newHome := filepath.Join(homeBase, username)

	if oldHome == newHome {
		return fmt.Errorf("user %s already at %s", username, newHome)
	}

	// Copy files
	if _, err := rc.Runner.Run(ctx, "rsync", "-a", oldHome+"/", newHome+"/"); err != nil {
		return fmt.Errorf("copying home: %w", err)
	}

	// Update user home
	if _, err := rc.Runner.Run(ctx, "usermod", "--home", newHome, username); err != nil {
		return fmt.Errorf("updating user home: %w", err)
	}

	// Remove old home and create symlink for compatibility
	rc.Runner.Run(ctx, "rm", "-rf", oldHome)
	rc.Runner.Symlink(newHome, oldHome)

	// Fix ownership
	rc.Runner.Run(ctx, "chown", "-R", u.Uid+":"+u.Gid, newHome)

	fmt.Printf("User %s moved: %s → %s (symlink created)\n", username, oldHome, newHome)
	return nil
}

// ListUsers shows managed users from metadata.
func ListUsers(rc *RunContext) error {
	cfg := rc.Config.Users
	homeBase := cfg.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}

	dbPath := filepath.Join(homeBase, ".rootfiles", "users.json")
	data, err := rc.Runner.ReadFile(dbPath)
	if err != nil {
		fmt.Println("No managed users found.")
		return nil
	}

	var db UsersDB
	if err := json.Unmarshal(data, &db); err != nil {
		return fmt.Errorf("parsing user database: %w", err)
	}

	fmt.Printf("Home base: %s\n", db.HomeBase)
	fmt.Printf("%-15s %-6s %-20s %-15s %s\n", "USER", "UID", "HOME", "SHELL", "GROUPS")
	fmt.Printf("%-15s %-6s %-20s %-15s %s\n", "----", "---", "----", "-----", "------")
	for _, u := range db.Users {
		fmt.Printf("%-15s %-6d %-20s %-15s %s\n",
			u.Name, u.UID, u.Home, u.Shell, strings.Join(u.Groups, ","))
	}
	return nil
}

func saveUserMeta(rc *RunContext, username, home, shell string, groups []string, sudoNopasswd bool, pubkey string) {
	cfg := rc.Config.Users
	homeBase := cfg.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}

	dbPath := filepath.Join(homeBase, ".rootfiles", "users.json")
	var db UsersDB

	if data, err := rc.Runner.ReadFile(dbPath); err == nil {
		json.Unmarshal(data, &db)
	}
	if db.Version == 0 {
		db.Version = 1
		db.HomeBase = homeBase
		db.CreatedBy = "rootfiles-v2"
	}

	// Get UID/GID
	u, _ := user.Lookup(username)
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	meta := UserMeta{
		Name:         username,
		UID:          uid,
		GID:          gid,
		Shell:        shell,
		Groups:       groups,
		SudoNopasswd: sudoNopasswd,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Home:         home,
	}
	if pubkey != "" {
		meta.SSHPubkeys = []string{pubkey}
	}

	// Update or append
	found := false
	for i, existing := range db.Users {
		if existing.Name == username {
			db.Users[i] = meta
			found = true
			break
		}
	}
	if !found {
		db.Users = append(db.Users, meta)
	}

	data, _ := json.MarshalIndent(db, "", "  ")
	rc.Runner.WriteFile(dbPath, data, 0600)
}
