package module

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GPUAllocation represents a per-user GPU assignment.
type GPUAllocation struct {
	Username  string `json:"username"`
	GPUs      []int  `json:"gpus"`
	Method    string `json:"method"`
	UpdatedAt string `json:"updated_at"`
}

// GPUAllocationsDB is the persistent store for GPU allocations.
type GPUAllocationsDB struct {
	Version     int             `json:"version"`
	TotalGPUs   int             `json:"total_gpus"`
	GPUModel    string          `json:"gpu_model"`
	Allocations []GPUAllocation `json:"allocations"`
}

type GPUModule struct{}

func NewGPUModule() *GPUModule { return &GPUModule{} }
func (m *GPUModule) Name() string { return "gpu" }

func (m *GPUModule) Check(_ context.Context, rc *RunContext) (*CheckResult, error) {
	db, err := loadGPUDB(rc)
	if err != nil || len(db.Allocations) == 0 {
		return &CheckResult{Satisfied: true}, nil
	}

	var changes []Change
	for _, alloc := range db.Allocations {
		method := effectiveMethod(rc, alloc.Method)

		if method == "env" || method == "both" {
			scriptPath := profileDScriptPath(alloc.Username)
			expected := buildProfileDScript(alloc.Username, alloc.GPUs)
			data, _ := rc.Runner.ReadFile(scriptPath)
			if string(data) != expected {
				changes = append(changes, Change{
					Description: fmt.Sprintf("Write GPU env script for %s (GPUs: %s)", alloc.Username, gpuListStr(alloc.GPUs)),
					Command:     fmt.Sprintf("write %s", scriptPath),
				})
			}
		}

		if method == "cgroup" || method == "both" {
			u, err := user.Lookup(alloc.Username)
			if err != nil {
				continue
			}
			slicePath := cgroupSlicePath(u.Uid)
			expected := buildCgroupConf(alloc.GPUs)
			data, _ := rc.Runner.ReadFile(slicePath)
			if string(data) != expected {
				changes = append(changes, Change{
					Description: fmt.Sprintf("Write cgroup GPU slice for %s (GPUs: %s)", alloc.Username, gpuListStr(alloc.GPUs)),
					Command:     fmt.Sprintf("write %s", slicePath),
				})
			}
		}
	}

	return &CheckResult{
		Satisfied: len(changes) == 0,
		Changes:   changes,
	}, nil
}

func (m *GPUModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	db, err := loadGPUDB(rc)
	if err != nil || len(db.Allocations) == 0 {
		return &ApplyResult{Changed: false}, nil
	}

	var messages []string
	changed := false

	for _, alloc := range db.Allocations {
		method := effectiveMethod(rc, alloc.Method)

		if method == "env" || method == "both" {
			scriptPath := profileDScriptPath(alloc.Username)
			expected := buildProfileDScript(alloc.Username, alloc.GPUs)
			data, _ := rc.Runner.ReadFile(scriptPath)
			if string(data) != expected {
				if err := rc.Runner.WriteFile(scriptPath, []byte(expected), 0644); err != nil {
					return nil, fmt.Errorf("writing profile.d script for %s: %w", alloc.Username, err)
				}
				messages = append(messages, fmt.Sprintf("GPU env script for %s (GPUs: %s)", alloc.Username, gpuListStr(alloc.GPUs)))
				changed = true
			}
		}

		if method == "cgroup" || method == "both" {
			u, err := user.Lookup(alloc.Username)
			if err != nil {
				continue
			}
			slicePath := cgroupSlicePath(u.Uid)
			expected := buildCgroupConf(alloc.GPUs)
			data, _ := rc.Runner.ReadFile(slicePath)
			if string(data) != expected {
				sliceDir := filepath.Dir(slicePath)
				if err := rc.Runner.MkdirAll(sliceDir, 0755); err != nil {
					return nil, fmt.Errorf("creating slice dir for %s: %w", alloc.Username, err)
				}
				if err := rc.Runner.WriteFile(slicePath, []byte(expected), 0644); err != nil {
					return nil, fmt.Errorf("writing cgroup conf for %s: %w", alloc.Username, err)
				}
				rc.Runner.Run(ctx, "systemctl", "daemon-reload")
				messages = append(messages, fmt.Sprintf("cgroup GPU slice for %s (GPUs: %s)", alloc.Username, gpuListStr(alloc.GPUs)))
				changed = true
			}
		}
	}

	return &ApplyResult{Changed: changed, Messages: messages}, nil
}

