package module

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type StorageModule struct{}

func NewStorageModule() *StorageModule { return &StorageModule{} }
func (m *StorageModule) Name() string  { return "storage" }

func (m *StorageModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config.Modules.Storage

	// Check data directory
	if cfg.DataDir != "" && !rc.Runner.FileExists(cfg.DataDir) {
		changes = append(changes, Change{
			Description: fmt.Sprintf("Create data directory %s", cfg.DataDir),
			Command:     fmt.Sprintf("mkdir -p %s", cfg.DataDir),
		})
	}

	// Check home base
	homeBase := rc.Config.Users.HomeBase
	if homeBase != "" && homeBase != "/home" && !rc.Runner.FileExists(homeBase) {
		changes = append(changes, Change{
			Description: fmt.Sprintf("Create home base directory %s", homeBase),
			Command:     fmt.Sprintf("mkdir -p %s", homeBase),
		})
	}

	// Check metadata directory
	if homeBase != "" && homeBase != "/home" {
		metaDir := filepath.Join(homeBase, ".rootfiles")
		if !rc.Runner.FileExists(metaDir) {
			changes = append(changes, Change{
				Description: "Create rootfiles metadata directory",
				Command:     fmt.Sprintf("mkdir -p %s", metaDir),
			})
		}
	}

	// Check symlinks
	for link, target := range cfg.Symlinks {
		if !isSymlinkTo(link, target) {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Create symlink %s → %s", link, target),
				Command:     fmt.Sprintf("ln -sfn %s %s", target, link),
			})
		}
	}

	// Check Docker storage directory
	dockerDir := rc.Config.Modules.Docker.StorageDir
	if dockerDir != "" && !rc.Runner.FileExists(dockerDir) {
		changes = append(changes, Change{
			Description: fmt.Sprintf("Create Docker storage directory %s", dockerDir),
			Command:     fmt.Sprintf("mkdir -p %s", dockerDir),
		})
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *StorageModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	cfg := rc.Config.Modules.Storage
	var messages []string
	changed := false

	// Create data directory
	if cfg.DataDir != "" {
		if err := rc.Runner.MkdirAll(cfg.DataDir, 0755); err != nil {
			return nil, fmt.Errorf("creating data dir: %w", err)
		}
		messages = append(messages, fmt.Sprintf("data directory %s ready", cfg.DataDir))
		changed = true
	}

	// Create home base
	homeBase := rc.Config.Users.HomeBase
	if homeBase != "" && homeBase != "/home" {
		rc.Runner.MkdirAll(homeBase, 0755)
		rc.Runner.MkdirAll(filepath.Join(homeBase, ".rootfiles"), 0755)
		messages = append(messages, fmt.Sprintf("home base %s ready", homeBase))
		changed = true
	}

	// Create symlinks
	for link, target := range cfg.Symlinks {
		if isSymlinkTo(link, target) {
			continue
		}
		// Ensure target exists
		rc.Runner.MkdirAll(target, 0755)
		// Remove existing if it's not a symlink
		if rc.Runner.FileExists(link) {
			rc.Runner.Run(ctx, "rm", "-rf", link)
		}
		if err := rc.Runner.Symlink(target, link); err != nil {
			return nil, fmt.Errorf("creating symlink %s → %s: %w", link, target, err)
		}
		messages = append(messages, fmt.Sprintf("symlink %s → %s", link, target))
		changed = true
	}

	// Docker storage directory
	dockerDir := rc.Config.Modules.Docker.StorageDir
	if dockerDir != "" {
		rc.Runner.MkdirAll(dockerDir, 0710)
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}

func isSymlinkTo(link, target string) bool {
	fi, err := os.Lstat(link)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return false
	}
	actual, err := os.Readlink(link)
	if err != nil {
		return false
	}
	return actual == target
}
