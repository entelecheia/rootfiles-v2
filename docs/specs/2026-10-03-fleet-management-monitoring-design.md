# Fleet management and monitoring

Status: approved for implementation by owner on 2026-10-03, tracked in #13
Implementation branch: feat/13-fleet-monitoring
Date: 2026-10-03

## Problem

rootfiles configures one host at a time. Operators who run a dozen GPU and
storage hosts drive it with shell loops (`ssh host sudo rootfiles apply --yes
--config - < site.yaml`, as the README suggests), and nothing answers the
fleet-level questions:

- Which hosts are reachable, and which rootfiles version does each run?
- Which hosts have drifted from their site config, and which have doctor
  findings (disk, failed units, GPU, SSH access)?
- Is a GPU node down, overheating, or reporting XID or ECC errors, right now?

Observed on one operator fleet (verified 2026-10-03, 10 reachable hosts):

| Item | Hosts |
|---|---|
| rootfiles v0.11.0 installed | 10 of 10 |
| `prometheus-node-exporter` active | 0 of 10 |
| `rootfiles-check.timer` active | 0 of 10 |
| Host firewall (UFW or firewalld) active | 0 of 10 |
| DCGM present | 4 of 10 (datacenter-class GPU hosts; the older workstation-class GPU hosts have none) |
| Docker present | 7 of 10 |
| An unmanaged Prometheus, Grafana and dcgm-exporter already on the default ports (9090, 9400) | 1 of 10 |

The fleet mixes Ubuntu 20.04, 22.04 and 24.04 (DGX OS included) with Rocky
Linux 8, and part of it is reachable only through a jump host. A GPU node in
that fleet stayed unreachable for more than twelve hours and was noticed only
because a service depending on it failed (verified 2026-10-03).

## Existing assets

Most of the per-host half already exists; the gap is aggregation, rollout and
GPU-level metrics.

- `status`, `check` and `doctor` print JSON (`-o json`); `check` and `doctor`
  also print Prometheus text (`-o prometheus`). `check` and `doctor` exit 2
  when they find drift or problems.
- `schedule enable` installs a systemd timer that writes `check.json` and
  `doctor.json` to the state directory and publishes `rootfiles_*` metrics to
  the node_exporter textfile directory.
- The `monitoring` module installs `prometheus-node-exporter` (opt-in).
- `apply --config -` reads a site config from stdin, so a controller can pipe
  per-host configs over SSH without copying files.
- `update` upgrades the binary in place with checksum verification.
- The `docker` and `nvidia` modules install Docker CE (with the compose
  plugin) and the NVIDIA container toolkit.
- `tunnel_token_file` established the pattern for secrets: a root-owned 0600
  file, opened with `O_NOFOLLOW`, never a command-line argument.
- The CLI builds for darwin/arm64, so read-only commands run on operator
  laptops.

## Decisions

| Decision | Owner | Limits | Revisit when |
|---|---|---|---|
| A host keeps running rootfiles as a single-host tool. Fleet features live in a controller subcommand (`rootfiles fleet`) that runs on the operator machine and drives the existing per-host commands over SSH | user | No agent or daemon is added to hosts beyond what modules already install | A fleet outgrows SSH fan-out (hundreds of hosts) |
| The controller shells out to the system `ssh` client | AI proposes, user picks | Inherits `~/.ssh/config`: `ProxyJump`, agents, `known_hosts`. The controller never disables host key checking | A platform without OpenSSH must be supported |
| The inventory is an operator-owned YAML file kept outside this repository | user | The repo ships a placeholder example and a validator only | - |
| The controller never asks for, stores or forwards sudo passwords. Remote privilege is `sudo -n` or a root login, set per host | user | A host whose sudo needs a password is reported as "needs privilege", not prompted | - |
| Mutating fleet commands run one host at a time and stop at the first failure | user | Parallelism only for read-only commands | Rollout time becomes the bottleneck |
| Monitoring uses Prometheus, Alertmanager, Grafana and NVIDIA DCGM Exporter. rootfiles installs and configures them; it does not reimplement metrics or alerting | user | Images and versions pinned in profile defaults | - |
| Rocky Linux hosts get read-only fleet reporting first; mutating rollout and exporters on Rocky wait for the capability layer in #12 | AI proposes, user picks | Unsupported host and module combinations fail before mutation with an explanation | #12 lands |

