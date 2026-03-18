# rootfiles-v2

[![Test](https://github.com/entelecheia/rootfiles-v2/actions/workflows/test.yaml/badge.svg)](https://github.com/entelecheia/rootfiles-v2/actions/workflows/test.yaml)
[![Release](https://img.shields.io/github/v/release/entelecheia/rootfiles-v2)](https://github.com/entelecheia/rootfiles-v2/releases/latest)

Server bootstrapping tool for Ubuntu and NVIDIA DGX OS. Single binary, declarative profiles, root-level system configuration.

## What it does

`rootfiles-v2` handles everything that requires root on a fresh server — so users can immediately run [dotfiles-v2](https://github.com/entelecheia/dotfiles-v2) for their personal environment.

```
rootfiles-v2 (root)              →  dotfiles-v2 (user)
━━━━━━━━━━━━━━━━━━━━━━           ━━━━━━━━━━━━━━━━━━━━
System packages (apt)             User dotfiles (chezmoi)
User accounts (/raid/home/)       Shell (zsh, starship, oh-my-zsh)
SSH server hardening              Dev tools (fnm, uv, pipx)
Docker + NVIDIA toolkit           Homebrew packages
Cloudflare tunnel + VLAN          AI tools (Claude Code)
Locale, timezone, firewall        Secrets (age)
Storage mounts & symlinks
```

## Install

```bash
# Latest stable release (recommended)
curl -fsSL https://raw.githubusercontent.com/entelecheia/rootfiles-v2/main/scripts/install.sh | sudo bash

# Specific version
curl -fsSL ... | sudo bash -s -- --version v0.1.0

# Dev channel (build from source, requires Go)
curl -fsSL ... | sudo bash -s -- --channel dev
```

The installer downloads a prebuilt binary, verifies its SHA256 checksum, and places it at `/usr/local/bin/rootfiles`.

## Quick start

```bash
# Interactive mode — walks you through every setting before applying
sudo rootfiles apply

# Or specify everything upfront (skips all prompts)
sudo rootfiles apply --profile dgx --yes
```

In interactive mode, `apply` presents each configurable setting (SSH, firewall, VLAN, storage, etc.) for review and lets you adjust values before execution. Use `--yes` to skip all prompts for CI/automation.

## Profiles

| Profile | Extends | Use case |
|---------|---------|----------|
| `base` | — | Locale, packages, SSH |
| `minimal` | base | + users, cloudflared, ufw |
| `dgx` | minimal | + Docker, NVIDIA toolkit, VLAN, RAID storage |
| `gpu-server` | minimal | + Docker, NVIDIA toolkit (non-DGX) |
| `full` | minimal | + Docker, storage, network |

DGX OS is auto-detected (`/etc/dgx-release`) and the appropriate profile is suggested.

## Modules

All modules are idempotent and support `--dry-run`.

| Module | Description |
|--------|-------------|
| `locale` | Locale generation, timezone |
| `packages` | APT package installation (18+ packages) |
| `ssh` | sshd hardening (root login, password auth, port) |
| `users` | User creation with custom home dirs, backup/restore |
| `docker` | Docker CE + daemon.json + storage relocation |
| `nvidia` | NVIDIA Container Toolkit |
| `cloudflared` | Cloudflare Tunnel + VLAN private network |
| `storage` | RAID/NVMe directory setup, symlinks |
| `network` | UFW firewall, port rules |

## Usage

### Apply configuration

```bash
# Interactive — prompts for profile, then walks through each setting:
#   SSH (root login, password auth, port)
#   Users (home base, sudo nopasswd)
#   Docker (storage dir)
#   Cloudflared (tunnel token, VLAN address)
#   Network (UFW, allowed ports)
#   Storage (data dir)
sudo rootfiles apply

# Full profile, interactive config review
sudo rootfiles apply --profile dgx

# Specific modules only
sudo rootfiles apply --module cloudflared,docker

# Dry-run (preview settings and changes, no execution)
sudo rootfiles apply --profile dgx --dry-run

# Unattended (CI/automation — skips all interactive prompts)
sudo rootfiles apply --profile dgx --yes \
  --home-base /raid/home \
  --tunnel-token "$CF_TUNNEL_TOKEN" \
  --vlan-address "172.16.229.32/32" \
  --user yjlee --ssh-pubkey "ssh-ed25519 AAAA..."
```

### Check system state

```bash
sudo rootfiles check --profile dgx
```

### User management

Users are created at a custom home base (e.g., `/raid/home/`) that survives OS reinstalls.

```bash
# Create user
sudo rootfiles user add yjlee --pubkey "ssh-ed25519 AAAA..."

# List managed users
sudo rootfiles user list

# Backup before OS reinstall
sudo rootfiles user backup

# Restore after OS reinstall (/raid/home/ preserved)
sudo rootfiles user restore

# Move existing user from /home/ to custom home
sudo rootfiles user rehome yjlee
```

### Cloudflare tunnel + VLAN

```bash
# Setup tunnel with private network
sudo rootfiles tunnel setup "$TOKEN" --vlan-address "172.16.229.32/32"

# Manage
sudo rootfiles tunnel status
sudo rootfiles tunnel update
sudo rootfiles tunnel restart
sudo rootfiles tunnel uninstall
```

## Environment variables

All flags can be set via environment variables for unattended operation:

| Variable | Description | Default |
|----------|-------------|---------|
| `ROOTFILES_PROFILE` | Profile name | `minimal` |
| `ROOTFILES_YES` | Skip all prompts | `false` |
| `ROOTFILES_HOME_BASE` | Custom home directory | `/home` |
| `ROOTFILES_USER` | Username to create | — |
| `ROOTFILES_TUNNEL_TOKEN` | Cloudflare tunnel token | — |
| `ROOTFILES_VLAN_ADDRESS` | VLAN private IP | — |
| `ROOTFILES_SSH_PUBKEY` | SSH public key | — |
| `ROOTFILES_TIMEZONE` | Timezone | `Asia/Seoul` |
| `ROOTFILES_DOCKER_ROOT` | Docker storage path | `/var/lib/docker` |

## Build from source

```bash
make build    # → bin/rootfiles
make test     # go test with race detection
```

Requires Go 1.23+.

## Architecture

```
cmd/rootfiles/        Entry point
internal/
  cli/                Cobra commands (apply, check, tunnel, user)
  config/             YAML profiles with inheritance, system detector
    profiles/         Embedded profile YAMLs (go:embed)
  module/             9 modules implementing Module interface
  exec/               Shell runner (dry-run aware), APT wrapper
  ui/                 Interactive prompts (Charm huh)
```

## CI

31 jobs across 3 test layers:
- **Unit**: Go tests with race detection
- **Integration**: 3 OS images (Ubuntu 22.04, 24.04, DGX mock) × 4 profiles
- **Module**: 2 OS × 7 modules (individual isolation)
- **Scenario**: E2E tests (user backup/restore, OS reinstall recovery, tunnel setup/teardown)

## License

MIT
