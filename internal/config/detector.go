package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// SystemInfo holds detected system information.
type SystemInfo struct {
	OS            string       `json:"os"`
	Version       string       `json:"version"`
	Codename      string       `json:"codename"`
	Arch          string       `json:"arch"`
	IsDGX         bool         `json:"is_dgx"`
	HasNVIDIAGPU  bool         `json:"has_nvidia_gpu"`
	GPUCount      int          `json:"gpu_count"`
	GPUModel      string       `json:"gpu_model"`
	CPUCores      int          `json:"cpu_cores"`
	MemoryGB      int          `json:"memory_gb"`
	StorageLayout []MountPoint `json:"storage_layout"`
}

// MountPoint represents a filesystem mount.
type MountPoint struct {
	Device    string `json:"device"`
	MountPath string `json:"mount_path"`
	FSType    string `json:"fs_type"`
	Options   string `json:"options,omitempty"`
}

// DetectSystem probes the current system and returns SystemInfo.
func DetectSystem() (*SystemInfo, error) {
	info := &SystemInfo{
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	parseOSRelease(info)
	detectDGX(info)
	detectGPU(info)
	detectMemory(info)
	detectStorage(info)

	return info, nil
}

// SuggestProfile returns a profile name based on detected system.
func (s *SystemInfo) SuggestProfile() string {
	if IsRocky(s) {
		return "rocky"
	}
	if s.IsDGX {
		return "dgx"
	}
	if s.HasNVIDIAGPU {
		return "gpu-server"
	}
	return "minimal"
}

func parseOSRelease(info *SystemInfo) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(val, "\"")
		switch key {
		case "ID":
			info.OS = val
		case "VERSION_ID":
			info.Version = val
		case "VERSION_CODENAME":
			info.Codename = val
		}
	}
}

func detectDGX(info *SystemInfo) {
	if _, err := os.Stat("/etc/dgx-release"); err == nil {
		info.IsDGX = true
		// Override OS identifier
		info.OS = "dgx-os"
	}
}

func detectGPU(info *SystemInfo) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=count,name", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return
	}
	info.HasNVIDIAGPU = true
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return
	}
	// Parse first line: "1, NVIDIA H100 80GB HBM3"
	first := strings.SplitN(lines, "\n", 2)[0]
	parts := strings.SplitN(first, ", ", 2)
	if len(parts) >= 1 {
		if count, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
			info.GPUCount = count
		}
	}
	if len(parts) >= 2 {
		info.GPUModel = strings.TrimSpace(parts[1])
	}
	// Count total GPUs from all lines
	if info.GPUCount == 0 {
		info.GPUCount = len(strings.Split(lines, "\n"))
	}
}

func detectMemory(info *SystemInfo) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.Atoi(fields[1]); err == nil {
					info.MemoryGB = kb / 1024 / 1024
				}
			}
			break
		}
	}
}

// Host state consulted when users.home_base is unset; tests redirect it.
var (
	useraddDefaultsPath        = "/etc/default/useradd"
	hostRoot                   = "/"
	rootUID             uint32 = 0
)

// managedHomeBases are checked for an existing <base>/.rootfiles, which
// means rootfiles already manages users there.
var managedHomeBases = []string{"/home", "/raid/home", "/data/home", "/nvme/home"}

// dataDriveMounts are tried in order. /mnt is excluded: cloud VMs mount
// ephemeral scratch disks there.
var dataDriveMounts = []string{"/raid", "/data", "/nvme"}

// homeFS lists local filesystems trusted to hold home directories.
var homeFS = map[string]bool{"ext4": true, "xfs": true, "btrfs": true, "zfs": true}

// defaultHomeBase picks users.home_base when a config leaves it unset. An
// existing layout wins so a host is never silently re-homed: an uncommented
// HOME= in /etc/default/useradd (the users module writes it on apply, /home
// included), then a base where rootfiles already manages users. Otherwise a
// separate read-write local data drive gets <mount>/home, on APT hosts only:
// Rocky has no SELinux home labeling for paths outside /home.
func defaultHomeBase(sys *SystemInfo) string {
	data, _ := os.ReadFile(useraddDefaultsPath)
	if hb := UseraddHome(data); hb != "" {
		return hb
	}
	for _, hb := range managedHomeBases {
		if _, err := os.Lstat(filepath.Join(hostRoot, hb, ".rootfiles")); err == nil && rootOnlyBase(hb) {
			return hb
		}
	}
	if ResolveDistro(sys).PackageBackend == "apt" {
		for _, mount := range dataDriveMounts {
			m, home := lastMount(sys, mount), lastMount(sys, mount+"/home")
			homes := mount // the visible mount that holds the homes
			if home != nil {
				homes = mount + "/home"
			}
			if homeMount(m) && (home == nil || homeMount(home)) && onSeparateDevice(homes) && rootOnlyBase(mount+"/home") {
				return mount + "/home"
			}
		}
	}
	return "/home"
}

