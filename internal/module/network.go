package module

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type NetworkModule struct{}

func NewNetworkModule() *NetworkModule { return &NetworkModule{} }
func (m *NetworkModule) Name() string  { return "network" }

func (m *NetworkModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	cfg := rc.Config.Modules.Network

	if cfg.UFW {
		if !rc.Runner.CommandExists("ufw") {
			changes = append(changes, Change{
				Description: "Install and enable UFW firewall",
				Command:     "apt-get install ufw && ufw enable",
			})
		} else {
			// Check if UFW is active
			res, _ := rc.Runner.Run(context.Background(), "ufw", "status")
			if res != nil && !strings.Contains(res.Stdout, "active") {
				changes = append(changes, Change{
					Description: "Enable UFW firewall",
					Command:     "ufw --force enable",
				})
			}
		}

		// Check allowed ports
		for _, port := range cfg.AllowedPorts {
			res, _ := rc.Runner.Run(context.Background(), "ufw", "status")
			portStr := strconv.Itoa(port)
			if res == nil || !strings.Contains(res.Stdout, portStr) {
				changes = append(changes, Change{
					Description: fmt.Sprintf("Allow port %d", port),
					Command:     fmt.Sprintf("ufw allow %d", port),
				})
			}
		}
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *NetworkModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	cfg := rc.Config.Modules.Network
	var messages []string
	changed := false

	if cfg.UFW {
		// Ensure UFW is installed
		if !rc.Runner.CommandExists("ufw") {
			if err := rc.APT.Install(ctx, []string{"ufw"}); err != nil {
				return nil, fmt.Errorf("installing ufw: %w", err)
			}
		}

		// Allow configured ports
		for _, port := range cfg.AllowedPorts {
			rc.Runner.Run(ctx, "ufw", "allow", strconv.Itoa(port))
		}

		// Enable UFW
		rc.Runner.Run(ctx, "ufw", "--force", "enable")
		messages = append(messages, fmt.Sprintf("UFW enabled, ports allowed: %v", cfg.AllowedPorts))
		changed = true
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}
