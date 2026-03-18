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

Latest stable release (recommended):

```bash
curl -fsSL https://raw.githubusercontent.com/entelecheia/rootfiles-v2/main/scripts/install.sh | sudo bash
```

Specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/entelecheia/rootfiles-v2/main/scripts/install.sh | sudo bash -s -- --version v0.1.0
```

Dev channel (build from source, requires Go):

```bash
curl -fsSL https://raw.githubusercontent.com/entelecheia/rootfiles-v2/main/scripts/install.sh | sudo bash -s -- --channel dev
```

The installer downloads a prebuilt binary, verifies its SHA256 checksum, and places it at `/usr/local/bin/rootfiles`.

## Quick start

```bash
sudo rootfiles apply
```

```bash
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

Interactive — prompts for profile, then walks through each setting (SSH, Users, Docker, Cloudflared, Network, Storage):

```bash
sudo rootfiles apply
```

```bash
sudo rootfiles apply --profile dgx
```

Specific modules only:

```bash
sudo rootfiles apply --module cloudflared,docker
```

Dry-run (preview changes, no execution):

```bash
sudo rootfiles apply --profile dgx --dry-run
```

From a backup snapshot:

```bash
sudo rootfiles apply --config /raid/backup/rootfiles-backup-*/config-snapshot.yaml
```

Unattended (CI/automation — skips all interactive prompts):

```bash
sudo rootfiles apply --profile dgx --yes --home-base /raid/home --tunnel-token "$CF_TUNNEL_TOKEN" --vlan-address "172.16.229.32/32" --user yjlee --ssh-pubkey "ssh-ed25519 AAAA..."
```

### Check system state

```bash
sudo rootfiles check --profile dgx
```

```bash
sudo rootfiles check --config /raid/backup/rootfiles-backup-*/config-snapshot.yaml
```

### System backup (for OS upgrade)

Captures system info, users, config files, Docker images, and a rootfiles-compatible config snapshot.

```bash
sudo rootfiles backup
```

```bash
sudo rootfiles backup -o /raid/backup
```

```bash
sudo rootfiles backup --skip-docker
```

```bash
sudo rootfiles backup --skip-etc
```

Backup output directory structure:

```
rootfiles-backup-{hostname}-{YYYYMMDD}/
├── system-info.json        # hostname, OS, GPU, arch, memory, mounts
├── users.json              # user metadata (from rootfiles DB)
├── etc-config.tar.gz       # /etc/ssh, docker, ufw, netplan, fstab
├── crontab-root.txt        # root crontab
├── root-ssh.tar.gz         # /root/.ssh/
├── usr-local-bin.tar.gz    # /usr/local/bin/
├── docker-images.txt       # docker image list
└── config-snapshot.yaml    # current system → rootfiles YAML config
```

Restore from snapshot:

```bash
sudo rootfiles apply --config /raid/backup/rootfiles-backup-*/config-snapshot.yaml --dry-run
```

```bash
sudo rootfiles apply --config /raid/backup/rootfiles-backup-*/config-snapshot.yaml --yes
```

### User management

Users are created at a custom home base (e.g., `/raid/home/`) that survives OS reinstalls.

```bash
sudo rootfiles user add yjlee --pubkey "ssh-ed25519 AAAA..."
```

```bash
sudo rootfiles user list
```

```bash
sudo rootfiles user list --names
```

```bash
sudo rootfiles user backup
```

```bash
sudo rootfiles user restore
```

```bash
sudo rootfiles user rehome yjlee
```

### Cloudflare tunnel + VLAN

```bash
sudo rootfiles tunnel setup "$TOKEN" --vlan-address "172.16.229.32/32"
```

```bash
sudo rootfiles tunnel status
```

```bash
sudo rootfiles tunnel update
```

```bash
sudo rootfiles tunnel restart
```

```bash
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
make build
```

```bash
make test
```

Requires Go 1.23+.

## Architecture

```
cmd/rootfiles/        Entry point
internal/
  cli/                Cobra commands (apply, backup, check, tunnel, user)
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
