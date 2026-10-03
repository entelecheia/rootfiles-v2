package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

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
		// Only an absolute recorded path names the applied file; a relative
		// one would resolve against the current directory.
		if filepath.IsAbs(last.ConfigPath) {
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

// Where the ownership walk of a reused config starts and the uid that must
// own it; tests stub both.
var (
	configTrustRoot        = "/"
	configOwnerUID  uint32 = 0
)

// resolveRunTarget is resolveTarget for the user, gpu, tunnel, schedule and
// backup subcommands. A config they were not given but would reuse from the
// last apply decides what root does (sudoers, groups, tunnel settings), so
// it is reused only when root alone controls it and this process can read
// it; otherwise they warn and fall back to the recorded profile or the one
// detection suggests.
func resolveRunTarget(cmd *cobra.Command, sys *config.SystemInfo, logger *slog.Logger) (profile, configPath string) {
	profile, configPath = resolveTarget(cmd, sys)
	if flag, _ := cmd.Flags().GetString("config"); configPath == "" || configPath == flag {
		return profile, configPath
	}
	if err := reusableConfig(configPath); err != nil {
		logger.Warn("not reusing the last applied config; pass --config or --profile to choose", "config", configPath, "err", err)
		if last, err := state.Last(); err == nil && last != nil && last.Profile != "" {
			return last.Profile, ""
		}
		return sys.SuggestProfile(), ""
	}
	return profile, configPath
}

// reusableConfig reports why path may not be reused: a directory from the
// root down to it, or the file itself, is not root-owned, is writable by
// group or others, or is a symlink, or this process cannot read the file.
func reusableConfig(path string) error {
	if err := config.RootOnlyBase(configTrustRoot, filepath.Dir(path), configOwnerUID); err != nil {
		return err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !fi.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", path)
	case !ok || st.Uid != configOwnerUID:
		return fmt.Errorf("%s is not owned by root", path)
	case fi.Mode().Perm()&0o022 != 0:
		return fmt.Errorf("%s is writable by group or others", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return f.Close()
}