// AssignGPUs assigns GPUs to a user and applies immediately.
func AssignGPUs(ctx context.Context, rc *RunContext, username string, gpus []int, method string) error {
	if _, err := user.Lookup(username); err != nil {
		return fmt.Errorf("user %s not found", username)
	}

	if method == "" {
		method = effectiveMethod(rc, "")
	}

	db, _ := loadGPUDB(rc)
	if db.Version == 0 {
		db.Version = 1
	}

	// Detect total GPUs and model
	totalGPUs, gpuModel := detectGPUInfo(ctx, rc)
	if totalGPUs > 0 {
		db.TotalGPUs = totalGPUs
		db.GPUModel = gpuModel
	}

	// Validate GPU indices
	if db.TotalGPUs > 0 {
		for _, g := range gpus {
			if g < 0 || g >= db.TotalGPUs {
				return fmt.Errorf("GPU index %d out of range (0-%d)", g, db.TotalGPUs-1)
			}
		}
	}

	// Check for conflicts
	for _, alloc := range db.Allocations {
		if alloc.Username == username {
			continue
		}
		for _, existing := range alloc.GPUs {
			for _, requested := range gpus {
				if existing == requested {
					return fmt.Errorf("GPU %d is already assigned to %s", requested, alloc.Username)
				}
			}
		}
	}

	// Update or add allocation
	alloc := GPUAllocation{
		Username:  username,
		GPUs:      gpus,
		Method:    method,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	found := false
	for i, a := range db.Allocations {
		if a.Username == username {
			db.Allocations[i] = alloc
			found = true
			break
		}
	}
	if !found {
		db.Allocations = append(db.Allocations, alloc)
	}

	if err := saveGPUDB(rc, db); err != nil {
		return err
	}

	// Apply immediately
	if method == "env" || method == "both" {
		scriptPath := profileDScriptPath(username)
		content := buildProfileDScript(username, gpus)
		if rc.DryRun {
			fmt.Printf("[dry-run] would write %s\n", scriptPath)
		} else {
			if err := rc.Runner.WriteFile(scriptPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing profile.d script: %w", err)
			}
		}
	}

	if method == "cgroup" || method == "both" {
		u, _ := user.Lookup(username)
		slicePath := cgroupSlicePath(u.Uid)
		content := buildCgroupConf(gpus)
		if rc.DryRun {
			fmt.Printf("[dry-run] would write %s\n", slicePath)
		} else {
			sliceDir := filepath.Dir(slicePath)
			rc.Runner.MkdirAll(sliceDir, 0755)
			if err := rc.Runner.WriteFile(slicePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing cgroup conf: %w", err)
			}
			rc.Runner.Run(ctx, "systemctl", "daemon-reload")
		}
	}

	fmt.Printf("Assigned GPUs %s to %s (method: %s)\n", gpuListStr(gpus), username, method)
	return nil
}

// RevokeGPUs removes GPU assignment for a user.
func RevokeGPUs(ctx context.Context, rc *RunContext, username string) error {
	db, err := loadGPUDB(rc)
	if err != nil {
		return fmt.Errorf("loading GPU database: %w", err)
	}

	found := false
	var method string
	var remaining []GPUAllocation
	for _, a := range db.Allocations {
		if a.Username == username {
			found = true
			method = a.Method
		} else {
			remaining = append(remaining, a)
		}
	}
	if !found {
		return fmt.Errorf("no GPU allocation found for %s", username)
	}

	db.Allocations = remaining
	if err := saveGPUDB(rc, db); err != nil {
		return err
	}

	if method == "" {
		method = effectiveMethod(rc, "")
	}

	// Clean up env script
	if method == "env" || method == "both" {
		scriptPath := profileDScriptPath(username)
		if rc.DryRun {
			fmt.Printf("[dry-run] would remove %s\n", scriptPath)
		} else {
			rc.Runner.Run(ctx, "rm", "-f", scriptPath)
		}
	}

	// Clean up cgroup conf
	if method == "cgroup" || method == "both" {
		u, err := user.Lookup(username)
		if err == nil {
			slicePath := cgroupSlicePath(u.Uid)
			sliceDir := filepath.Dir(slicePath)
			if rc.DryRun {
				fmt.Printf("[dry-run] would remove %s\n", sliceDir)
			} else {
				rc.Runner.Run(ctx, "rm", "-rf", sliceDir)
				rc.Runner.Run(ctx, "systemctl", "daemon-reload")
			}
		}
	}

	fmt.Printf("Revoked GPU allocation for %s\n", username)
	return nil
}

// ListGPUAllocations prints the current GPU allocation table.
func ListGPUAllocations(rc *RunContext) error {
	db, err := loadGPUDB(rc)
	if err != nil {
		fmt.Println("No GPU allocations configured.")
		return nil
	}
	if len(db.Allocations) == 0 {
		fmt.Println("No GPU allocations configured.")
		return nil
	}

	if db.TotalGPUs > 0 {
		fmt.Printf("Total GPUs: %d", db.TotalGPUs)
		if db.GPUModel != "" {
			fmt.Printf(" (%s)", db.GPUModel)
		}
		fmt.Println()
	}

	fmt.Printf("\n%-15s %-15s %-10s %s\n", "USER", "GPUs", "METHOD", "UPDATED")
	fmt.Printf("%-15s %-15s %-10s %s\n", "----", "----", "------", "-------")
	for _, a := range db.Allocations {
		fmt.Printf("%-15s %-15s %-10s %s\n",
			a.Username, gpuListStr(a.GPUs), a.Method, a.UpdatedAt)
	}
	return nil
}

// gpuSMIInfo holds per-GPU nvidia-smi data.
type gpuSMIInfo struct {
	Name     string
	MemUsed  string
	MemTotal string
	Util     string
}

// ShowGPUStatus shows nvidia-smi output cross-referenced with allocations.
func ShowGPUStatus(ctx context.Context, rc *RunContext) error {
	totalGPUs := countGPUDevices()
	if totalGPUs == 0 {
		return fmt.Errorf("no NVIDIA GPU devices found in /dev")
	}

	// Query nvidia-smi — may only see a subset due to cgroup restrictions.
	// We parse what's visible and leave the rest as "N/A".
	smiData := make(map[int]*gpuSMIInfo)
	result, err := rc.Runner.Query(ctx, "nvidia-smi",
		"--query-gpu=index,name,memory.used,memory.total,utilization.gpu",
		"--format=csv,noheader,nounits")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Split(line, ", ")
			if len(fields) < 5 {
				continue
			}
			idx, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
			smiData[idx] = &gpuSMIInfo{
				Name:     strings.TrimSpace(fields[1]),
				MemUsed:  strings.TrimSpace(fields[2]),
				MemTotal: strings.TrimSpace(fields[3]),
				Util:     strings.TrimSpace(fields[4]),
			}
		}
	}

	db, _ := loadGPUDB(rc)

	// Build assignment map
	assignMap := make(map[int]string)
	for _, a := range db.Allocations {
		for _, g := range a.GPUs {
			assignMap[g] = a.Username
		}
	}

	// Use model from DB if available (visible GPUs may be restricted)
	defaultModel := db.GPUModel
	if defaultModel == "" {
		for _, info := range smiData {
			defaultModel = info.Name
			break
		}
	}

	fmt.Printf("%-5s %-25s %-15s %-6s %s\n", "GPU", "MODEL", "MEMORY", "UTIL", "ASSIGNED TO")
	fmt.Printf("%-5s %-25s %-15s %-6s %s\n", "---", "-----", "------", "----", "-----------")

	for i := 0; i < totalGPUs; i++ {
		assigned := "(unassigned)"
		if u, ok := assignMap[i]; ok {
			assigned = u
		}

		if info, ok := smiData[i]; ok {
			name := info.Name
			if len(name) > 24 {
				name = name[:24]
			}
			fmt.Printf("%-5d %-25s %5s/%-5s MB %4s%%  %s\n",
				i, name, info.MemUsed, info.MemTotal, info.Util, assigned)
		} else {
			// GPU exists but not visible to nvidia-smi (cgroup restricted)
			name := defaultModel
			if name == "" {
				name = "NVIDIA GPU"
			}
			if len(name) > 24 {
				name = name[:24]
			}
			fmt.Printf("%-5d %-25s   (restricted)     ---   %s\n",
				i, name, assigned)
		}
	}

	return nil
}

