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
Security baseline, quotas, health
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

The installer downloads a prebuilt binary, verifies its SHA256 checksum, and places it at `/usr/local/bin/rootfiles`. A `root` symlink is also created in the same directory — every example below works with either name:

```bash
sudo rootfiles apply    # long form
sudo root apply         # same thing, shorter
```

## Quick start

```bash
sudo rootfiles apply
```

```bash
sudo rootfiles apply --profile dgx --yes
```

In interactive mode, `apply` presents each configurable setting (SSH, firewall, VLAN, storage, etc.) for review, pre-filled with the profile's values, and flags any choice that is weaker than the profile before asking to apply. Use `--yes` to skip all prompts for CI/automation.

### Safety

`apply` is designed to be safe to re-run on a live server:

- **No SSH lockout** — password auth is only disabled when at least one account can log in with a key (declared accounts count); enabling UFW always admits the SSH port; `sshd -t` validates the config and a failing change is reverted; a port change on socket-activated Ubuntu restarts `ssh.socket`. `--force` overrides the key check.
- **No data loss** — existing directories are never `rm -rf`'d (a populated path blocks a symlink instead), `daemon.json` is merged key by key, `user rehome` keeps the old home as a backup.
- **Backups & rollback** — every file a run overwrites or removes is saved under `/var/lib/rootfiles/backups/<id>`; `rootfiles rollback <id>` restores it.
- **One run at a time** — mutating commands need root and take `/run/rootfiles.lock`; Ctrl-C cancels in-flight commands.
- **Audit trail** — real runs are logged as JSON to `/var/log/rootfiles.log`; the last result is kept in `/var/lib/rootfiles/state.json` (shown by `status`).

## Profiles

| Profile | Extends | Use case |
|---------|---------|----------|
| `base` | — | Locale, packages, SSH |
| `minimal` | base | + users, cloudflared, ufw, security baseline, journald cap |
| `dgx` | minimal | + Docker, NVIDIA toolkit, persistenced, fabric manager, VLAN, RAID storage |
| `gpu-server` | minimal | + Docker, NVIDIA toolkit, persistenced (non-DGX) |
| `full` | minimal | + Docker, storage, network |

DGX OS is auto-detected (`/etc/dgx-release`) and the appropriate profile is suggested.

### Site configs

Keep host- or site-specific settings in a small file that extends a profile. Any key it sets wins — including `false`, so a site config can switch a module off:

```yaml
# site.yaml
extends: dgx            # a built-in profile, or a path relative to this file
timezone: UTC
users:
  accounts:             # created if missing; missing keys/groups added; never removed
    - name: admin
      ssh_pubkeys: ["ssh-ed25519 AAAA... admin@laptop"]
      groups: [sudo]
modules:
  cloudflared:
    enabled: false
  system:
    hostname: gpu01
    apt_mirror: http://mirror.kakao.com/ubuntu
```

```bash
rootfiles config init --extends dgx --file site.yaml   # scaffold (or --from-system to capture this host)
rootfiles config validate --config site.yaml           # unknown keys, paths, ports, CIDRs …
rootfiles config show --config site.yaml               # fully merged result (token masked)
sudo rootfiles apply --config site.yaml
```

`--config -` reads the config from stdin, which is handy for fleets: `ssh gpu01 sudo rootfiles apply --yes --config - < site.yaml` (rootfiles itself stays single-host; drive many hosts from Ansible or a shell loop).

## Modules

All modules are idempotent and support `--dry-run`.

| Module | Description |
|--------|-------------|
| `locale` | Locale generation, timezone (installs tzdata when missing) |
| `system` | Hostname, swapfile (only when no swap), sysctl, journald cap, Ubuntu APT mirror |
| `packages` | APT package installation (non-interactive, waits for the dpkg lock) |
| `users` | Custom home base, declared `users.accounts` (keys, groups), backup/restore |
| `ssh` | sshd hardening (root login, password + keyboard-interactive auth, port, MaxAuthTries) with lockout guard |
| `security` | Security-only unattended upgrades (no reboot, NVIDIA/CUDA excluded), fail2ban sshd jail, NTP |
| `docker` | Docker CE + daemon.json merge + storage relocation |
| `nvidia` | NVIDIA Container Toolkit, Docker runtime, nvidia-persistenced / fabric manager |
| `gpu` | Per-user GPU allocation (env vars, cgroups) |
| `cloudflared` | Cloudflare Tunnel service (token in a root-only env file) + VLAN private network |
| `storage` | RAID/NVMe directory setup, symlinks |
| `network` | UFW firewall, port rules (SSH port always allowed) |
| `monitoring` | Prometheus node exporter (opt-in) |

