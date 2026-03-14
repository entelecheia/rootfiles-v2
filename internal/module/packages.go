package module

import (
	"context"
	"fmt"
)

type PackagesModule struct{}

func NewPackagesModule() *PackagesModule { return &PackagesModule{} }
func (m *PackagesModule) Name() string   { return "packages" }

func (m *PackagesModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	allPkgs := rc.Config.AllPackages()

	var missing []string
	for _, pkg := range allPkgs {
		if !rc.APT.IsInstalled(pkg) {
			missing = append(missing, pkg)
		}
	}

	if len(missing) > 0 {
		changes = append(changes, Change{
			Description: fmt.Sprintf("Install %d packages: %v", len(missing), missing),
			Command:     fmt.Sprintf("apt-get install -y %v", missing),
		})
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *PackagesModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	allPkgs := rc.Config.AllPackages()

	var missing []string
	for _, pkg := range allPkgs {
		if !rc.APT.IsInstalled(pkg) {
			missing = append(missing, pkg)
		}
	}

	if len(missing) == 0 {
		return &ApplyResult{Changed: false, Messages: []string{"all packages already installed"}}, nil
	}

	// Update package index
	if err := rc.APT.Update(ctx); err != nil {
		return nil, fmt.Errorf("apt update: %w", err)
	}

	// Install missing packages
	if err := rc.APT.Install(ctx, missing); err != nil {
		return nil, fmt.Errorf("apt install: %w", err)
	}

	return &ApplyResult{
		Changed:  true,
		Messages: []string{fmt.Sprintf("installed %d packages", len(missing))},
	}, nil
}