// --- helpers ---

func gpuDBPath(rc *RunContext) string {
	homeBase := rc.Config.Users.HomeBase
	if homeBase == "" {
		homeBase = "/home"
	}
	return filepath.Join(homeBase, ".rootfiles", "gpu-allocations.json")
}

func loadGPUDB(rc *RunContext) (*GPUAllocationsDB, error) {
	var db GPUAllocationsDB
	data, err := rc.Runner.ReadFile(gpuDBPath(rc))
	if err != nil {
		return &db, err
	}
	if err := json.Unmarshal(data, &db); err != nil {
		return &db, fmt.Errorf("parsing GPU database: %w", err)
	}
	return &db, nil
}

func saveGPUDB(rc *RunContext, db *GPUAllocationsDB) error {
	dbPath := gpuDBPath(rc)
	metaDir := filepath.Dir(dbPath)
	rc.Runner.MkdirAll(metaDir, 0755)
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling GPU database: %w", err)
	}
	return rc.Runner.WriteFile(dbPath, data, 0600)
}

func effectiveMethod(rc *RunContext, method string) string {
	if method != "" {
		return method
	}
	m := rc.Config.Modules.Nvidia.GPUAllocation.Method
	if m == "" {
		return "env"
	}
	return m
}

func profileDScriptPath(username string) string {
	return fmt.Sprintf("/etc/profile.d/rootfiles-gpu-%s.sh", username)
}

