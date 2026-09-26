package module

import (
	"context"
	"fmt"
)

type NvidiaModule struct{}

func NewNvidiaModule() *NvidiaModule { return &NvidiaModule{} }
func (m *NvidiaModule) Name() string { return "nvidia" }

// needsRuntime reports whether Docker is present but lacks the nvidia
// runtime. Hosts without Docker have nothing to configure.
func (m *NvidiaModule) needsRuntime(rc *RunContext) (bool, error) {
	if !rc.Runner.CommandExists("docker") {
		return false, nil
	}
	daemon, err := readDaemonJSON(rc)
	if err != nil {
		return false, err
	}
	return !hasNvidiaRuntime(daemon), nil
}

func (m *NvidiaModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change

	if !rc.APT.IsInstalled("nvidia-container-toolkit") {
		changes = append(changes, Change{
			Description: "Install NVIDIA Container Toolkit",
			Command:     "apt-get install nvidia-container-toolkit",
		})
	}

	need, err := m.needsRuntime(rc)
	if err != nil {
		return nil, err
	}
	if need {
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
	var messages, warnings []string
	changed := false

	if !rc.APT.IsInstalled("nvidia-container-toolkit") {
		if err := rc.APT.AddKeyring(ctx, "nvidia-container-toolkit",
			"https://nvidia.github.io/libnvidia-container/gpgkey"); err != nil {
			return nil, fmt.Errorf("adding nvidia apt key: %w", err)
		}

		arch := "amd64"
		if rc.Config.System != nil && rc.Config.System.Arch != "" {
			arch = rc.Config.System.Arch
		}
		repoLine := fmt.Sprintf(
			"deb [signed-by=/etc/apt/keyrings/nvidia-container-toolkit.gpg arch=%s] https://nvidia.github.io/libnvidia-container/stable/deb/$(ARCH) /",
			arch)
		if err := rc.APT.AddSourceList(ctx, "nvidia-container-toolkit", repoLine); err != nil {
			return nil, fmt.Errorf("adding nvidia apt source: %w", err)
		}
		if err := rc.APT.Update(ctx); err != nil {
			return nil, fmt.Errorf("apt update: %w", err)
		}
		if err := rc.APT.Install(ctx, []string{"nvidia-container-toolkit"}); err != nil {
			return nil, fmt.Errorf("installing nvidia-container-toolkit: %w", err)
		}
		messages = append(messages, "NVIDIA Container Toolkit installed")
		changed = true
	}

	// Runtime registration is independent of installation: a preinstalled
	// toolkit (DGX OS) may still be missing from a rewritten daemon.json.
	need, err := m.needsRuntime(rc)
	if err != nil {
		return nil, err
	}
	if need {
		if _, err := rc.Runner.Run(ctx, "nvidia-ctk", "runtime", "configure", "--runtime=docker"); err != nil {
			return nil, fmt.Errorf("configuring docker nvidia runtime: %w", err)
		}
		if _, err := rc.Runner.Run(ctx, "systemctl", "restart", "docker"); err != nil {
			warnings = append(warnings, fmt.Sprintf("restarting docker: %s", firstLine(err.Error())))
		}
		messages = append(messages, "Docker nvidia runtime configured")
		changed = true
	}

	return &ApplyResult{Changed: changed, Messages: messages, Warnings: warnings}, nil
}
