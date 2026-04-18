# rootfiles-v2

Go-based server bootstrapping tool for Ubuntu and NVIDIA DGX OS.

## Build & Test

```bash
make build        # → bin/rootfiles
make test         # go test ./... -race
```

## Architecture

- `cmd/rootfiles/` — entry point
- `internal/cli/` — cobra commands (apply, backup, check, gpu, tunnel, upgrade, user)
- `internal/config/` — config structs, YAML profile loader, system detector
- `internal/module/` — Module interface + 10 implementations (locale, packages, ssh, users, docker, nvidia, gpu, cloudflared, storage, network)
- `internal/exec/` — shell runner (dry-run aware), APT wrapper
- `internal/ui/` — interactive prompts (Charm huh), unattended bypass

## Module Interface

Every module implements three methods with identical signatures:

```go
type Module interface {
    Name() string
    Check(ctx context.Context, rc *RunContext) (*CheckResult, error)
    Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error)
}
```

- `Name()` returns the module id used in `defaultOrder` and `--module` filters.
- `Check()` reports pending changes without side effects; `CheckResult.Satisfied` gates whether `Apply()` runs.
- `Apply()` performs idempotent changes; returns `ApplyResult.Changed` so callers can summarise diffs.

`NewRegistry()` in `internal/module/module.go` wires all 10 implementations. `defaultOrder` in the same file defines execution sequence. These two lists MUST stay in sync — `TestRegistryDefaultOrderSync` in `module_test.go` enforces this.

## Conventions

- Go 1.23+, conventional commits
- Profiles embedded via go:embed in `internal/config/profiles/`
- Module execution order is static (defined in `module.go defaultOrder`)
- `--yes` propagates via RunContext, bypasses all prompts
- `--dry-run` logs commands without executing (write ops gated)
- GPU allocation DB writes go through `withGPUDBLock` (flock + atomic rename) to protect concurrent `gpu assign`/`revoke` calls
