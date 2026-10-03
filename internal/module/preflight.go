package module

import "context"

// Preflight checks cross-component safety before any module can mutate host
// configuration. Installation dependencies remain in their ordered modules.
func Preflight(ctx context.Context, modules []Module, rc *RunContext) error {
	for _, m := range modules {
		if m.Name() == "monitoring" {
			return monitoringPreflight(ctx, rc)
		}
	}
	return nil
}
