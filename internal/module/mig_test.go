package module

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMIGStatus(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *mig.mode.current*) printf '0, NVIDIA A100-SXM4-80GB, Enabled, Enabled\n1, NVIDIA A100-SXM4-80GB, Disabled, Enabled\n' ;;
  -L) printf 'GPU 0: NVIDIA A100-SXM4-80GB (UUID: GPU-aaa)\n  MIG 3g.40gb     Device  0: (UUID: MIG-111)\n  MIG 3g.40gb     Device  1: (UUID: MIG-222)\nGPU 1: NVIDIA A100-SXM4-80GB (UUID: GPU-bbb)\n' ;;
esac
`
	os.WriteFile(filepath.Join(dir, "nvidia-smi"), []byte(script), 0755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	gpus, err := MIGStatus(context.Background(), newDryRunRC(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 2 || gpus[0].Current != "Enabled" || len(gpus[0].Instances) != 2 {
		t.Fatalf("unexpected: %+v", gpus)
	}
	if gpus[1].Current != "Disabled" || gpus[1].Pending != "Enabled" || len(gpus[1].Instances) != 0 {
		t.Errorf("GPU 1 should show a pending enable and no instances: %+v", gpus[1])
	}
}
