# Rocky, fleet and monitoring verification

Scope: #12, #13 and #15-#18. The owner authorized implementation and deployment on 2026-10-03. Internal inventories, access identities and secret values remain outside this repository.

## Implementation order

1. Add native Rocky distro/package capabilities and fail before unsupported host changes.
2. Add read-only inventory reporting with fixed remote commands and applied provenance.
3. Add explicit-selection serial rollouts, bounded timeouts and controller audit records.
4. Add exporter bindings, dependency/conflict checks and supported GPU collectors.
5. Add the monitoring hub, isolated file discovery, root-only secrets and alert rules.
6. Verify, review the exact head and pass CI before merge and release.
7. Deploy a canary, verify live endpoints and unchanged workloads, then roll out serially.

## Required verification

| Contract | Evidence |
|---|---|
| Ubuntu/DGX compatibility | Existing integration, module and scenario workflow matrix |
| Rocky native package/locale/security behavior | Rocky 8.9, 8.10 and maintained 9 systemd fixtures, APT/dpkg/locale-gen traps |
| Existing identity and SSH access preservation | Password/home/shell/groups/key assertions and real loopback sshd logins |
| Security-only updates | Effective DNF configuration, bound BaseOS/AppStream advisory metadata, GPU exclusions, no reboot, active timer |
| Check purity and idempotency | Cache-only queries, repeated apply/check, fake commands and backed-up files |
| Fleet reporting and stop behavior | Fake-SSH unit tests and two real SSH containers with verified host keys |
| Read-only sudo boundary | Exact-command rendering and actual allowed/refused sudo operations |
| Operator support | Darwin controller tests/build and Linux cross-builds |
| Exporters | Pre-mutation port ownership checks, collector syntax, real GPU metrics after release |
| Hub | Generated Compose/config validation, readiness, targets and per-host rule evaluation |
| Secrets and remote access | Root-owned 0600 files, loopback web ports, owner-only Access and DM receiver readback |

Rocky fixtures on Ubuntu CI do not establish enforcement under a SELinux-enabled kernel. SELinux command and refusal paths have unit coverage; production non-default ports require the existing firewall and labeling prerequisites.

## Deployment gates

- Publish and download a checksummed release, then verify binary hashes on every reachable host.
- Keep prior binaries, site configuration, state and service configuration for recovery.
- Preserve GPU drivers, running application containers and existing web ports.
- Validate the new monitoring hub before retiring any existing monitoring components.
- Preserve prior monitoring volumes and images; no data deletion, storage migration or pruning is part of this rollout.
- Report unavailable hosts and unsupported module coverage explicitly. An unreachable host is not a successful deployment.