- 2026-10-03: Read-only fleet commands get passwordless privilege through a sudoers drop-in that allows only the exact `status`, `check` and `doctor` command lines, managed by the `users` module. Mutating commands keep requiring a root login or full sudo. Basis: owner decision on #13.
- 2026-10-03: Hub web UIs (Grafana, Prometheus, Alertmanager) bind to loopback by default. External access goes only through a Cloudflare Tunnel on the hub, never through an opened port. Basis: owner decision on #13.
- 2026-10-03: A perimeter firewall with an open internal segment is a supported deployment. `monitoring.perimeter_firewall: true` records it, so exporters need no host firewall rule there. Basis: owner decision on #13.
- 2026-10-03: All exporter and hub ports are configurable, keep upstream defaults, and are checked for conflicts before apply. Basis: owner decision on #13; one observed host already holds 9090 and 9400 (verified 2026-10-03).

Rejected alternatives:

- **Ansible playbooks.** A second source of truth for host configuration,
  duplicating module logic that rootfiles already tests, and with no access to
  `check`/`doctor` semantics.
- **Go SSH library (`x/crypto/ssh`).** Would have to re-implement `ProxyJump`,
  agent forwarding, `known_hosts` handling and per-host config that operators
  already maintain for OpenSSH.
- **Pull-based agent.** A new long-running privileged daemon on every host,
  with its own update and authentication story, to replace something SSH
  already provides.
- **Custom metrics collector in the controller.** Prometheus already does
  scraping, retention and alerting; `fleet status` is a point-in-time view,
  not a monitoring system.

## Design

### Inventory

```yaml
# fleet.yaml (operator-owned, not committed here)
defaults:
  sudo: nopasswd          # nopasswd | root | none
  parallel: 2
hosts:
  gpu01:
    ssh: gpu01            # ssh destination: config alias or user@host
    address: 10.0.0.11    # scrape address as seen from the monitoring hub
    groups: [gpu, dgx]
    config: sites/gpu01.yaml
  storage01:
    ssh: storage01
    address: 10.0.0.21
    groups: [storage]
    sudo: none
```

- `config` paths resolve relative to the inventory file. Each site config is
  validated with the environment-independent `config.LoadSite` before anything is sent.
- `ssh` values starting with `-` and unknown keys are rejected (strict
  decoding, like profiles).
- Selection: `--host a,b` and `--group g` (union). Mutating commands refuse to
  run without an explicit selection; `--all` must be spelled out.

### Read-only reporting (phase 1)

`rootfiles fleet status|check|doctor [selection] [-o text|json]`

- For each host, in parallel (bounded by `--parallel`), run
  `ssh -- <dest> /usr/local/bin/rootfiles <cmd> -o json`, prefixed with
  `sudo -n` when the host's `sudo` is `nopasswd`. The absolute path matches
  the sudoers rule below and does not depend on the remote `secure_path`.
- `fleet status` also works where rootfiles is missing: it falls back to a
  fixed probe (hostname, `/etc/os-release`, uptime, `rootfiles --version`),
  so a host that still needs bootstrapping shows up as such.
- When an inventory supplies a site config, the controller compares its resolved,
  environment-independent canonical fingerprint with `applied_config_sha256` in
  the fixed read-only JSON response. A mismatch reports configuration drift; a
  missing fingerprint reports unverified configuration rather than success. Provenance requires
  a successful apply record carrying the digest; legacy and failed records do not count.
  The controller cannot pass arbitrary `--config` paths through the sudo rule.
- Each host lands in one of: `ok`, `drift` (exit 2 from `check`), `findings`
  (exit 2 from `doctor`), `needs-privilege` (sudo required a password),
  `unreachable` (ssh exit 255 or timeout), `error`.
- Text output is one table row per host with the state and a short reason;
  `-o json` emits the per-host results with the remote JSON embedded verbatim.
- Exit code: 0 when every selected host is `ok`, 2 when any host is in another
  state, 1 for controller or inventory errors. This matches `check`.

### Read-only privilege (phase 1)

```yaml
users:
  fleet_sudo_users: [ops]
```

