package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/state"
)

// ExitError carries a process exit code. An empty Msg exits silently
// (e.g. `check` reporting drift with code 2).
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }
func (e *ExitError) ExitCode() int { return e.Code }

// Exit codes shared by read-only commands, suitable for monitoring:
// 0 = everything satisfied, 2 = pending changes / problems found.
const exitDrift = 2

const annotPrometheus = "rootfiles/prometheus"

func addOutputFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "text", "Output format: text or json")
}

// addOutputFlagWithMetrics also allows Prometheus text exposition, for the
// node_exporter textfile collector.
func addOutputFlagWithMetrics(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "text", "Output format: text, json or prometheus")
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotPrometheus] = "true"
}

func outputFormat(cmd *cobra.Command) (string, error) {
	f, _ := cmd.Flags().GetString("output")
	switch {
	case f == "" || f == "text":
		return "text", nil
	case f == "json":
		return "json", nil
	case f == "prometheus" && cmd.Annotations[annotPrometheus] == "true":
		return "prometheus", nil
	default:
		return "", fmt.Errorf("unknown --output %q", f)
	}
}

func boolMetric(b bool) int {
	if b {
		return 1
	}
	return 0
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// resolveTarget picks the profile/config to evaluate: explicit flags or
// env first, then what was last applied on this host, then the profile
// suggested by system detection.
func resolveTarget(cmd *cobra.Command, sys *config.SystemInfo) (profile, configPath string) {
	profile, _ = cmd.Flags().GetString("profile")
	configPath, _ = cmd.Flags().GetString("config")
	if profile == "" {
		profile = os.Getenv("ROOTFILES_PROFILE")
	}
	if profile != "" || configPath != "" {
		return profile, configPath
	}
	if last, err := state.Last(); err == nil && last != nil {
		if last.ConfigPath != "" && last.ConfigPath != "-" {
			if _, err := os.Stat(last.ConfigPath); err == nil {
				return "", last.ConfigPath
			}
		}
		if last.Profile != "" {
			return last.Profile, ""
		}
	}
	return sys.SuggestProfile(), ""
}
