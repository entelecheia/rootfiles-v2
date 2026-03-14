# rootfiles-v2

Go-based server bootstrapping tool for Ubuntu and NVIDIA DGX OS.

## Build & Test

```bash
make build        # → bin/rootfiles
make test         # go test ./... -race
```

## Architecture

- `cmd/rootfiles/` — entry point
- `internal/cli/` — cobra commands (apply, check, tunnel, user)
- `internal/config/` — config structs, YAML profile loader, system detector
- `internal/module/` — Module interface + 9 implementations (locale, packages, ssh, users, docker, nvidia, cloudflared, storage, network)
- `internal/exec/` — shell runner (dry-run aware), APT wrapper
- `internal/ui/` — interactive prompts (Charm huh), unattended bypass

## Conventions

- Go 1.23+, conventional commits
- Profiles embedded via go:embed in `internal/config/profiles/`
- Module execution order is static (defined in `module.go defaultOrder`)
- `--yes` propagates via RunContext, bypasses all prompts
- `--dry-run` logs commands without executing (write ops gated)