- The `users` module writes `/etc/sudoers.d/rootfiles-fleet` with one
  `Cmnd_Alias` that lists exactly
  `/usr/local/bin/rootfiles status -o json`,
  `/usr/local/bin/rootfiles check -o json` and
  `/usr/local/bin/rootfiles doctor -o json`, and grants it `NOPASSWD` to the
  listed users. No wildcards: `--config` or other arguments are not allowed,
  so the rule cannot be used to read arbitrary files as root.
- The drop-in goes through the existing `writeSudoers` path (temporary name,
  `visudo -cf`, then rename).
- Check refuses to write the rule when the binary is not root-owned or is
  group- or world-writable, because a writable binary behind a `NOPASSWD`
  rule is a root shell.
- sudo applies the last matching rule, and both Ubuntu and Rocky read
  `/etc/sudoers.d` after the group rules, so the `NOPASSWD` entry wins for the
  three commands only. Apply verifies the effect with
  `sudo -l -U <user>` instead of trusting the file contents.
- An empty list removes the drop-in.

### Rollout (phase 2)

`rootfiles fleet apply|update|bootstrap|schedule [selection] --yes [--dry-run]`

- `apply` resolves profile and file-based inheritance locally without operator
  environment overrides, then sends the effective configuration on stdin. A real
  rollout stages it atomically at a root-owned content-hash configuration path before invoking
  `rootfiles apply --yes --config <managed-path>`, so later check/status/doctor
  use the applied configuration. Dry-run uses stdin without persisting a file.
  Configs with an inline `tunnel_token` are refused; use `tunnel_token_file`.
- `update` runs the remote `rootfiles update` (optionally `--version`).
- `bootstrap` installs rootfiles on a host that lacks it, through the release
  installer with checksum verification and a pinned version.
- `schedule` runs `schedule enable|disable`.
- Hosts run serially in inventory order. The first failure stops the run and
  the remaining hosts are reported as skipped. `--dry-run` is passed through.
- Each run is appended to a local JSON Lines log under the controller's state
  directory (time, operator, command, hosts, per-host result). Hosts keep
  their own audit log as today.

### Host exporters (phase 3)

Extend the `monitoring` module:

```yaml
modules:
  monitoring:
    node_exporter: true
    dcgm_exporter: true            # GPU hosts only
    node_exporter_port: 9100       # default
    dcgm_exporter_port: 9400       # default
    listen_address: 10.0.0.11      # empty = all interfaces
    allow_from: [10.0.0.5/32]      # monitoring hub
    perimeter_firewall: false      # true: a network firewall already blocks outside access
```

- `dcgm_exporter` runs NVIDIA's dcgm-exporter container through Docker and the
  NVIDIA runtime under a managed systemd unit, with a pinned image tag. It
  requires installed Docker, an NVIDIA container runtime and GPU access; Check
  inspects those dependencies without requiring their installation-module flags
  or changing a vendor stack. A managed collectors file enables the supported
  counters used by the GPU rules, including ECC when available on the GPU. Whether a distro package is preferable
  on DGX OS is decided during implementation (inferred, not yet checked).
- The image tag must support the host's driver branch. The oldest branch in
  the observed fleet is R535 (verified 2026-10-03); compatibility is checked
  per branch during implementation.
- `listen_address` binds node_exporter and dcgm-exporter to one address.
- Check fails, before any change, when a configured port is already held by a
  process rootfiles does not manage, and names the holder when it can (for
  example an existing exporter container). The operator picks another port or
  retires the other service.
- `allow_from` adds source-restricted firewall rules for the exporter ports
  when the host firewall is active. rootfiles does not turn a firewall on as a
  side effect; that stays with the `network` module.
- `doctor` warns when an exporter listens on a non-private address while
  neither a host firewall rule nor `perimeter_firewall: true` covers it.

### Monitoring hub (phase 4)

A new opt-in part of the `monitoring` module, enabled on one host:

```yaml
modules:
  monitoring:
    hub:
      enabled: true
      data_dir: /data/monitoring
      retention: 30d
      targets_file: /etc/rootfiles/monitoring/discovery/targets.json
      alert_receiver_file: /etc/rootfiles/monitoring/receiver.yaml   # root-only 0600
      listen_address: 127.0.0.1    # default; UIs reachable only on the hub itself
      grafana_port: 3000
      prometheus_port: 9090
      alertmanager_port: 9093
```

