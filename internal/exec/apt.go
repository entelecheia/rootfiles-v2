package exec

import (
	"context"
	"strings"
)

// APT wraps apt-get operations.
type APT struct {
	Runner *Runner
}

// NewAPT creates a new APT wrapper.
func NewAPT(runner *Runner) *APT {
	return &APT{Runner: runner}
}

// Update runs apt-get update.
func (a *APT) Update(ctx context.Context) error {
	_, err := a.Runner.Run(ctx, "apt-get", "update", "-qq")
	return err
}

// Install installs packages via apt-get.
func (a *APT) Install(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"install", "-y", "-qq"}, packages...)
	_, err := a.Runner.Run(ctx, "apt-get", args...)
	return err
}

// IsInstalled checks if a package is installed.
func (a *APT) IsInstalled(pkg string) bool {
	res, err := a.Runner.Run(context.Background(), "dpkg", "-s", pkg)
	if err != nil {
		return false
	}
	return res.ExitCode == 0 && strings.Contains(res.Stdout, "Status: install ok installed")
}

// AddKeyring downloads a GPG key and saves it to /etc/apt/keyrings/.
func (a *APT) AddKeyring(ctx context.Context, name, keyURL string) error {
	keyPath := "/etc/apt/keyrings/" + name + ".gpg"
	if a.Runner.FileExists(keyPath) {
		return nil
	}
	a.Runner.MkdirAll("/etc/apt/keyrings", 0755)
	_, err := a.Runner.RunShell(ctx, "curl -fsSL "+keyURL+" | gpg --dearmor -o "+keyPath)
	return err
}

// AddSourceList writes an APT source list file.
func (a *APT) AddSourceList(ctx context.Context, name, content string) error {
	path := "/etc/apt/sources.list.d/" + name + ".list"
	return a.Runner.WriteFile(path, []byte(content+"\n"), 0644)
}
