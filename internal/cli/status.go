package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	execpkg "github.com/entelecheia/rootfiles-v2/internal/exec"
	"github.com/entelecheia/rootfiles-v2/internal/module"
	"github.com/entelecheia/rootfiles-v2/internal/ui"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show full system status at a glance",
		Long:  "Unified dashboard: system, profile, modules, GPU allocations, tunnel, users.",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	out := cmd.OutOrStdout()

	profileName, _ := cmd.Flags().GetString("profile")
	configPath, _ := cmd.Flags().GetString("config")
	if profileName == "" {
		profileName = os.Getenv("ROOTFILES_PROFILE")
	}

	sysInfo, _ := config.DetectSystem()
	if sysInfo == nil {
		sysInfo = &config.SystemInfo{}
	}

	active := profileName
	if active == "" && configPath == "" {
		active = sysInfo.SuggestProfile()
	}

	cfg, cfgErr := config.Load(active, configPath, sysInfo)
	if cfg == nil {
		cfg = &config.Config{}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	runner := execpkg.NewRunner(true, logger)
	rc := &module.RunContext{
		Config: cfg,
		Runner: runner,
		APT:    execpkg.NewAPT(runner),
		DryRun: true,
		Yes:    true,
	}

	ui.WriteHeader(out, "rootfiles status")

	renderSystemSection(out, sysInfo)
	renderProfileSection(out, active, profileName, sysInfo, configPath, cfgErr)
	renderModulesSection(ctx, out, rc)
	renderGPUSection(out, rc)
	renderTunnelSection(ctx, out, rc)
	renderUsersSection(ctx, out, rc)

	fmt.Fprintln(out)
	return nil
}

func renderSystemSection(out io.Writer, sys *config.SystemInfo) {
	ui.WriteSection(out, "System")

	osLine := firstNonEmpty(sys.OS, "unknown")
	if sys.Version != "" {
		osLine = fmt.Sprintf("%s %s", osLine, sys.Version)
	}
	if sys.Codename != "" {
		osLine = fmt.Sprintf("%s (%s)", osLine, sys.Codename)
	}
	ui.WriteKV(out, "OS", osLine)

	if host, err := os.Hostname(); err == nil && host != "" {
		ui.WriteKV(out, "Hostname", host)
	}
	ui.WriteKV(out, "Architecture", firstNonEmpty(sys.Arch, "unknown"))
	if sys.MemoryGB > 0 {
		ui.WriteKV(out, "Memory", fmt.Sprintf("%d GiB", sys.MemoryGB))
	}
	if sys.CPUCores > 0 {
		ui.WriteKV(out, "CPU Cores", fmt.Sprintf("%d", sys.CPUCores))
	}

	switch {
	case sys.IsDGX:
		ui.WriteKV(out, "GPU", fmt.Sprintf("DGX %s × %s", countStr(sys.GPUCount), firstNonEmpty(sys.GPUModel, "NVIDIA")))
	case sys.HasNVIDIAGPU:
		ui.WriteKV(out, "GPU", fmt.Sprintf("%s × %s", countStr(sys.GPUCount), firstNonEmpty(sys.GPUModel, "NVIDIA")))
	default:
		ui.WriteKV(out, "GPU", "none detected")
	}

	if paths := uniqueMountRoots(sys.StorageLayout); len(paths) > 0 {
		ui.WriteKV(out, "Storage", strings.Join(paths, ", "))
	}
}

func renderProfileSection(out io.Writer, active, flagProfile string, sys *config.SystemInfo, configPath string, loadErr error) {
	ui.WriteSection(out, "Profile")

	ui.WriteKV(out, "Active", firstNonEmpty(active, "(none)"))
	ui.WriteKV(out, "Suggested", firstNonEmpty(sys.SuggestProfile(), "minimal"))
	switch {
	case configPath != "":
		ui.WriteKV(out, "Config", configPath)
	case flagProfile != "":
		ui.WriteKV(out, "Config", "(embedded profile)")
	default:
		ui.WriteKV(out, "Config", "(auto)")
	}
	if loadErr != nil {
		ui.WriteHint(out, fmt.Sprintf("%s config load failed: %v", ui.WarnMark(), loadErr))
	}
}

func renderModulesSection(ctx context.Context, out io.Writer, rc *module.RunContext) {
	reg := module.NewRegistry()
	modules := reg.Resolve(rc.Config, nil)
	if len(modules) == 0 {
		ui.WriteSection(out, "Modules (none enabled)")
		ui.WriteHint(out, "no profile selected — select one with --profile")
		return
	}

	results, err := module.CheckAll(ctx, modules, rc)
	if err != nil {
		ui.WriteSection(out, "Modules (check failed)")
		ui.WriteHint(out, fmt.Sprintf("%s %v", ui.WarnMark(), err))
		return
	}

	satisfied := 0
	for _, m := range modules {
		if r := results[m.Name()]; r != nil && r.Satisfied {
			satisfied++
		}
	}
	ui.WriteSection(out, fmt.Sprintf("Modules (%d/%d satisfied)", satisfied, len(modules)))

	for _, m := range modules {
		r := results[m.Name()]
		marker := ui.OKMark()
		if r == nil || !r.Satisfied {
			marker = ui.PendingMark()
		}
		ui.WriteBullet(out, marker, m.Name())
	}

	pending := len(modules) - satisfied
	if pending > 0 {
		fmt.Fprintln(out)
		ui.WriteHint(out, fmt.Sprintf("%d module(s) need attention — run 'rootfiles check' for details.", pending))
	}
}

