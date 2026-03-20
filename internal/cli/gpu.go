package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/entelecheia/rootfiles-v2/internal/module"
)

func newGPUCmd() *cobra.Command {
	gpuCmd := &cobra.Command{
		Use:   "gpu",
		Short: "Manage per-user GPU allocation",
	}

	// rootfiles gpu assign USERNAME --gpus 0,1 [--method env|cgroup|both]
	assignCmd := &cobra.Command{
		Use:   "assign USERNAME",
		Short: "Assign GPUs to a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			username := args[0]

			gpuStr, _ := cmd.Flags().GetString("gpus")
			if gpuStr == "" {
				return fmt.Errorf("--gpus is required (e.g., --gpus 0,1)")
			}
			gpus, err := parseGPUList(gpuStr)
			if err != nil {
				return err
			}

			method, _ := cmd.Flags().GetString("method")
			return module.AssignGPUs(context.Background(), rc, username, gpus, method)
		},
	}
	assignCmd.Flags().String("gpus", "", "Comma-separated GPU indices (e.g., 0,1,2)")
	assignCmd.Flags().String("method", "", "Isolation method: env, cgroup, or both (default from profile)")
	gpuCmd.AddCommand(assignCmd)

	// rootfiles gpu revoke USERNAME
	gpuCmd.AddCommand(&cobra.Command{
		Use:   "revoke USERNAME",
		Short: "Revoke GPU allocation from a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.RevokeGPUs(context.Background(), rc, args[0])
		},
	})

	// rootfiles gpu list
	gpuCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List current GPU allocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.ListGPUAllocations(rc)
		},
	})

	// rootfiles gpu status
	gpuCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show GPU status with allocation info",
		RunE: func(cmd *cobra.Command, args []string) error {
			rc := buildRunContext(cmd)
			return module.ShowGPUStatus(context.Background(), rc)
		},
	})

	return gpuCmd
}

func parseGPUList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	var gpus []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		idx, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid GPU index %q: %w", p, err)
		}
		if idx < 0 {
			return nil, fmt.Errorf("GPU index must be >= 0, got %d", idx)
		}
		gpus = append(gpus, idx)
	}
	if len(gpus) == 0 {
		return nil, fmt.Errorf("no GPU indices specified")
	}
	return gpus, nil
}
