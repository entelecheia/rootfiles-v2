package config

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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

func detectStorage(info *SystemInfo) {
	data, err := os.ReadFile("/proc/mounts")
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
				info.StorageLayout = append(info.StorageLayout, MountPoint{
					Device:    fields[0],
					MountPath: mountPath,
					FSType:    fields[2],
				})
				break
			}
		}
	}
}
