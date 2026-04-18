package module

import (
	"context"
	"fmt"
	"strings"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

type SSHModule struct{}

func NewSSHModule() *SSHModule    { return &SSHModule{} }
func (m *SSHModule) Name() string { return "ssh" }

func (m *SSHModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config.SSH

	desired := m.buildConfig(cfg)
	existing, _ := rc.Runner.ReadFile("/etc/ssh/sshd_config.d/00-rootfiles.conf")

	if strings.TrimSpace(string(existing)) != strings.TrimSpace(desired) {
		changes = append(changes, Change{
			Description: "Deploy custom sshd configuration",
			Command:     "write /etc/ssh/sshd_config.d/00-rootfiles.conf",
		})
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *SSHModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	cfg := rc.Config.SSH
	content := m.buildConfig(cfg)

	// Ensure config.d directory exists
	rc.Runner.MkdirAll("/etc/ssh/sshd_config.d", 0755)

	if err := rc.Runner.WriteFile("/etc/ssh/sshd_config.d/00-rootfiles.conf", []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("writing sshd config: %w", err)
	}

	// Reload sshd
	if _, err := rc.Runner.Run(ctx, "systemctl", "reload", "sshd"); err != nil {
		// Try ssh service name (some distros)
		rc.Runner.Run(ctx, "systemctl", "reload", "ssh")
	}

	return &ApplyResult{
		Changed:  true,
		Messages: []string{"sshd configuration deployed and reloaded"},
	}, nil
}

func (m *SSHModule) buildConfig(cfg config.SSHConfig) string {
	var b strings.Builder
	b.WriteString("# Managed by rootfiles-v2\n")

	if cfg.DisableRootLogin {
		b.WriteString("PermitRootLogin no\n")
	}
	if cfg.DisablePasswordAuth {
		b.WriteString("PasswordAuthentication no\n")
	}
	if cfg.Port > 0 {
		b.WriteString(fmt.Sprintf("Port %d\n", cfg.Port))
	}

	return b.String()
}