Modules run in this order; `users` precedes `ssh` so declared operators exist before password auth is turned off.

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

Unattended (CI/automation — skips all interactive prompts). `--user`/`--ssh-pubkey` declare an account that is created before SSH is hardened, and `--tunnel-token` installs the tunnel service:

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

`check` exits **0** when everything is satisfied, **2** when changes are pending and **1** on errors. Machine-readable output:

```bash
rootfiles check -o json          # modules, satisfied, changes[{description, command}]
rootfiles check -o prometheus    # rootfiles_module_satisfied{module="…"} …
```

### Health check

`doctor` inspects the live system rather than the profile: SSH access (keys vs. `sshd -T`), firewall vs. SSH port, reboot-required, disk usage, NTP, GPU driver / persistence mode, failed systemd units, and the last apply. Exit code 2 on failures (`--strict`: also on warnings); `-o json|prometheus` supported.

```bash
rootfiles doctor
```

Run `check` and `doctor` on a schedule (reports go to `/var/lib/rootfiles/{check,doctor}.json`, and to the node exporter textfile directory when the `monitoring` module is enabled):

```bash
sudo rootfiles schedule enable --on-calendar daily
sudo rootfiles schedule disable
```

### Rollback

```bash
rootfiles rollback                     # list backup sessions
sudo rootfiles rollback 20260926-101500 --dry-run
sudo rootfiles rollback 20260926-101500
```

Rollback restores files only (configs, units, symlinks); packages, accounts and running services are not reverted — restart affected services afterwards.

### Status dashboard

Unified at-a-glance view — system info, active profile, module satisfaction, GPU allocations, tunnel service, and managed users in one pass:

```bash
rootfiles status
```

Without flags, `status` and `check` evaluate against the profile/config last applied on this host (falling back to detection). `-o json` is available. Evaluate against a specific profile without applying anything:

```bash
rootfiles status --profile dgx
```

Output uses terminal colour via lipgloss. Colour is stripped automatically for non-TTY output (`rootfiles status | less`, CI logs) and when `NO_COLOR=1` is set.

### Self-update

Update to the latest GitHub release:

```bash
sudo rootfiles update
```

Check for updates without installing:

```bash
rootfiles update --check
```

Pin a specific version:

```bash
sudo rootfiles update --version v0.9.0
```

Preview the upgrade plan without replacing the binary:

```bash
sudo rootfiles update --dry-run
```

`update` is the canonical spelling; `rootfiles upgrade` works as an alias for muscle-memory compatibility.

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

After an OS reinstall, restore accounts first (their SSH keys are what allow the SSH hardening to proceed), then re-apply:

