# rootfiles-v2

Go-based server bootstrapping tool for Ubuntu and NVIDIA DGX OS.

## Build & Test

```bash
make build        # → bin/rootfiles
make test         # go test ./... -race
```

## Architecture

- `cmd/rootfiles/` — entry point
- `internal/cli/` — cobra commands (apply, backup, check, config, doctor, gpu, rollback, schedule, status, tunnel, update, user); `update` keeps `upgrade` as an alias. `runtime.go` holds the root/lock preflight (mutating commands are listed in `mutatingCommands`), audit logger and `newRunner`
- `internal/config/` — config structs, YAML profile loader (profiles merged as YAML trees, strict keys), `Validate()`, system detector
- `internal/module/` — Module interface + 13 implementations (locale, system, packages, users, ssh, security, docker, nvidia, gpu, cloudflared, storage, network, monitoring), plus `doctor.go`
- `internal/exec/` — shell runner (dry-run aware; `Runner.Backup` preserves files before WriteFile/Remove/Rename/Symlink), APT wrapper
- `internal/state/` — `/var/lib/rootfiles/state.json` + history, global flock
- `internal/ui/` — interactive prompts (Charm huh) + shared output styling: `styles.go` (lipgloss palette), `markers.go` (✓ ✗ → ⚠), `format.go` (WriteHeader/Section/KV/Hint/Bullet). lipgloss auto-detects TTY and honours `NO_COLOR`; all status-style reports must go through these helpers.

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

`NewRegistry()` in `internal/module/module.go` wires all 13 implementations. `defaultOrder` in the same file defines execution sequence. These two lists MUST stay in sync — `TestRegistryDefaultOrderSync` in `module_test.go` enforces this.

## Conventions

- Go 1.23+, conventional commits
- Profiles embedded via go:embed in `internal/config/profiles/`
- Module execution order is static (defined in `module.go defaultOrder`)
- `--yes` propagates via RunContext, bypasses all prompts
- `--dry-run` logs commands without executing (write ops gated)
- GPU allocation DB writes go through `withGPUDBLock` (flock + atomic rename) to protect concurrent `gpu assign`/`revoke` calls
- File writes/removals go through `rc.Runner` (never `os.*` directly) so dry-run and backups apply; never `rm -rf` user data
- `Check()` and `Apply()` must agree: Apply only acts on what Check reports and returns `Changed` only for real work; non-fatal problems go in `ApplyResult.Warnings`
- Tests must not touch the host: override package-level path vars and stub commands on PATH (`fakeBin`) instead of calling real systemctl/apt
