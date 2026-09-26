package module

import (
	"strings"
	"testing"
)

func TestUnattendedPolicyExcludesNvidiaAndNeverReboots(t *testing.T) {
	for _, want := range []string{`"nvidia-";`, `"libnvidia-";`, `"cuda-";`, `Automatic-Reboot "false"`, `-security";`} {
		if !strings.Contains(unattendedPolicyContent, want) {
			t.Errorf("policy missing %q", want)
		}
	}
	if strings.Contains(unattendedPolicyContent, `${distro_codename}-updates`) {
		t.Error("policy must be security-only")
	}
}

func TestFail2banJailUsesSSHPorts(t *testing.T) {
	j := fail2banJail([]int{22, 2222})
	if !strings.Contains(j, "port     = 22,2222") || !strings.Contains(j, "backend  = systemd") {
		t.Errorf("unexpected jail:\n%s", j)
	}
}

func TestSSHBuildConfigHardening(t *testing.T) {
	m := NewSSHModule()
	got := m.buildConfig(sshCfg(true, 3))
	for _, want := range []string{"PasswordAuthentication no", "KbdInteractiveAuthentication no", "MaxAuthTries 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
