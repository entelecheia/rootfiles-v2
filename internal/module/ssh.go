package module

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/entelecheia/rootfiles-v2/internal/config"
)

type SSHModule struct{}

func NewSSHModule() *SSHModule    { return &SSHModule{} }
func (m *SSHModule) Name() string { return "ssh" }

// sshdDropInPath is overridable in tests.
var sshdDropInPath = "/etc/ssh/sshd_config.d/00-rootfiles.conf"

func (m *SSHModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config.SSH

	desired := m.buildConfig(cfg)
	existing, _ := rc.Runner.ReadFile(sshdDropInPath)

	if strings.TrimSpace(string(existing)) != strings.TrimSpace(desired) {
		desc := "Deploy custom sshd configuration"
		if err := m.lockoutGuard(rc); err != nil {
			desc += fmt.Sprintf(" (blocked: %v)", err)
		}
		changes = append(changes, Change{
			Description: desc,
			Command:     "write " + sshdDropInPath,
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

	existing, readErr := rc.Runner.ReadFile(sshdDropInPath)
	if readErr == nil && strings.TrimSpace(string(existing)) == strings.TrimSpace(content) {
		return &ApplyResult{Changed: false}, nil
	}

	if err := m.lockoutGuard(rc); err != nil {
		if !rc.Force {
			return nil, fmt.Errorf("%w (re-run with --force to override)", err)
		}
		fmt.Printf("  ⚠ ssh: %v — continuing because of --force\n", err)
	}

	var messages []string

	// Admit a new SSH port through an active firewall before sshd moves to it.
	if cfg.Port > 0 {
		if st, ok := queryUFW(ctx, rc); ok && st.Active && !st.Allowed[cfg.Port] {
			if _, err := rc.Runner.Run(ctx, "ufw", "allow", strconv.Itoa(cfg.Port)+"/tcp"); err != nil {
				return nil, fmt.Errorf("allowing ssh port %d in ufw: %w", cfg.Port, err)
			}
			messages = append(messages, fmt.Sprintf("ufw: allowed ssh port %d", cfg.Port))
		}
	}

	if err := rc.Runner.MkdirAll("/etc/ssh/sshd_config.d", 0755); err != nil {
		return nil, fmt.Errorf("creating sshd_config.d: %w", err)
	}
	if err := rc.Runner.WriteFile(sshdDropInPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("writing sshd config: %w", err)
	}

	// Validate the full merged configuration; restore the previous drop-in
	// on failure so a reload/reboot never picks up a broken sshd config.
	if err := m.validate(ctx, rc); err != nil {
		if readErr == nil {
			_ = rc.Runner.WriteFile(sshdDropInPath, existing, 0644)
		} else {
			_ = rc.Runner.Remove(sshdDropInPath)
		}
		return nil, fmt.Errorf("sshd config validation failed, change reverted: %w", err)
	}
	messages = append(messages, "sshd configuration deployed")

	portChanged := portLine(string(existing)) != portLine(content)
	var warnings []string
	if msg, err := m.reload(ctx, rc, portChanged); err != nil {
		warnings = append(warnings, fmt.Sprintf("%v (restart sshd manually)", firstLine(err.Error())))
	} else {
		messages = append(messages, msg)
	}

	return &ApplyResult{Changed: true, Messages: messages, Warnings: warnings}, nil
}

// lockoutGuard refuses a configuration that would leave no way to log in
// over SSH: password auth disabled while no account (root only if root
// login stays permitted) has an authorized key.
func (m *SSHModule) lockoutGuard(rc *RunContext) error {
	cfg := rc.Config.SSH
	if !cfg.DisablePasswordAuth {
		return nil
	}
	if len(keyLoginAccounts(!cfg.DisableRootLogin)) > 0 || len(declaredKeyAccounts(rc)) > 0 {
		return nil
	}
	if cfg.DisableRootLogin {
		return fmt.Errorf("disabling password auth and root login, but no non-root account has an SSH authorized key")
	}
	return fmt.Errorf("disabling password auth, but no account has an SSH authorized key")
}

// declaredKeyAccounts lists users.accounts entries with SSH keys. The
// users module (which runs before ssh) creates them, so they count as
// key-based logins even when not yet present (e.g. in check/dry-run).
func declaredKeyAccounts(rc *RunContext) []string {
	if !rc.Config.IsModuleEnabled("users") {
		return nil
	}
	var names []string
	for _, a := range rc.Config.Users.Accounts {
		if len(a.SSHPubkeys) > 0 {
			names = append(names, a.Name)
		}
	}
	return names
}

// validate runs `sshd -t` when sshd is installed. sshd refuses to test
// without its privilege separation directory, so ensure it exists.
func (m *SSHModule) validate(ctx context.Context, rc *RunContext) error {
	bin := sshdBinary(rc)
	if bin == "" || rc.DryRun {
		return nil
	}
	_ = rc.Runner.MkdirAll("/run/sshd", 0755)
	if _, err := rc.Runner.Run(ctx, bin, "-t"); err != nil {
		return err
	}
	return nil
}

// reload makes sshd pick up the new configuration. On socket-activated
// systems (Ubuntu 22.10+) the listening port belongs to ssh.socket, whose
// generator reads sshd_config: a port change needs daemon-reload plus a
// socket restart, a plain `reload ssh` is not enough.
func (m *SSHModule) reload(ctx context.Context, rc *RunContext, portChanged bool) (string, error) {
	if res, err := rc.Runner.Query(ctx, "systemctl", "is-active", "ssh.socket"); err == nil && strings.TrimSpace(res.Stdout) == "active" {
		msg := "ssh reloaded"
		if portChanged {
			if _, err := rc.Runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
				return "", fmt.Errorf("systemctl daemon-reload: %w", err)
			}
			if _, err := rc.Runner.Run(ctx, "systemctl", "restart", "ssh.socket"); err != nil {
				return "", fmt.Errorf("restarting ssh.socket: %w", err)
			}
			msg = "ssh.socket restarted on new port"
		}
		// The daemon spawned by the socket keeps running; reload it if up.
		if _, err := rc.Runner.Run(ctx, "systemctl", "try-reload-or-restart", "ssh.service"); err != nil {
			return "", fmt.Errorf("reloading ssh.service: %w", err)
		}
		return msg, nil
	}

	var lastErr error
	for _, svc := range []string{"ssh", "sshd"} {
		if _, err := rc.Runner.Run(ctx, "systemctl", "reload", svc); err == nil {
			return svc + " reloaded", nil
		} else {
			lastErr = err
		}
	}
	return "", fmt.Errorf("reloading sshd: %w", lastErr)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// portLine returns the "Port N" directive of a drop-in, or "" if none.
func portLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "Port ") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func (m *SSHModule) buildConfig(cfg config.SSHConfig) string {
	var b strings.Builder
	b.WriteString("# Managed by rootfiles-v2\n")

	if cfg.DisableRootLogin {
		b.WriteString("PermitRootLogin no\n")
	}
	if cfg.DisablePasswordAuth {
		b.WriteString("PasswordAuthentication no\n")
		// PAM keyboard-interactive would otherwise still accept passwords.
		b.WriteString("KbdInteractiveAuthentication no\n")
	}
	if cfg.MaxAuthTries > 0 {
		b.WriteString(fmt.Sprintf("MaxAuthTries %d\n", cfg.MaxAuthTries))
	}
	if cfg.Port > 0 {
		b.WriteString(fmt.Sprintf("Port %d\n", cfg.Port))
	}

	return b.String()
}
