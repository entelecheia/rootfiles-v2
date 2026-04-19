package module

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

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
	return m.installBinaryVersion(ctx, rc, "")
}

// installBinaryVersion downloads cloudflared at the given version (e.g.
// "2024.9.1"). Empty version falls back to the "latest" alias URL, which
// GitHub resolves server-side to whatever release cloudflared has tagged
// as latest. Non-empty versions use the explicit release download path
// so operators can pin.
func (m *CloudflaredModule) installBinaryVersion(ctx context.Context, rc *RunContext, version string) error {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("cloudflared has no prebuilt binary for arch %q", arch)
	}

	var url string
	if version == "" {
		url = fmt.Sprintf(
			"https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-%s",
			arch,
		)
	} else {
		url = fmt.Sprintf(
			"https://github.com/cloudflare/cloudflared/releases/download/%s/cloudflared-linux-%s",
			version, arch,
		)
	}

	if _, err := rc.Runner.Run(ctx, "curl", "-fsSL", "-o", cloudflaredBinary, url); err != nil {
		return fmt.Errorf("downloading cloudflared: %w", err)
	}
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

// TunnelUpdate updates the cloudflared binary and restarts the service.
// An empty version selects the upstream "latest" release; a pinned version
// (e.g. "2024.9.1") downloads that specific release.
func TunnelUpdate(ctx context.Context, rc *RunContext, version string) error {
	m := NewCloudflaredModule()

	current := currentCloudflaredVersion(ctx, rc)
	target := version
	if target == "" {
		latest, err := FetchLatestCloudflaredVersion(ctx)
		if err != nil {
			fmt.Printf("Warning: could not resolve latest cloudflared release (%v); using /latest/download/ alias.\n", err)
		} else {
			target = latest
		}
	}

	ui.WriteSection(os.Stdout, "Cloudflared update")
	ui.WriteKV(os.Stdout, "Current", firstNonEmptyCloudflared(current, "(not installed)"))
	ui.WriteKV(os.Stdout, "Target", firstNonEmptyCloudflared(target, "latest"))

	if target != "" && current == target {
		ui.WriteHint(os.Stdout, "already on target version — nothing to do.")
		return nil
	}

	fmt.Println()
	fmt.Printf("Downloading cloudflared %s ...\n", firstNonEmptyCloudflared(target, "latest"))
	if err := m.installBinaryVersion(ctx, rc, target); err != nil {
		return err
	}

	// Restart the service only if it's actually installed, so a pure binary
	// refresh on a host that's not running the tunnel doesn't surface a
	// confusing "Unit cloudflared.service not loaded" message.
	if res, err := rc.Runner.Query(ctx, "systemctl", "is-enabled", cloudflaredService); err == nil && strings.TrimSpace(res.Stdout) != "" {
		if _, err := rc.Runner.Run(ctx, "systemctl", "restart", cloudflaredService); err != nil {
			return fmt.Errorf("restarting cloudflared: %w", err)
		}
		fmt.Println(ui.StyleSuccess.Render(ui.MarkOK + " cloudflared updated and service restarted."))
	} else {
		fmt.Println(ui.StyleSuccess.Render(ui.MarkOK + " cloudflared binary updated (service not installed, skip restart)."))
	}
	return nil
}

// FetchLatestCloudflaredVersion queries GitHub's release API for the current
// upstream tag of cloudflared/cloudflare. Surfaced publicly so the cli can
// call it for `--check` without duplicating the HTTP plumbing.
func FetchLatestCloudflaredVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/cloudflare/cloudflared/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var info struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if info.TagName == "" {
		return "", fmt.Errorf("no tag_name in response")
	}
	return info.TagName, nil
}

// currentCloudflaredVersion parses `cloudflared --version` output. Returns
// the empty string when the binary is missing or the output cannot be parsed.
// Example output: "cloudflared version 2024.9.1 (built 2024-09-13-10:42 UTC)".
func currentCloudflaredVersion(ctx context.Context, rc *RunContext) string {
	if !rc.Runner.FileExists(cloudflaredBinary) {
		return ""
	}
	res, err := rc.Runner.Query(ctx, cloudflaredBinary, "--version")
	if err != nil {
		return ""
	}
	for _, token := range strings.Fields(res.Stdout) {
		// Versions look like "YYYY.M.P" — leading digit followed by a dot.
		if len(token) > 3 && token[0] >= '0' && token[0] <= '9' && strings.Contains(token, ".") {
			return token
		}
	}
	return ""
}

func firstNonEmptyCloudflared(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
