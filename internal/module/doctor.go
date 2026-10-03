package module

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// Doctor runs read-only health checks that matter for operating a server
// remotely: can I still get in, is anything about to break, does anything
// need a reboot. Unlike Check, it inspects live state rather than config
// drift.

// Finding is one doctor result.
type Finding struct {
	Check  string `json:"check"`
	Level  string `json:"level"` // ok | warn | fail | skip
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelFail = "fail"
	LevelSkip = "skip"
)

// Overridable in tests.
var (
	rebootRequiredPath     = "/var/run/reboot-required"
	rebootRequiredPkgsPath = "/var/run/reboot-required.pkgs"
)

// Doctor runs all checks.
func Doctor(ctx context.Context, rc *RunContext) []Finding {
	var out []Finding
	out = append(out, doctorSSH(ctx, rc)...)
	out = append(out, doctorFirewall(ctx, rc))
	out = append(out, doctorReboot())
	out = append(out, doctorDisks(rc)...)
	out = append(out, doctorTimeSync(ctx, rc))
	out = append(out, doctorGPU(ctx, rc)...)
	out = append(out, doctorFailedUnits(ctx, rc))
	return out
}

func doctorSSH(ctx context.Context, rc *RunContext) []Finding {
	bin := sshdBinary(rc)
	if bin == "" {
		return []Finding{{Check: "ssh", Level: LevelWarn, Detail: "sshd is not installed", Hint: "apt-get install openssh-server"}}
	}
	// sshd -T refuses to run without its privilege separation dir, which
	// only exists while sshd is running. Creating it is harmless.
	_ = os.MkdirAll("/run/sshd", 0755)
	res, err := rc.Runner.Query(ctx, bin, "-T")
	if err != nil {
		return []Finding{{Check: "ssh config", Level: LevelFail, Detail: "sshd -T failed: " + firstLine(err.Error()),
			Hint: "fix sshd_config before the next sshd restart"}}
	}
	eff := map[string]string{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			if _, seen := eff[k]; !seen {
				eff[k] = v
			}
		}
	}
	rootAllowed := eff["permitrootlogin"] != "no"
	keyed := keyLoginAccounts(rootAllowed)

	f := Finding{Check: "ssh access", Level: LevelOK}
	switch {
	case eff["passwordauthentication"] == "no" && len(keyed) == 0:
		f.Level, f.Detail = LevelFail, "password auth is disabled and no account has an authorized key"
		f.Hint = "add a key: rootfiles apply --user <name> --ssh-pubkey '<key>'"
	case len(keyed) == 0:
		f.Level, f.Detail = LevelWarn, "no account has an SSH key; logins rely on passwords"
		f.Hint = "add a key before disabling password auth"
	default:
		f.Detail = fmt.Sprintf("key login for %s; password auth %s; root login %s",
			strings.Join(keyed, ", "), eff["passwordauthentication"], eff["permitrootlogin"])
	}
	out := []Finding{f}
	pwOnly, known := passwordOnlyAccounts()
	if len(pwOnly) == 0 {
		return out
	}
	// Password auth may differ per user (ssh.password_auth_users renders a
	// Match block), so ask sshd for each account when it is off globally.
	var viaPassword, noLogin []string
	for _, name := range pwOnly {
		on := eff["passwordauthentication"] != "no"
		if !on {
			on = userPasswordAuth(ctx, rc, bin, name)
		}
		if on {
			viaPassword = append(viaPassword, name)
		} else {
			noLogin = append(noLogin, name)
		}
	}
	if len(viaPassword) > 0 {
		detail := "password-only accounts (no authorized key): " + strings.Join(viaPassword, ", ")
		if !known {
			detail = "accounts without an authorized key (password state unknown, run as root): " + strings.Join(viaPassword, ", ")
		}
		out = append(out, Finding{Check: "ssh password-only", Level: LevelWarn, Detail: detail,
			Hint: "add their keys, then drop them from ssh.password_auth_users / disable password auth"})
	}
	if len(noLogin) > 0 && known {
		out = append(out, Finding{Check: "ssh no-login", Level: LevelWarn,
			Detail: "accounts with a password but no authorized key cannot log in over SSH: " + strings.Join(noLogin, ", "),
			Hint:   "add their keys or list them in ssh.password_auth_users"})
	}
	return out
}

// userPasswordAuth reports whether sshd's effective config allows password
// auth for one user (`sshd -T -C`), so Match blocks are honoured.
func userPasswordAuth(ctx context.Context, rc *RunContext, bin, user string) bool {
	res, err := rc.Runner.Query(ctx, bin, "-T", "-C", "user="+user+",host=localhost,addr=127.0.0.1")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), " "); ok && k == "passwordauthentication" {
			return v == "yes"
		}
	}
	return false
}

func doctorFirewall(ctx context.Context, rc *RunContext) Finding {
	st, ok := queryUFW(ctx, rc)
	if !ok {
		return Finding{Check: "firewall", Level: LevelSkip, Detail: "ufw not installed"}
	}
	if !st.Active {
		return Finding{Check: "firewall", Level: LevelWarn, Detail: "ufw is inactive", Hint: "enable via modules.network.ufw"}
	}
	for _, p := range sshPorts(ctx, rc) {
		if !st.Allowed[p] {
			return Finding{Check: "firewall", Level: LevelFail,
				Detail: fmt.Sprintf("ufw is active but SSH port %d is not allowed", p),
				Hint:   fmt.Sprintf("ufw allow %d/tcp", p)}
		}
	}
	return Finding{Check: "firewall", Level: LevelOK, Detail: "ufw active, SSH port allowed"}
}