// lastMount returns the visible (last) mount at path, or nil when nothing
// is mounted there.
func lastMount(sys *SystemInfo, path string) *MountPoint {
	var last *MountPoint
	for i := range sys.StorageLayout {
		if sys.StorageLayout[i].MountPath == path {
			last = &sys.StorageLayout[i]
		}
	}
	return last
}

// homeMount reports whether m is a read-write filesystem trusted to hold
// home directories. useradd fails on a read-only mount.
func homeMount(m *MountPoint) bool {
	return m != nil && homeFS[m.FSType] && !slices.Contains(strings.Split(m.Options, ","), "ro")
}

// onSeparateDevice reports whether path is on a different device than /.
// A bind mount of the root filesystem reports the root's filesystem type
// but is not a data drive.
func onSeparateDevice(path string) bool {
	dev, ok := deviceOf(path)
	rootDev, rootOK := deviceOf("/")
	return ok && rootOK && dev != rootDev
}

// deviceOf returns the st_dev of path under hostRoot; tests stub it.
var deviceOf = statDevice

func statDevice(path string) (uint64, bool) {
	fi, err := os.Stat(filepath.Join(hostRoot, path))
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}

// ErrAmbiguousHomeBase means detection found rootfiles metadata under a
// base other than the one it picked.
var ErrAmbiguousHomeBase = errors.New("ambiguous home base")

// detectHomeBase is defaultHomeBase, refusing to guess when another
// root-controlled base already holds rootfiles user or GPU metadata.
func detectHomeBase(sys *SystemInfo) (string, error) {
	hb := defaultHomeBase(sys)
	for _, other := range managedHomeBases {
		if other == hb || !rootOnlyBase(other) {
			continue
		}
		// A regular .rootfiles file is the legacy flat users DB.
		for _, path := range []string{
			filepath.Join(other, ".rootfiles", "users.json"),
			filepath.Join(other, ".rootfiles", "gpu-allocations.json"),
			filepath.Join(other, ".rootfiles"),
		} {
			fi, err := os.Lstat(filepath.Join(hostRoot, path))
			if err == nil && fi.Mode().IsRegular() {
				return "", fmt.Errorf("users.home_base: %w: detected %s, but %s exists; set users.home_base, ROOTFILES_HOME_BASE or --home-base", ErrAmbiguousHomeBase, hb, path)
			}
		}
	}
	return hb, nil
}

// rootOnlyBase reports whether only root controls base: its parent and, if
// present, base itself are real directories owned by root that group and
// others cannot write. Otherwise a user could pre-create the directory new
// homes go under, for example on a world-writable scratch mount.
func rootOnlyBase(base string) bool {
	if !rootOnlyDir(filepath.Join(hostRoot, filepath.Dir(base))) {
		return false
	}
	_, err := os.Lstat(filepath.Join(hostRoot, base))
	return os.IsNotExist(err) || rootOnlyDir(filepath.Join(hostRoot, base))
}

func rootOnlyDir(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.IsDir() || fi.Mode().Perm()&0o022 != 0 {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == rootUID
}

// UseraddHome returns the last HOME= value of an /etc/default/useradd
// file, cleaned, or "" when it is unset or relative. Like useradd, it only
// reads lines that start with HOME=.
func UseraddHome(data []byte) string {
	hb := ""
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "HOME="); ok {
			hb = strings.TrimSuffix(v, "\r")
		}
	}
	if !filepath.IsAbs(hb) {
		return ""
	}
	return filepath.Clean(hb)
}

// procMountsPath is read by detectStorage; tests redirect it.
var procMountsPath = "/proc/mounts"

func detectStorage(info *SystemInfo) {
	data, err := os.ReadFile(procMountsPath)
	if err != nil {
		return
	}
	interesting := []string{"/raid", "/data", "/nvme", "/mnt"}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPath := fields[1]
		for _, prefix := range interesting {
			if strings.HasPrefix(mountPath, prefix) {
				mp := MountPoint{
					Device:    fields[0],
					MountPath: mountPath,
					FSType:    fields[2],
				}
				if len(fields) > 3 {
					mp.Options = fields[3]
				}
				info.StorageLayout = append(info.StorageLayout, mp)
				break
			}
		}
	}
}