```bash
sudo rootfiles user restore
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

List system users (UID 1000-65533):

```bash
sudo rootfiles user list --system
```

```bash
sudo rootfiles user list --system --names
```

Show UID/GID/groups for a user:

```bash
sudo rootfiles user id yjlee
```

List all groups or groups for a specific user:

```bash
sudo rootfiles user groups
```

```bash
sudo rootfiles user groups yjlee
```

Add/remove a user from groups:

```bash
sudo rootfiles user group-add yjlee --docker --sudo
```

```bash
sudo rootfiles user group-add yjlee --groups dev,ops
```

```bash
sudo rootfiles user group-del yjlee --docker
```

Set passwords in batch. By default a random password is generated per user and printed once; `--suffix` keeps the legacy `username + suffix` scheme and `--expire` forces a change at next login. Passwords are passed to `chpasswd` on stdin, never on a command line:

```bash
sudo rootfiles user passwd alice bob --expire
```

```bash
sudo rootfiles user passwd alice bob --suffix '!@'
```

```bash
sudo rootfiles user passwd --all --dry-run
```

```bash
sudo rootfiles user passwd --file users.txt
```

```bash
sudo rootfiles user passwd alice bob --password 'shared-pass'
```

```bash
sudo rootfiles user backup
```

```bash
sudo rootfiles user restore
```

```bash
sudo rootfiles user rehome yjlee              # verified copy; old home kept as <old>.rootfiles-bak-<ts>
sudo rootfiles user rehome yjlee --remove-old
```

Lifecycle:

```bash
sudo rootfiles user key add yjlee "ssh-ed25519 AAAA... yjlee@laptop"
sudo rootfiles user key list yjlee
sudo rootfiles user key rm yjlee yjlee@laptop     # by comment, key, or list index
sudo rootfiles user lock yjlee                    # blocks password *and* SSH key logins
sudo rootfiles user unlock yjlee
sudo rootfiles user expire yjlee 2026-12-31       # or: never
sudo rootfiles user del yjlee --archive           # tar.gz home under <home_base>/.rootfiles/archive
sudo rootfiles user du                            # home usage, largest first
sudo rootfiles user audit                         # users.json vs. system accounts (exit 2 on drift)
```

Disk quotas on the home_base filesystem (XFS project quota with `prjquota`, or ext4 user quota with `usrquota` + `quotaon`; the limit is re-applied by `user restore`):

```bash
sudo rootfiles user quota set yjlee 500G
sudo rootfiles user quota show
sudo rootfiles user quota rm yjlee
```

### GPU allocation

Assign GPUs to individual users to prevent resource contention on shared GPU servers.

```bash
sudo rootfiles gpu assign alice --gpus 0,1,2,3 --method env
```

```bash
sudo rootfiles gpu assign bob --gpus 4,5,6,7 --method cgroup
```

```bash
sudo rootfiles gpu list
```

```bash
sudo rootfiles gpu status
```

```bash
sudo rootfiles gpu revoke alice
```

Methods:

| Method | Mechanism | Scope |
|--------|-----------|-------|
| `env` | Sets `CUDA_VISIBLE_DEVICES` / `NVIDIA_VISIBLE_DEVICES` via `/etc/profile.d/` script | Login shells |
| `cgroup` | systemd slice with `DeviceAllow` rules | All processes in user session |
| `both` | env + cgroup combined | Full isolation |

The default method is configured per profile (`env` for gpu-server, `both` for dgx).

MIG mode and instances (read-only; repartition with `nvidia-smi mig`):

```bash
rootfiles gpu mig status
```

### Cloudflare tunnel + VLAN

```bash
sudo rootfiles tunnel setup "$TOKEN" --vlan-address "172.16.229.32/32"
```

```bash
sudo rootfiles tunnel status
```

Upgrade just the `cloudflared` binary to the latest upstream release (no other modules touched). The service is restarted when present; binary-only refreshes on hosts without the tunnel skip the restart:

```bash
sudo rootfiles tunnel update                     # fetch latest
sudo rootfiles tunnel update --check             # compare installed vs. latest, no download
sudo rootfiles tunnel update --version 2024.9.1  # pin to a specific cloudflared release
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
| `ROOTFILES_STATE_DIR` | State, history and file backups | `/var/lib/rootfiles` |
| `ROOTFILES_LOG_FILE` | JSON audit log | `/var/log/rootfiles.log` |
| `ROOTFILES_LOCK_FILE` | Global lock | `/run/rootfiles.lock` |

## Build from source

```bash
make build
```

```bash
make test
```

Requires Go 1.23+.

## Architecture

Interactive blueprint: [docs/architecture/rootfiles-v2-rendered.html](docs/architecture/rootfiles-v2-rendered.html) (spec: [rootfiles-v2.architecture.json](docs/architecture/rootfiles-v2.architecture.json)).

```
cmd/rootfiles/        Entry point
internal/
  cli/                Cobra commands (apply, backup, check, config, doctor, gpu, rollback,
                      schedule, status, tunnel, update, user) + root/lock preflight
  config/             YAML profiles (tree-merged extends), validation, system detector
    profiles/         Embedded profile YAMLs (go:embed)
  module/             13 modules implementing the Module interface, doctor checks
  exec/               Shell runner (dry-run aware, per-run file backups), APT wrapper
  state/              Applied-state record, history, global lock
  ui/                 Interactive prompts (Charm huh) + shared output styling
                      (lipgloss palette, ✓ ✗ → ⚠ markers, WriteHeader/
                      Section/KV/Hint/Bullet helpers)
```

`NewRegistry()` and `defaultOrder` in `internal/module/module.go` are a two-part module contract enforced by `TestRegistryDefaultOrderSync`. The GPU allocation database (`<home-base>/.rootfiles/gpu-allocations.json`) is read-modify-written under `syscall.Flock` with atomic tmp-and-rename writes, so concurrent `gpu assign` / `revoke` calls cannot lose an allocation.

## CI

Every push and pull request runs:

| Job | Purpose |
|-----|---------|
| `lint` | `gofmt`, `go vet`, `go mod tidy` drift |
| `vuln` | `govulncheck ./...` (stdlib + deps) |
| `unit` | `go test ./... -race -count=1` + per-function coverage summary, coverage artifact |
| `integration` | 3 OS images × 4 profiles (11 combinations) |
| `module` | 2 OS × 9 modules + GPU on DGX mock (isolated module execution) |
| `scenario` | 14 E2E flows: dry-run-all-profiles, user backup/restore, user rehome, user list names, user lifecycle, user quota, declarative accounts, tunnel setup/teardown, OS reinstall recovery, system backup, GPU allocation, status, SSH lockout guard, state/rollback/lock |
| `release` | GoReleaser on every `v*` tag (triggered automatically when a version tag is pushed) |

## License

MIT
