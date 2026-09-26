package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// dockerDaemonJSONPath is overridable in tests.
var dockerDaemonJSONPath = "/etc/docker/daemon.json"

// readDaemonJSON loads /etc/docker/daemon.json as a generic map so that
// keys owned by other tools (nvidia-ctk runtimes, DGX OS defaults,
// operator edits) survive a rewrite. A missing file yields an empty map.
func readDaemonJSON(rc *RunContext) (map[string]any, error) {
	data, err := rc.Runner.ReadFile(dockerDaemonJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dockerDaemonJSONPath, err)
	}
	m := map[string]any{}
	if len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", dockerDaemonJSONPath, err)
	}
	return m, nil
}

func writeDaemonJSON(rc *RunContext, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := rc.Runner.MkdirAll(filepath.Dir(dockerDaemonJSONPath), 0755); err != nil {
		return err
	}
	return rc.Runner.WriteFile(dockerDaemonJSONPath, append(data, '\n'), 0644)
}

// hasNvidiaRuntime reports whether daemon.json registers the nvidia runtime.
func hasNvidiaRuntime(m map[string]any) bool {
	runtimes, ok := m["runtimes"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = runtimes["nvidia"]
	return ok
}
