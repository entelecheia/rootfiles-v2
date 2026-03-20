package module

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
)

// Module is the interface that all rootfiles modules must implement.
type Module interface {
	Name() string
	Check(ctx context.Context, rc *RunContext) (*CheckResult, error)
	Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error)
}

// RunContext carries config and runtime options to all modules.
type RunContext struct {
	Config *config.Config
	Runner *exec.Runner
	APT    *exec.APT
	DryRun bool
	Yes    bool // unattended mode
}

// CheckResult holds the result of a module's Check operation.
type CheckResult struct {
	Satisfied bool
	Changes   []Change
}

// Change describes a single pending change.
type Change struct {
	Description string
	Command     string // shell command for dry-run display
}

// ApplyResult holds the result of a module's Apply operation.
type ApplyResult struct {
	Changed  bool
	Messages []string
}

// defaultOrder defines the static module execution order.
// Dependencies are implicit in the ordering.
var defaultOrder = []string{
	"locale",
	"packages",
	"ssh",
	"users",
	"docker",
	"nvidia",
	"gpu",
	"cloudflared",
	"storage",
	"network",
}

// Registry manages module registration and resolution.
type Registry struct {
	modules map[string]Module
}

// NewRegistry creates a registry with all modules registered.
func NewRegistry() *Registry {
	r := &Registry{modules: make(map[string]Module)}
	r.Register(NewLocaleModule())
	r.Register(NewPackagesModule())
	r.Register(NewSSHModule())
	r.Register(NewUsersModule())
	r.Register(NewDockerModule())
	r.Register(NewNvidiaModule())
	r.Register(NewGPUModule())
	r.Register(NewCloudflaredModule())
	r.Register(NewStorageModule())
	r.Register(NewNetworkModule())
	return r
}

// Register adds or replaces a module.
func (r *Registry) Register(m Module) {
	r.modules[m.Name()] = m
}

// Resolve returns modules in execution order, filtered by config and --module flag.
func (r *Registry) Resolve(cfg *config.Config, filter []string) []Module {
	filterSet := make(map[string]bool, len(filter))
	for _, f := range filter {
		filterSet[f] = true
	}

	var result []Module
	for _, name := range defaultOrder {
		if !cfg.IsModuleEnabled(name) {
			continue
		}
		if len(filterSet) > 0 && !filterSet[name] {
			continue
		}
		if m, ok := r.modules[name]; ok {
			result = append(result, m)
		}
	}
	return result
}

// RunAll executes Check then Apply on each module in order.
// Non-fatal module errors are logged but do not stop execution of remaining modules.
func RunAll(ctx context.Context, modules []Module, rc *RunContext) error {
	var errors []string
	for _, m := range modules {
		check, err := m.Check(ctx, rc)
		if err != nil {
			fmt.Printf("  ⚠ %s: check error: %v\n", m.Name(), err)
			continue
		}
		if check.Satisfied {
			fmt.Printf("  ✓ %s: already satisfied\n", m.Name())
			continue
		}

		// Show pending changes
		for _, c := range check.Changes {
			fmt.Printf("  → %s: %s\n", m.Name(), c.Description)
		}

		if rc.DryRun {
			continue
		}

		result, err := m.Apply(ctx, rc)
		if err != nil {
			fmt.Printf("  ✗ %s: %v\n", m.Name(), err)
			errors = append(errors, fmt.Sprintf("%s: %v", m.Name(), err))
			continue
		}
		if result.Changed {
			for _, msg := range result.Messages {
				fmt.Printf("  ✓ %s: %s\n", m.Name(), msg)
			}
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("%d module(s) failed: %v", len(errors), errors)
	}
	return nil
}

// ensureMetaDir ensures the .rootfiles metadata directory exists under homeBase.
// If a legacy flat file exists at that path (from older rootfiles versions),
// it is migrated into the directory as users.json.
func ensureMetaDir(runner *exec.Runner, homeBase string) error {
	metaDir := filepath.Join(homeBase, ".rootfiles")
	info, err := os.Stat(metaDir)
	if err == nil && !info.IsDir() {
		// Legacy flat file exists — migrate it into the directory.
		tmp := metaDir + ".migrating"
		if err := os.Rename(metaDir, tmp); err != nil {
			return fmt.Errorf("migrating legacy .rootfiles file: %w", err)
		}
		if err := os.MkdirAll(metaDir, 0755); err != nil {
			// Attempt to restore on failure.
			os.Rename(tmp, metaDir)
			return fmt.Errorf("creating .rootfiles dir after migration: %w", err)
		}
		dest := filepath.Join(metaDir, "users.json")
		if err := os.Rename(tmp, dest); err != nil {
			return fmt.Errorf("moving legacy file into .rootfiles dir: %w", err)
		}
		slog.Info("migrated legacy .rootfiles file to directory", "path", dest)
	}
	return runner.MkdirAll(metaDir, 0755)
}

// CheckAll runs Check on each module and returns results.
func CheckAll(ctx context.Context, modules []Module, rc *RunContext) (map[string]*CheckResult, error) {
	results := make(map[string]*CheckResult, len(modules))
	for _, m := range modules {
		check, err := m.Check(ctx, rc)
		if err != nil {
			return nil, fmt.Errorf("module %s check: %w", m.Name(), err)
		}
		results[m.Name()] = check
	}
	return results, nil
}
