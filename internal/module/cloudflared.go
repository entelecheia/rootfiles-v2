package module

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

type CloudflaredModule struct{}

func NewCloudflaredModule() *CloudflaredModule { return &CloudflaredModule{} }
func (m *CloudflaredModule) Name() string      { return "cloudflared" }

const (
	cloudflaredBinary  = "/usr/local/bin/cloudflared"
	cloudflaredService = "cloudflared"
	vlanNetdevPath     = "/etc/systemd/network/10-cloudflared-vlan.netdev"
	vlanNetworkPath    = "/etc/systemd/network/10-cloudflared-vlan.network"
)

func (m *CloudflaredModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config.Modules.Cloudflared

	// Check binary
	if !rc.Runner.FileExists(cloudflaredBinary) {
		changes = append(changes, Change{
			Description: "Install cloudflared binary",
			Command:     "download cloudflared → " + cloudflaredBinary,
		})
	}

	// Check VLAN if private network enabled
	if cfg.PrivateNetwork.Enabled && cfg.PrivateNetwork.Address != "" {
		iface := cfg.PrivateNetwork.Interface
		if iface == "" {
			iface = "vlan0"
		}
		if !rc.Runner.FileExists(vlanNetdevPath) {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Create VLAN interface %s (%s)", iface, cfg.PrivateNetwork.Address),
				Command:     "configure systemd-networkd dummy interface",
			})
		}
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *CloudflaredModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	var messages []string
	changed := false

	// Install binary
	if !rc.Runner.FileExists(cloudflaredBinary) {
		if err := m.installBinary(ctx, rc); err != nil {
			return nil, err
		}
		messages = append(messages, "cloudflared binary installed")
		changed = true
	}

	// Setup VLAN private network
	cfg := rc.Config.Modules.Cloudflared
	if cfg.PrivateNetwork.Enabled && cfg.PrivateNetwork.Address != "" {
		if err := m.setupVLAN(ctx, rc); err != nil {
			return nil, err
		}
		messages = append(messages, fmt.Sprintf("VLAN %s configured (%s)",
			cfg.PrivateNetwork.Interface, cfg.PrivateNetwork.Address))
		changed = true
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}

func (m *CloudflaredModule) installBinary(ctx context.Context, rc *RunContext) error {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	}

	url := fmt.Sprintf(
		"https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-%s",
		arch,
	)

	// Download
	if _, err := rc.Runner.Run(ctx, "curl", "-fsSL", "-o", cloudflaredBinary, url); err != nil {
		return fmt.Errorf("downloading cloudflared: %w", err)
	}
	// Make executable
	if _, err := rc.Runner.Run(ctx, "chmod", "+x", cloudflaredBinary); err != nil {
		return fmt.Errorf("chmod cloudflared: %w", err)
	}
	return nil
}

func (m *CloudflaredModule) setupVLAN(ctx context.Context, rc *RunContext) error {
	cfg := rc.Config.Modules.Cloudflared.PrivateNetwork
	iface := cfg.Interface
	if iface == "" {
		iface = "vlan0"
	}

	// Create systemd-networkd config directory
	rc.Runner.MkdirAll("/etc/systemd/network", 0755)

	// Write netdev file
	netdev := fmt.Sprintf(`# Managed by rootfiles-v2 — cloudflared private network
[NetDev]
Name=%s
Kind=dummy
`, iface)
	if err := rc.Runner.WriteFile(vlanNetdevPath, []byte(netdev), 0644); err != nil {
		return fmt.Errorf("writing netdev: %w", err)
	}

	// Write network file
	network := fmt.Sprintf(`# Managed by rootfiles-v2 — cloudflared private network
[Match]
Name=%s

[Network]
Address=%s
`, iface, cfg.Address)
	if err := rc.Runner.WriteFile(vlanNetworkPath, []byte(network), 0644); err != nil {
		return fmt.Errorf("writing network: %w", err)
	}

	// Apply immediately
	rc.Runner.Run(ctx, "systemctl", "restart", "systemd-networkd")

	// Verify interface came up
	if _, err := rc.Runner.Run(ctx, "ip", "link", "show", iface); err != nil {
		// Manual fallback: create dummy interface directly
		rc.Runner.Run(ctx, "ip", "link", "add", iface, "type", "dummy")
		rc.Runner.Run(ctx, "ip", "addr", "add", cfg.Address, "dev", iface)
		rc.Runner.Run(ctx, "ip", "link", "set", iface, "up")
	}

	return nil
}

