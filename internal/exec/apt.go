package exec

import (
	"context"
	"strings"
)

// APT wraps apt-get operations.
type APT struct {
	Runner *Runner
}

// NewAPT creates a new APT wrapper.
func NewAPT(runner *Runner) *APT {
	return &APT{Runner: runner}
}

// aptEnv keeps apt/dpkg from prompting: debconf questions (tzdata, …),
// and needrestart's interactive service-restart dialog on Ubuntu 22.04+.
var aptEnv = []string{
	"DEBIAN_FRONTEND=noninteractive",
	"NEEDRESTART_MODE=a",
}

// aptLockOpts waits for the dpkg lock (held e.g. by unattended-upgrades on
// a freshly booted server) instead of failing immediately.
var aptLockOpts = []string{"-o", "DPkg::Lock::Timeout=300"}

// Update runs apt-get update.
func (a *APT) Update(ctx context.Context) error {
	args := append(append([]string{}, aptLockOpts...), "update", "-qq")
	_, err := a.Runner.RunEnv(ctx, aptEnv, "apt-get", args...)
	return err
}

// Install installs packages via apt-get. Existing modified conffiles are
// kept (confold) so an install never blocks on a conffile prompt, and
// --no-remove aborts instead of silently removing conflicting packages
// (e.g. ntp when systemd-timesyncd is requested).
func (a *APT) Install(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{}, aptLockOpts...)
	args = append(args,
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold",
		"install", "-y", "-qq", "--no-remove")
	args = append(args, packages...)
	_, err := a.Runner.RunEnv(ctx, aptEnv, "apt-get", args...)
	return err
}

// IsInstalled checks if a package is installed (always queries the real system).
func (a *APT) IsInstalled(pkg string) bool {
	return a.Installed([]string{pkg})[pkg]
}

// Installed reports which of pkgs are installed, using a single
// dpkg-query call. Unknown packages are simply absent from the result.
func (a *APT) Installed(pkgs []string) map[string]bool {
	if len(pkgs) == 0 {
		return map[string]bool{}
	}
	args := append([]string{"-W", "-f=${Package} ${Status}\\n"}, pkgs...)
	// dpkg-query exits non-zero when any package is unknown but still
	// prints the known ones, so parse stdout regardless of the error.
	res, _ := a.Runner.Query(context.Background(), "dpkg-query", args...)
	if res == nil {
		return map[string]bool{}
	}
	return parseDpkgQuery(res.Stdout)
}

// parseDpkgQuery parses `dpkg-query -W -f='${Package} ${Status}\n'` output.
func parseDpkgQuery(out string) map[string]bool {
	installed := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		// "<pkg>[:arch] <want> ok installed" (want is install or hold)
		if len(f) == 4 && f[2] == "ok" && f[3] == "installed" {
			installed[strings.SplitN(f[0], ":", 2)[0]] = true
		}
	}
	return installed
}

// AddKeyring downloads a GPG key and saves it to /etc/apt/keyrings/.
func (a *APT) AddKeyring(ctx context.Context, name, keyURL string) error {
	keyPath := "/etc/apt/keyrings/" + name + ".gpg"
	if a.Runner.FileExists(keyPath) {
		return nil
	}
	if err := a.Runner.MkdirAll("/etc/apt/keyrings", 0755); err != nil {
		return err
	}
	_, err := a.Runner.RunShell(ctx, "curl -fsSL "+keyURL+" | gpg --batch --yes --dearmor -o "+keyPath)
	return err
}

// AddSourceList writes an APT source list file.
func (a *APT) AddSourceList(ctx context.Context, name, content string) error {
	path := "/etc/apt/sources.list.d/" + name + ".list"
	return a.Runner.WriteFile(path, []byte(content+"\n"), 0644)
}
