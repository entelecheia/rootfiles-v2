package module

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Remote-access helpers shared by the ssh and network modules. Both modules
// can cut off the operator's SSH session, so they consult the same view of
// "which port is sshd on" and "who can still log in with a key".

// passwdPath is overridable in tests.
var passwdPath = "/etc/passwd"

// ufwState is the parsed output of `ufw status`.
type ufwState struct {
	Active  bool
	Allowed map[int]bool
}

// parseUFWStatus parses `ufw status` output. Only exact port matches on
// ALLOW rules count (so "2222" does not satisfy a request for "22").
func parseUFWStatus(out string) ufwState {
	st := ufwState{Allowed: map[int]bool{}}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Status:") {
			st.Active = strings.TrimSpace(strings.TrimPrefix(line, "Status:")) == "active"
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		allow := false
		for _, f := range fields[1:] {
			if f == "ALLOW" {
				allow = true
				break
			}
		}
		if !allow {
			continue
		}
		spec := fields[0]
		if i := strings.Index(spec, "/"); i >= 0 {
			if proto := spec[i+1:]; proto != "tcp" {
				continue // udp-only rules do not admit SSH
			}
			spec = spec[:i]
		}
		for _, ps := range strings.Split(spec, ",") {
			if p, err := strconv.Atoi(ps); err == nil {
				st.Allowed[p] = true
			}
		}
	}
	return st
}

// queryUFW returns the current UFW state, or ok=false when ufw is absent.
func queryUFW(ctx context.Context, rc *RunContext) (ufwState, bool) {
	if !rc.Runner.CommandExists("ufw") {
		return ufwState{Allowed: map[int]bool{}}, false
	}
	res, err := rc.Runner.Query(ctx, "ufw", "status")
	if err != nil || res == nil {
		return ufwState{Allowed: map[int]bool{}}, false
	}
	return parseUFWStatus(res.Stdout), true
}

// sshPorts returns the ports sshd listens on (or will, once the ssh module
// has run): the configured port when set, otherwise the effective ports
// reported by `sshd -T`, otherwise every Port directive in the config
// files, falling back to 22. Erring towards more ports is deliberate: the
// result decides what the firewall must keep open.
func sshPorts(ctx context.Context, rc *RunContext) []int {
	if rc.Config.SSH.Port > 0 {
		return []int{rc.Config.SSH.Port}
	}
	if bin := sshdBinary(rc); bin != "" {
		// sshd -T refuses to run without its privilege separation dir,
		// which only exists while sshd runs. Creating it is harmless.
		_ = os.MkdirAll("/run/sshd", 0755)
		if res, err := rc.Runner.Query(ctx, bin, "-T"); err == nil {
			if ports := parsePortDirectives(res.Stdout); len(ports) > 0 {
				return ports
			}
		}
	}
	var all string
	for _, p := range append([]string{sshdConfigPath}, globFiles(sshdConfigDir, "*.conf")...) {
		if data, err := rc.Runner.ReadFile(p); err == nil {
			all += string(data) + "\n"
		}
	}
	if ports := parsePortDirectives(all); len(ports) > 0 {
		return ports
	}
	return []int{22}
}

// Overridable in tests.
var (
	sshdConfigPath = "/etc/ssh/sshd_config"
	sshdConfigDir  = "/etc/ssh/sshd_config.d"
)

// parsePortDirectives collects unique "Port N" values (case-insensitive).
func parsePortDirectives(text string) []int {
	seen := map[int]bool{}
	var ports []int
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.EqualFold(f[0], "port") {
			if p, err := strconv.Atoi(f[1]); err == nil && p > 0 && !seen[p] {
				seen[p] = true
				ports = append(ports, p)
			}
		}
	}
	return ports
}

func globFiles(dir, pattern string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, pattern))
	return m
}

// sshdBinary locates sshd, which lives in /usr/sbin and may be off PATH.
func sshdBinary(rc *RunContext) string {
	if rc.Runner.CommandExists("sshd") {
		return "sshd"
	}
	if rc.Runner.FileExists("/usr/sbin/sshd") {
		return "/usr/sbin/sshd"
	}
	return ""
}

// keyLoginAccounts lists accounts that can log in over SSH with a public
// key: a real login shell and a non-empty authorized_keys. root is included
// only when includeRoot is set (i.e. root login stays permitted).
func keyLoginAccounts(includeRoot bool) []string {
	f, err := os.Open(passwdPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var names []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) < 7 {
			continue
		}
		name, uidStr, home, shell := parts[0], parts[2], parts[5], parts[6]
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			continue
		}
		if uid == 0 {
			if !includeRoot {
				continue
			}
		} else if uid < 1000 || uid == 65534 {
			continue
		}
		if strings.HasSuffix(shell, "nologin") || strings.HasSuffix(shell, "/false") {
			continue
		}
		if hasAuthorizedKey(filepath.Join(home, ".ssh", "authorized_keys")) {
			names = append(names, name)
		}
	}
	return names
}

func hasAuthorizedKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return true
		}
	}
	return false
}