// TunnelSetup installs cloudflared and configures tunnel + VLAN.
// Called from CLI `rootfiles tunnel setup`.
func TunnelSetup(ctx context.Context, rc *RunContext, token, vlanAddr string) error {
	m := NewCloudflaredModule()

	// Install binary if needed
	if !rc.Runner.FileExists(cloudflaredBinary) {
		fmt.Println("Installing cloudflared...")
		if err := m.installBinary(ctx, rc); err != nil {
			return err
		}
	}

	// Install tunnel service
	if token != "" {
		fmt.Println("Setting up tunnel service...")
		if _, err := rc.Runner.Run(ctx, cloudflaredBinary, "service", "install", token); err != nil {
			return fmt.Errorf("installing tunnel service: %w", err)
		}
		rc.Runner.Run(ctx, "systemctl", "enable", "--now", cloudflaredService)
	}

	// Setup VLAN
	if vlanAddr != "" {
		fmt.Println("Configuring VLAN private network...")
		rc.Config.Modules.Cloudflared.PrivateNetwork.Address = vlanAddr
		if rc.Config.Modules.Cloudflared.PrivateNetwork.Interface == "" {
			rc.Config.Modules.Cloudflared.PrivateNetwork.Interface = "vlan0"
		}
		if err := m.setupVLAN(ctx, rc); err != nil {
			return err
		}
	}

	fmt.Println("Tunnel setup complete.")
	return nil
}

// TunnelStatus shows cloudflared and VLAN status.
func TunnelStatus(ctx context.Context, rc *RunContext) error {
	ui.WriteSection(os.Stdout, "Tunnel")

	// Binary version
	if rc.Runner.FileExists(cloudflaredBinary) {
		res, _ := rc.Runner.Run(ctx, cloudflaredBinary, "--version")
		ui.WriteKV(os.Stdout, "Binary", strings.TrimSpace(res.Stdout))
	} else {
		ui.WriteKV(os.Stdout, "Binary", ui.StyleHint.Render("not installed"))
	}

	// Service status
	res, err := rc.Runner.Run(ctx, "systemctl", "is-active", cloudflaredService)
	if err != nil {
		ui.WriteKV(os.Stdout, "Service", ui.StyleHint.Render("inactive"))
	} else {
		state := strings.TrimSpace(res.Stdout)
		if state == "active" {
			ui.WriteKV(os.Stdout, "Service", ui.StyleSuccess.Render("active"))
		} else {
			ui.WriteKV(os.Stdout, "Service", ui.StyleHint.Render(state))
		}
	}

	// VLAN interface
	cfg := rc.Config.Modules.Cloudflared.PrivateNetwork
	iface := cfg.Interface
	if iface == "" {
		iface = "vlan0"
	}
	res, err = rc.Runner.Run(ctx, "ip", "addr", "show", iface)
	if err != nil {
		ui.WriteKV(os.Stdout, fmt.Sprintf("VLAN (%s)", iface), ui.StyleHint.Render("not configured"))
	} else {
		// Extract address line
		found := false
		for _, line := range strings.Split(res.Stdout, "\n") {
			if strings.Contains(line, "inet ") {
				ui.WriteKV(os.Stdout, fmt.Sprintf("VLAN (%s)", iface), strings.TrimSpace(line))
				found = true
				break
			}
		}
		if !found {
			ui.WriteKV(os.Stdout, fmt.Sprintf("VLAN (%s)", iface), ui.StyleHint.Render("up (no inet)"))
		}
	}

	return nil
}

// TunnelUpdate updates cloudflared binary.
func TunnelUpdate(ctx context.Context, rc *RunContext) error {
	m := NewCloudflaredModule()
	fmt.Println("Updating cloudflared...")
	if err := m.installBinary(ctx, rc); err != nil {
		return err
	}
	// Restart service if running
	rc.Runner.Run(ctx, "systemctl", "restart", cloudflaredService)
	fmt.Println("cloudflared updated and service restarted.")
	return nil
}

// TunnelUninstall removes tunnel service, VLAN, and binary.
func TunnelUninstall(ctx context.Context, rc *RunContext) error {
	// Stop and uninstall service
	rc.Runner.Run(ctx, "systemctl", "stop", cloudflaredService)
	rc.Runner.Run(ctx, cloudflaredBinary, "service", "uninstall")

	// Remove VLAN
	cfg := rc.Config.Modules.Cloudflared.PrivateNetwork
	iface := cfg.Interface
	if iface == "" {
		iface = "vlan0"
	}
	rc.Runner.Run(ctx, "ip", "link", "delete", iface)
	rc.Runner.Run(ctx, "rm", "-f", vlanNetdevPath, vlanNetworkPath)

	// Remove binary
	rc.Runner.Run(ctx, "rm", "-f", cloudflaredBinary)

	fmt.Println("Tunnel uninstalled (service, VLAN, binary removed).")
	return nil
}