func doctorReboot() Finding {
	if _, err := os.Stat(rebootRequiredPath); err != nil {
		return Finding{Check: "reboot", Level: LevelOK, Detail: "no reboot required"}
	}
	detail := "reboot required"
	if data, err := os.ReadFile(rebootRequiredPkgsPath); err == nil {
		pkgs := strings.Fields(string(data))
		if len(pkgs) > 0 {
			detail += " by " + strings.Join(uniqueStrings(pkgs), ", ")
		}
	}
	return Finding{Check: "reboot", Level: LevelWarn, Detail: detail, Hint: "schedule a reboot"}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// diskLevel grades usage: ≥90% fail, ≥80% warn.
func diskLevel(usedPct float64) string {
	switch {
	case usedPct >= 90:
		return LevelFail
	case usedPct >= 80:
		return LevelWarn
	default:
		return LevelOK
	}
}

func doctorDisks(rc *RunContext) []Finding {
	paths := []string{"/"}
	for _, p := range []string{rc.Config.Users.HomeBase, rc.Config.Modules.Docker.StorageDir, rc.Config.Modules.Storage.DataDir} {
		if p != "" {
			paths = append(paths, p)
		}
	}
	seenDev := map[uint64]bool{}
	var out []Finding
	for _, p := range paths {
		var st syscall.Statfs_t
		if err := syscall.Statfs(p, &st); err != nil || st.Blocks == 0 {
			continue
		}
		var fi syscall.Stat_t
		if syscall.Stat(p, &fi) == nil {
			if seenDev[uint64(fi.Dev)] {
				continue
			}
			seenDev[uint64(fi.Dev)] = true
		}
		used := float64(st.Blocks-st.Bfree) / float64(st.Blocks-st.Bfree+st.Bavail) * 100
		free := float64(st.Bavail) * float64(st.Bsize) / (1 << 30)
		f := Finding{Check: "disk " + p, Level: diskLevel(used), Detail: fmt.Sprintf("%.0f%% used, %.0f GiB free", used, free)}
		if f.Level != LevelOK {
			f.Hint = "free space (docker system prune, old checkpoints, logs)"
		}
		out = append(out, f)
	}
	return out
}

func doctorTimeSync(ctx context.Context, rc *RunContext) Finding {
	res, err := rc.Runner.Query(ctx, "timedatectl", "show", "-p", "NTPSynchronized", "--value")
	if err != nil {
		return Finding{Check: "time sync", Level: LevelSkip, Detail: "timedatectl unavailable"}
	}
	if strings.TrimSpace(res.Stdout) == "yes" {
		return Finding{Check: "time sync", Level: LevelOK, Detail: "clock synchronized"}
	}
	return Finding{Check: "time sync", Level: LevelWarn, Detail: "clock not synchronized",
		Hint: "enable systemd-timesyncd or chrony"}
}

func doctorGPU(ctx context.Context, rc *RunContext) []Finding {
	if !rc.Runner.CommandExists("nvidia-smi") {
		return nil
	}
	res, err := rc.Runner.Query(ctx, "nvidia-smi", "--query-gpu=index,persistence_mode", "--format=csv,noheader")
	if err != nil {
		msg := firstLine(err.Error())
		if res != nil && strings.Contains(res.Stdout+res.Stderr, "mismatch") {
			msg = "driver/library version mismatch"
		}
		return []Finding{{Check: "gpu driver", Level: LevelFail, Detail: "nvidia-smi failed: " + msg,
			Hint: "a driver upgrade usually needs a reboot"}}
	}
	lines := nonEmptyLines(res.Stdout)
	out := []Finding{{Check: "gpu driver", Level: LevelOK, Detail: fmt.Sprintf("%d GPU(s) visible", len(lines))}}
	var off []string
	for _, l := range lines {
		if idx, mode, ok := strings.Cut(l, ","); ok && strings.TrimSpace(mode) != "Enabled" {
			off = append(off, strings.TrimSpace(idx))
		}
	}
	if len(off) > 0 {
		out = append(out, Finding{Check: "gpu persistence", Level: LevelWarn,
			Detail: "persistence mode off on GPU " + strings.Join(off, ","),
			Hint:   "systemctl enable --now nvidia-persistenced"})
	}
	return out
}

func doctorFailedUnits(ctx context.Context, rc *RunContext) Finding {
	res, err := rc.Runner.Query(ctx, "systemctl", "--failed", "--no-legend", "--plain")
	if err != nil {
		return Finding{Check: "systemd", Level: LevelSkip, Detail: "systemctl unavailable"}
	}
	var units []string
	for _, l := range nonEmptyLines(res.Stdout) {
		units = append(units, strings.Fields(l)[0])
	}
	if len(units) == 0 {
		return Finding{Check: "systemd", Level: LevelOK, Detail: "no failed units"}
	}
	return Finding{Check: "systemd", Level: LevelWarn, Detail: "failed units: " + strings.Join(units, ", "),
		Hint: "systemctl status <unit>"}
}
