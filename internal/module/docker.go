package module

import (
	"context"
	"fmt"
	"strings"
)

type DockerModule struct{}

func NewDockerModule() *DockerModule { return &DockerModule{} }
func (m *DockerModule) Name() string { return "docker" }

func (m *DockerModule) Check(ctx context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change

	if !rc.Runner.CommandExists("docker") {
		changes = append(changes, Change{
			Description: "Install Docker CE",
			Command:     "apt-get install docker-ce docker-ce-cli containerd.io",
		})
	}

	// Check daemon.json
	cfg := rc.Config.Modules.Docker
	if cfg.StorageDir != "" {
		data, _ := rc.Runner.ReadFile("/etc/docker/daemon.json")
		if !strings.Contains(string(data), cfg.StorageDir) {
			changes = append(changes, Change{
				Description: fmt.Sprintf("Configure Docker storage at %s", cfg.StorageDir),
				Command:     "write /etc/docker/daemon.json",
			})
		}
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *DockerModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	var messages []string
	changed := false
	cfg := rc.Config.Modules.Docker

	if !rc.Runner.CommandExists("docker") {
		// Add Docker repo
		codename := "jammy"
		if rc.Config.System != nil && rc.Config.System.Codename != "" {
			codename = rc.Config.System.Codename
		}
		arch := "amd64"
		if rc.Config.System != nil && rc.Config.System.Arch != "" {
			arch = rc.Config.System.Arch
		}

		rc.APT.AddKeyring(ctx, "docker", "https://download.docker.com/linux/ubuntu/gpg")
		repoLine := fmt.Sprintf("deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu %s stable",
			arch, codename)
		rc.APT.AddSourceList(ctx, "docker", repoLine)
		rc.APT.Update(ctx)

		pkgs := []string{"docker-ce", "docker-ce-cli", "containerd.io",
			"docker-buildx-plugin", "docker-compose-plugin"}
		if err := rc.APT.Install(ctx, pkgs); err != nil {
			return nil, fmt.Errorf("installing docker: %w", err)
		}
		messages = append(messages, "Docker CE installed")
		changed = true
	}

	// Configure daemon.json
	if cfg.StorageDir != "" {
		daemonJSON := fmt.Sprintf(`{
  "data-root": "%s",
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
`, cfg.StorageDir)

		rc.Runner.MkdirAll("/etc/docker", 0755)
		rc.Runner.MkdirAll(cfg.StorageDir, 0710)

		data, _ := rc.Runner.ReadFile("/etc/docker/daemon.json")
		if !strings.Contains(string(data), cfg.StorageDir) {
			rc.Runner.WriteFile("/etc/docker/daemon.json", []byte(daemonJSON), 0644)
			rc.Runner.Run(ctx, "systemctl", "restart", "docker")
			messages = append(messages, fmt.Sprintf("Docker data-root set to %s", cfg.StorageDir))
			changed = true
		}
	}

	// Enable and start
	rc.Runner.Run(ctx, "systemctl", "enable", "--now", "docker")

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}