- Prometheus, Alertmanager and Grafana run as containers from one generated
  compose file under one systemd unit, with pinned images and bind-mounted data
  under `data_dir`.
- The UIs publish on `listen_address` (loopback by default). Prometheus and
  Alertmanager talk over the compose network, and scraping is outbound, so the
  loopback binding does not affect collection. The same port-conflict check
  as the exporters applies.
- External access goes through a Cloudflare Tunnel on the hub (the existing
  `cloudflared` module with `tunnel_token_file`). The public hostname for
  Grafana and its access policy are set on the Cloudflare side; rootfiles only
  documents the expected origin (`http://127.0.0.1:<grafana_port>`).
- Scrape targets come from Prometheus `file_sd`. `rootfiles fleet targets`
  renders them from the inventory (`address`, groups as labels, exporter ports
  from each host's site config), and `rootfiles fleet targets --push <hub>`
  writes them atomically to an isolated discovery directory on the hub through
  the same SSH path. Mount the directory, rather than one file, so atomic file
  replacements are visible inside the container without exposing secret files. Prometheus reloads
  `file_sd` without a restart.
- The alerting rules are embedded, versioned with the binary, and kept small:
  - Target down for 5 minutes.
  - Filesystem above 90 percent.
  - Drift and doctor failures (`rootfiles_module_satisfied`,
    `rootfiles_doctor_findings{level="fail"}`), plus a stale-report rule on
    `rootfiles_check_timestamp_seconds`, from the textfile metrics the
    `schedule` timer already publishes.
  - GPU XID errors, uncorrectable ECC errors, and sustained temperature above
    threshold, from dcgm-exporter.
- Alertmanager receiver settings (for example a chat webhook or bot token)
  are read from `alert_receiver_file`, using the same file checks as
  `tunnel_token_file`. They never appear in a site config or on a command line.
- Grafana provisioning sets the Prometheus datasource only. Dashboards are
  imported by the operator; shipping dashboard JSON is out of scope.

### Security

- No new listening service on managed hosts except the exporters, and those
  bind to `listen_address` when set. Hub UIs bind to loopback unless the
  operator sets another address, and are published externally only through
  Cloudflare.
- The only passwordless privilege the feature adds is the read-only sudoers
  rule, limited to three exact command lines.
- OpenSSH hands the remote command to the remote shell as one string, so the
  remote command line holds only fixed tokens and validated values (for
  example a version matching `vX.Y.Z`), each shell-quoted. Site configs travel
  on stdin. Locally the destination follows `--`, so an inventory value cannot
  become an ssh option.
- The controller relies on OpenSSH host key verification and does not offer a
  flag to disable it.
- Secrets travel only as files on the target host (`tunnel_token_file`,
  `alert_receiver_file`). The controller refuses inline secrets in piped
  configs.

## Phases

| Phase | Sub-issue scope | Depends on |
|---|---|---|
| 1 | Inventory, `fleet status/check/doctor`, read-only sudoers rule | - |
| 2 | `fleet apply/update/bootstrap/schedule` | 1 |
| 3 | `dcgm_exporter`, ports and conflict check, `listen_address`, `allow_from`, `perimeter_firewall`, doctor exposure check | - (Rocky: #12) |
| 4 | Monitoring hub (loopback UIs, Cloudflare origin), `fleet targets`, embedded alert rules | 1, 3 |

Phases 1 and 3 can proceed in parallel.

## Testing

- Unit: inventory decoding and validation, selection, state classification
  from exit codes, target rendering, compose and rule rendering. The SSH client
  is a fake `ssh` on `PATH`, following the existing `fakeBin` pattern; tests
  never open a network connection.
- Scenario (`tests/scenarios/`): a controller run against two local containers
  reachable over SSH, covering `ok`, `drift`, `unreachable` and
  stop-on-first-failure.
- Exporters and hub: Check/Apply agreement and idempotency (a second apply
  reports no changes), with `systemctl` and `docker` stubbed.

## Out of scope

- GPU scheduling, user portals, SSO and reservations. These are separate
  layers with their own tooling.
- Log aggregation (Loki or similar).
- Bare-metal provisioning and OS installation.
- Shipping Grafana dashboards.
- Rocky-specific module behavior, which belongs to #12.