func renderGPUSection(out io.Writer, rc *module.RunContext) {
	db, _ := module.LoadGPUDB(rc)
	ui.WriteSection(out, "GPU Allocations")

	if db == nil || (db.TotalGPUs == 0 && len(db.Allocations) == 0) {
		ui.WriteKV(out, "Total GPUs", "none recorded")
		ui.WriteHint(out, "run 'rootfiles gpu assign <user> <indices>' to register an allocation")
		return
	}

	totalLabel := fmt.Sprintf("%d", db.TotalGPUs)
	if db.GPUModel != "" {
		totalLabel = fmt.Sprintf("%d × %s", db.TotalGPUs, db.GPUModel)
	}
	ui.WriteKV(out, "Total GPUs", totalLabel)

	assignedGPUs := 0
	for _, a := range db.Allocations {
		assignedGPUs += len(a.GPUs)
	}
	if len(db.Allocations) == 0 {
		ui.WriteKV(out, "Assigned", "none")
		return
	}
	ui.WriteKV(out, "Assigned", fmt.Sprintf("%d GPU(s) across %d user(s)", assignedGPUs, len(db.Allocations)))

	// Sort allocations by username for deterministic output.
	allocs := append([]module.GPUAllocation(nil), db.Allocations...)
	sort.Slice(allocs, func(i, j int) bool { return allocs[i].Username < allocs[j].Username })
	for _, a := range allocs {
		ids := make([]string, 0, len(a.GPUs))
		for _, g := range a.GPUs {
			ids = append(ids, fmt.Sprintf("%d", g))
		}
		ui.WriteBullet(out, ui.StyleValue.Render(a.Username), strings.Join(ids, ","))
	}
}

const cloudflaredStatusBinary = "/usr/local/bin/cloudflared"

func renderTunnelSection(ctx context.Context, out io.Writer, rc *module.RunContext) {
	ui.WriteSection(out, "Tunnel")

	binary := "(not installed)"
	if rc.Runner.FileExists(cloudflaredStatusBinary) {
		if res, err := rc.Runner.Query(ctx, cloudflaredStatusBinary, "--version"); err == nil {
			binary = strings.TrimSpace(res.Stdout)
		} else {
			binary = cloudflaredStatusBinary
		}
	}
	ui.WriteKV(out, "cloudflared", binary)

	switch status := systemctlStatus(ctx, "cloudflared"); status {
	case "active":
		ui.WriteKV(out, "Service", ui.StyleSuccess.Render("active"))
	case "unknown":
		ui.WriteKV(out, "Service", ui.StyleHint.Render("unknown"))
	default:
		ui.WriteKV(out, "Service", ui.StyleHint.Render(status))
	}

	iface := rc.Config.Modules.Cloudflared.PrivateNetwork.Interface
	if iface == "" {
		iface = "vlan0"
	}
	if addr := interfaceAddress(iface); addr != "" {
		ui.WriteKV(out, "VLAN", fmt.Sprintf("%s on %s", addr, iface))
	} else {
		ui.WriteKV(out, "VLAN", fmt.Sprintf("%s not configured", iface))
	}
}

func renderUsersSection(ctx context.Context, out io.Writer, rc *module.RunContext) {
	ui.WriteSection(out, "Users")

	homeBase := rc.Config.Users.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}
	ui.WriteKV(out, "Home base", homeBase)

	db, _ := module.LoadUsersDB(rc)
	managed := 0
	if db != nil {
		managed = len(db.Users)
	}
	ui.WriteKV(out, "Managed", fmt.Sprintf("%d user(s)", managed))

	// System-user count: ignore scan errors (missing /etc/passwd, etc.).
	sysUsers, err := module.ScanSystemUsersExported(ctx, rc)
	if err == nil {
		ui.WriteKV(out, "System", fmt.Sprintf("%d user(s) (UID 1000-65533)", len(sysUsers)))
	}
}

// --- helpers ---

// systemctlStatus returns the systemd unit state (active/inactive/failed/
// unknown) without erroring when systemctl is absent.
func systemctlStatus(ctx context.Context, unit string) string {
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", unit)
	out, err := cmd.Output()
	state := strings.TrimSpace(string(out))
	if state == "" && err != nil {
		return "unknown"
	}
	return state
}

// interfaceAddress returns the first IPv4 inet address on iface or "" if the
// interface does not exist / has no address / `ip` binary is missing.
func interfaceAddress(iface string) string {
	out, err := exec.Command("ip", "-brief", "addr", "show", iface).Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	for _, f := range fields {
		if strings.Contains(f, "/") && strings.Count(f, ".") == 3 {
			return f
		}
	}
	return ""
}

// uniqueMountRoots deduplicates mount paths to their two-level prefix so
// Docker overlay mounts (/raid/docker/overlay2/<hash>/merged) collapse into
// a single "/raid/docker" entry instead of flooding the status output.
func uniqueMountRoots(mounts []config.MountPoint) []string {
	seen := make(map[string]bool, len(mounts))
	var out []string
	for _, m := range mounts {
		root := mountRoot(m.MountPath)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		out = append(out, root)
	}
	sort.Strings(out)
	return out
}

func mountRoot(p string) string {
	parts := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 3)
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return "/" + parts[0]
	default:
		return "/" + parts[0] + "/" + parts[1]
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func countStr(n int) string {
	if n <= 0 {
		return "1"
	}
	return fmt.Sprintf("%d", n)
}