func buildProfileDScript(username string, gpus []int) string {
	gpuStr := gpuListStr(gpus)
	return fmt.Sprintf(`# Managed by rootfiles-v2 — do not edit
if [ "$(id -un)" = "%s" ]; then
    export CUDA_VISIBLE_DEVICES=%s
    export NVIDIA_VISIBLE_DEVICES=%s
fi
`, username, gpuStr, gpuStr)
}

func cgroupSlicePath(uid string) string {
	return fmt.Sprintf("/etc/systemd/system/user-%s.slice.d/gpu-access.conf", uid)
}

func buildCgroupConf(gpus []int) string {
	var lines []string
	lines = append(lines, "# Managed by rootfiles-v2 — do not edit")
	lines = append(lines, "[Slice]")
	for _, g := range gpus {
		lines = append(lines, fmt.Sprintf("DeviceAllow=/dev/nvidia%d rwm", g))
	}
	lines = append(lines, "DeviceAllow=/dev/nvidiactl rwm")
	lines = append(lines, "DeviceAllow=/dev/nvidia-uvm rwm")
	lines = append(lines, "DeviceAllow=/dev/nvidia-uvm-tools rwm")
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func gpuListStr(gpus []int) string {
	sorted := make([]int, len(gpus))
	copy(sorted, gpus)
	sort.Ints(sorted)
	parts := make([]string, len(sorted))
	for i, g := range sorted {
		parts[i] = strconv.Itoa(g)
	}
	return strings.Join(parts, ",")
}

func detectGPUInfo(ctx context.Context, rc *RunContext) (int, string) {
	// Count GPU devices from /dev/nvidia* to avoid cgroup visibility restrictions.
	// nvidia-smi only sees GPUs allowed by the calling user's cgroup, but
	// /dev/nvidia[0-9]* device files always exist for all physical GPUs.
	count := countGPUDevices()
	model := ""

	// Try nvidia-smi for the model name (may see fewer GPUs if cgroup-restricted)
	result, err := rc.Runner.Query(ctx, "nvidia-smi",
		"--query-gpu=name", "--format=csv,noheader")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				model = line
				break
			}
		}
	}

	return count, model
}

// countGPUDevices counts /dev/nvidia[0-9]+ device files.
func countGPUDevices() int {
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "nvidia") {
			rest := name[len("nvidia"):]
			if rest != "" {
				if _, err := strconv.Atoi(rest); err == nil {
					count++
				}
			}
		}
	}
	return count
}
