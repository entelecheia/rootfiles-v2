package module

import (
	"context"
	"fmt"
)

type NvidiaModule struct{}

func NewNvidiaModule() *NvidiaModule { return &NvidiaModule{} }
func (m *NvidiaModule) Name() string { return "nvidia" }

func (m *NvidiaModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change

	if !rc.APT.IsInstalled("nvidia-container-toolkit") {
		changes = append(changes, Change{
			Description: "Install NVIDIA Container Toolkit",
			Command:     "apt-get install nvidia-container-toolkit",
		})
	}

	// Check if Docker nvidia runtime is configured
	data, _ := rc.Runner.ReadFile("/etc/docker/daemon.json")
	if len(data) > 0 && !containsBytes(data, "nvidia") {
		changes = append(changes, Change{
			Description: "Configure Docker nvidia runtime",
			Command:     "nvidia-ctk runtime configure --runtime=docker",
		})
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *NvidiaModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	var messages []string
	changed := false

	if !rc.APT.IsInstalled("nvidia-container-toolkit") {
		// Add NVIDIA container toolkit repo
		rc.APT.AddKeyring(ctx, "nvidia-container-toolkit",
			"https://nvidia.github.io/libnvidia-container/gpgkey")

		arch := "amd64"
		if rc.Config.System != nil && rc.Config.System.Arch != "" {
			arch = rc.Config.System.Arch
		}
		repoLine := fmt.Sprintf(
			"deb [signed-by=/etc/apt/keyrings/nvidia-container-toolkit.gpg arch=%s] https://nvidia.github.io/libnvidia-container/stable/deb/$(ARCH) /",
			arch)
		rc.APT.AddSourceList(ctx, "nvidia-container-toolkit", repoLine)
		rc.APT.Update(ctx)

		if err := rc.APT.Install(ctx, []string{"nvidia-container-toolkit"}); err != nil {
			return nil, fmt.Errorf("installing nvidia-container-toolkit: %w", err)
		}

		// Configure Docker runtime
		rc.Runner.Run(ctx, "nvidia-ctk", "runtime", "configure", "--runtime=docker")
		rc.Runner.Run(ctx, "systemctl", "restart", "docker")

		messages = append(messages, "NVIDIA Container Toolkit installed and configured")
		changed = true
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}

func containsBytes(data []byte, s string) bool {
	for i := 0; i <= len(data)-len(s); i++ {
		if string(data[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}
