#!/usr/bin/env bash
# Run the opt-in real Prometheus/Alertmanager/Grafana acceptance scenario.
# First generate fixtures with:
#   ROOTFILES_MONITORING_SCENARIO_DIR="$PWD/hub-fixtures" \
#     go test ./internal/module -run '^TestWriteMonitoringScenarioFixtures$' -count=1
# Then run: tests/scenarios/monitoring-hub.sh hub-fixtures
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
CONTROLLER_BIN="${ROOTFILES_TEST_BINARY:-$REPO_ROOT/rootfiles}"
if [[ ! -x "$CONTROLLER_BIN" ]]; then
    echo "ROOTFILES_TEST_BINARY must name an executable rootfiles controller binary" >&2
    exit 2
fi
FIXTURE_ARG="${1:-hub-fixtures}"
if [[ "$FIXTURE_ARG" = /* ]]; then
    FIXTURE_ROOT="$(cd "$FIXTURE_ARG" && pwd -P)"
else
    FIXTURE_ROOT="$(cd "$PWD/$FIXTURE_ARG" && pwd -P)"
fi
if [[ "$FIXTURE_ROOT" == "/" || "$FIXTURE_ROOT" == "$REPO_ROOT" ]]; then
    echo "refusing to run scenario against a non-disposable fixture root: $FIXTURE_ROOT" >&2
    exit 2
fi
CONFIG_DIR="$FIXTURE_ROOT/etc/rootfiles/monitoring"
COMPOSE="$CONFIG_DIR/compose.yaml"
PROM_CONFIG="$CONFIG_DIR/prometheus.yaml"
ALERT_RULES="$CONFIG_DIR/alert-rules.yaml"
RECEIVER="$CONFIG_DIR/receiver.yaml"
TARGETS="$CONFIG_DIR/discovery/targets.json"
DATA_DIR="$FIXTURE_ROOT/var/lib/rootfiles/monitoring"

for command in docker curl python3 ssh ssh-keygen; do
    command -v "$command" >/dev/null 2>&1 || { echo "required command not found: $command" >&2; exit 2; }
done
docker compose version >/dev/null
for file in "$COMPOSE" "$PROM_CONFIG" "$ALERT_RULES" "$TARGETS" \
    "$RECEIVER" "$CONFIG_DIR/grafana-admin-password" "$CONFIG_DIR/telegram-bot-token" \
    "$FIXTURE_ROOT/fixture-root.txt" "$FIXTURE_ROOT/dcgm-image.txt"; do
    [[ -f "$file" ]] || { echo "missing generated fixture file: $file" >&2; exit 2; }
    [[ ! -L "$file" ]] || { echo "fixture files must not be symlinks: $file" >&2; exit 2; }
done
[[ ! -L "$DATA_DIR" ]] || { echo "fixture data directory must not be a symlink" >&2; exit 2; }
grep -Fxq 'scenario-only-grafana-password' "$CONFIG_DIR/grafana-admin-password" || {
    echo "refusing a non-scenario Grafana password file" >&2
    exit 2
}
grep -Fxq 'scenario-only-not-a-telegram-token' "$CONFIG_DIR/telegram-bot-token" || {
    echo "refusing a non-scenario Telegram token file" >&2
    exit 2
}

EMBEDDED_ROOT="$(cat "$FIXTURE_ROOT/fixture-root.txt")"
if [[ -z "$EMBEDDED_ROOT" || "$EMBEDDED_ROOT" != /* ]]; then
    echo "fixture-root.txt must contain the original absolute fixture directory" >&2
    exit 2
fi
# Artifacts may be downloaded into another workspace. Relocate only source
# path prefixes in generated bind mounts; preserve all other compose settings.
if [[ "$EMBEDDED_ROOT" != "$FIXTURE_ROOT" ]]; then
    OLD_ROOT="$EMBEDDED_ROOT" NEW_ROOT="$FIXTURE_ROOT" COMPOSE_FILE="$COMPOSE" ROOT_MARKER="$FIXTURE_ROOT/fixture-root.txt" python3 -c 'import os,pathlib; p=pathlib.Path(os.environ["COMPOSE_FILE"]); old=os.environ["OLD_ROOT"]; new=os.environ["NEW_ROOT"]; s=p.read_text(); (p.write_text(s.replace(old,new)) if old in s else (_ for _ in ()).throw(SystemExit("generated compose file lacks its fixture-root prefix"))); pathlib.Path(os.environ["ROOT_MARKER"]).write_text(new+"\n")'
fi

# This fixture receiver is inert and never reads a real Telegram secret.
cat >"$RECEIVER" <<'YAML'
route:
  receiver: fixture-noop
receivers:
  - name: fixture-noop
YAML
if grep -Eiq 'telegram|chat_id|bot_token' "$RECEIVER"; then
    echo "scenario Alertmanager receiver must remain no-op and must not contact Telegram" >&2
    exit 2
fi
chmod 0600 "$RECEIVER" "$CONFIG_DIR/grafana-admin-password" "$CONFIG_DIR/telegram-bot-token"
chmod 0644 "$TARGETS"
if ! grep -Fq 'RootfilesMonitoringScenarioAlwaysFires' "$ALERT_RULES"; then
cat >>"$ALERT_RULES" <<'YAML'
  - name: rootfiles-monitoring-acceptance
    rules:
      - alert: RootfilesMonitoringScenarioAlwaysFires
        expr: vector(1)
        for: 0s
        labels:
          severity: scenario
        annotations:
          summary: Generated monitoring hub acceptance rule
YAML
fi

PROJECT="rootfiles-monitoring-scenario-$$-${RANDOM}"
SSH_IMAGE="rootfiles-monitoring-target-push:scenario-$$-${RANDOM}"
SSH_CONTAINER="rootfiles-monitoring-target-push-hub-$$-${RANDOM}"
WORK_TMP="$(mktemp -d "$FIXTURE_ROOT/scenario-work.XXXXXX")"
COMPOSE_CMD=(docker compose --project-name "$PROJECT" --file "$COMPOSE")
cleanup() {
    local status=$?
    if [[ "$status" -ne 0 ]]; then
        "${COMPOSE_CMD[@]}" logs --no-color >&2 2>/dev/null || true
    fi
    "${COMPOSE_CMD[@]}" down --remove-orphans >/dev/null 2>&1 || true
    docker rm -f "$SSH_CONTAINER" >/dev/null 2>&1 || true
    docker image rm "$SSH_IMAGE" >/dev/null 2>&1 || true
    rm -rf "$WORK_TMP"
    return "$status"
}
trap cleanup EXIT

# The production default must name a published NVIDIA image. This metadata
# check avoids pulling GPU layers and catches a nonexistent pinned tag.
DCGM_IMAGE=$(cat "$FIXTURE_ROOT/dcgm-image.txt")
[[ "$DCGM_IMAGE" == nvcr.io/nvidia/k8s/dcgm-exporter:* && "$DCGM_IMAGE" != *[[:space:]]* ]] || {
    echo "invalid production default DCGM image reference" >&2
    exit 2
}
docker buildx imagetools inspect --raw "$DCGM_IMAGE" >"$WORK_TMP/dcgm-manifest.json"
echo "PASS: production default DCGM image is published in the NVIDIA registry"

# Rendered hub services run as container root with capabilities dropped. Set
# only disposable hub data/config files to root ownership for reads and writes.
mkdir -p "$DATA_DIR/prometheus" "$DATA_DIR/alertmanager" "$DATA_DIR/grafana"
docker run --rm \
    --volume "$DATA_DIR:/scenario-data" \
    --volume "$CONFIG_DIR/discovery:/scenario-discovery" \
    --volume "$RECEIVER:/receiver" \
    --volume "$CONFIG_DIR/grafana-admin-password:/grafana-password" \
    --volume "$CONFIG_DIR/telegram-bot-token:/telegram-token" \
    busybox:1.36.1 sh -c '
      chown -R 0:0 /scenario-data /scenario-discovery
      chown 0:0 /receiver /grafana-password /telegram-token
      chmod 0755 /scenario-data /scenario-data/prometheus /scenario-data/alertmanager /scenario-data/grafana /scenario-discovery
      chmod 0600 /receiver /grafana-password /telegram-token
      chmod 0644 /scenario-discovery/targets.json
    '

"${COMPOSE_CMD[@]}" config --quiet
"${COMPOSE_CMD[@]}" pull
"${COMPOSE_CMD[@]}" run --rm --no-deps --entrypoint promtool prometheus \
    check config /etc/prometheus/prometheus.yaml
"${COMPOSE_CMD[@]}" run --rm --no-deps --entrypoint promtool prometheus \
    check rules /etc/prometheus/alert-rules.yaml
cat >"$WORK_TMP/fleet-targetdown.test.yml" <<'YAML'
rule_files:
  - /etc/prometheus/alert-rules.yaml
evaluation_interval: 1m
tests:
  - interval: 1m
    input_series:
      - series: 'up{job="fleet",host="scenario-down"}'
        values: '0+0x5'
    alert_rule_test:
      - eval_time: 5m
        alertname: FleetTargetDown
        exp_alerts:
          - exp_labels:
              alertname: FleetTargetDown
              host: scenario-down
              job: fleet
              severity: critical
YAML
"${COMPOSE_CMD[@]}" run --rm --no-deps \
    --volume "$WORK_TMP/fleet-targetdown.test.yml:/tmp/fleet-targetdown.test.yml:ro" \
    --entrypoint promtool prometheus test rules /tmp/fleet-targetdown.test.yml
echo "PASS: generated FleetTargetDown rule fires after a simulated five-minute outage"
"${COMPOSE_CMD[@]}" run --rm --no-deps --entrypoint amtool alertmanager \
    check-config /etc/alertmanager/alertmanager.yml
"${COMPOSE_CMD[@]}" up -d

wait_http() {
    local name="$1" url="$2"
    for _ in $(seq 1 90); do
        if curl --silent --show-error --fail "$url" >/dev/null 2>&1; then
            echo "READY: $name"
            return 0
        fi
        sleep 2
    done
    echo "timed out waiting for $name at $url" >&2
    return 1
}

wait_http prometheus http://127.0.0.1:9090/-/ready
wait_http alertmanager http://127.0.0.1:9093/-/ready
wait_http grafana http://127.0.0.1:3000/api/health

for item in 'prometheus 9090 9090' 'alertmanager 9093 9093' 'grafana 3000 3000'; do
    read -r service host_port container_port <<<"$item"
    binding=$("${COMPOSE_CMD[@]}" port "$service" "$container_port")
    if [[ "$binding" != *"127.0.0.1:$host_port"* ]]; then
        echo "$service is not bound only to loopback $host_port: $binding" >&2
        exit 1
    fi
done

target_ready=0
for _ in $(seq 1 60); do
    curl --silent --show-error --fail 'http://127.0.0.1:9090/api/v1/targets?state=active' >"$WORK_TMP/targets.json" || true
    if python3 -c 'import json,sys; p=json.load(open(sys.argv[1])); ts=p.get("data",{}).get("activeTargets",[]); ok=any(t.get("health")=="up" and t.get("labels",{}).get("host")=="monitoring-scenario-prometheus" and t.get("labels",{}).get("exporter")=="scenario" and "prometheus:9090" in t.get("scrapeUrl","") for t in ts); sys.exit(0 if ok else 1)' "$WORK_TMP/targets.json" 2>/dev/null; then
        target_ready=1
        break
    fi
    sleep 2
done
if [[ "$target_ready" != 1 ]]; then
    cat "$WORK_TMP/targets.json" >&2
    echo "file-discovery target did not become healthy with the expected labels" >&2
    exit 1
fi
echo "PASS: generated file-discovery target is healthy with host/exporter labels"

PROM_CONTAINER_BEFORE=$("${COMPOSE_CMD[@]}" ps -q prometheus)
[[ -n "$PROM_CONTAINER_BEFORE" ]] || { echo "Prometheus container ID is unavailable" >&2; exit 1; }
PROM_STARTED_BEFORE=$(docker inspect --format '{{.State.StartedAt}}' "$PROM_CONTAINER_BEFORE")

# Exercise the real fleet targets --push SSH code path against a disposable
# OpenSSH container. Its discovery mount is the same host directory Prometheus
# uses, and the inventory transports as root so the production path guard runs.
PUSH_DIR="$WORK_TMP/fleet-target-push"
SSH_HOME="$PUSH_DIR/home"
SSH_CONFIG="$SSH_HOME/.ssh/config"
SSH_KNOWN_HOSTS="$SSH_HOME/.ssh/known_hosts"
SSH_KEY="$SSH_HOME/.ssh/id_ed25519"
SSH_BIN_DIR="$PUSH_DIR/bin"
SSH_ARGS_LOG="$PUSH_DIR/ssh-args.log"
mkdir -p "$SSH_HOME/.ssh" "$SSH_BIN_DIR" "$PUSH_DIR/config/rootfiles" "$PUSH_DIR/state"
chmod 0700 "$SSH_HOME/.ssh"
ssh-keygen -q -t ed25519 -N '' -f "$SSH_KEY"
cat >"$WORK_TMP/Dockerfile.ssh" <<'DOCKERFILE'
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq openssh-server && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd /root/.ssh /etc/rootfiles/monitoring/discovery \
    && chmod 0700 /root/.ssh
EXPOSE 22
CMD ["/bin/sh", "-c", "ssh-keygen -A && exec /usr/sbin/sshd -D -e -o PermitRootLogin=prohibit-password -o PasswordAuthentication=no -o PubkeyAuthentication=yes"]
DOCKERFILE
docker build -q -t "$SSH_IMAGE" -f "$WORK_TMP/Dockerfile.ssh" "$WORK_TMP" >/dev/null
docker run -d --name "$SSH_CONTAINER" -p 127.0.0.1::22 \
    --volume "$CONFIG_DIR/discovery:/etc/rootfiles/monitoring/discovery" \
    "$SSH_IMAGE" >/dev/null
host_key_ready=0
for _ in $(seq 1 30); do
    if docker exec "$SSH_CONTAINER" test -s /etc/ssh/ssh_host_ed25519_key.pub 2>/dev/null; then
        host_key_ready=1
        break
    fi
    sleep 1
done
[[ "$host_key_ready" == 1 ]] || { echo "SSH fixture did not generate its ED25519 host key" >&2; exit 1; }
docker cp "$SSH_KEY.pub" "$SSH_CONTAINER:/root/.ssh/authorized_keys" >/dev/null
docker exec "$SSH_CONTAINER" chown root:root /root/.ssh/authorized_keys
docker exec "$SSH_CONTAINER" chmod 0600 /root/.ssh/authorized_keys
SSH_PORT=$(docker port "$SSH_CONTAINER" 22/tcp | awk -F: '{print $NF}')
KNOWN_HOST_NAMES="monitoring-hub,[monitoring-hub]:$SSH_PORT,[127.0.0.1]:$SSH_PORT"
docker exec "$SSH_CONTAINER" cat /etc/ssh/ssh_host_ed25519_key.pub |
    awk -v names="$KNOWN_HOST_NAMES" 'NF >= 2 { print names, $1, $2 }' >"$SSH_KNOWN_HOSTS"
REAL_SSH=$(command -v ssh)
cat >"$SSH_CONFIG" <<EOF
Host monitoring-hub
  HostName 127.0.0.1
  Port $SSH_PORT
  User root
  IdentityFile $SSH_KEY
  IdentityAgent none
  IdentitiesOnly yes
  HostKeyAlias monitoring-hub
  StrictHostKeyChecking yes
  UserKnownHostsFile $SSH_KNOWN_HOSTS
EOF
chmod 0600 "$SSH_CONFIG" "$SSH_KNOWN_HOSTS" "$SSH_KEY"
cat >"$SSH_BIN_DIR/ssh" <<EOF
#!/bin/sh
printf '%s\\n' "\$*" >>"$SSH_ARGS_LOG"
exec "$REAL_SSH" -F "$SSH_CONFIG" "\$@"
EOF
chmod 0755 "$SSH_BIN_DIR/ssh"
ssh_ready=0
for _ in $(seq 1 30); do
    if [[ "$("$REAL_SSH" -F "$SSH_CONFIG" -o BatchMode=yes -o ConnectTimeout=2 monitoring-hub id -u 2>/dev/null || true)" == 0 ]]; then
        ssh_ready=1
        break
    fi
    sleep 1
done
if [[ "$ssh_ready" != 1 ]]; then
    echo "trusted root SSH fixture did not authenticate as UID 0" >&2
    exit 1
fi

cat >"$PUSH_DIR/hub.yaml" <<'YAML'
users:
  home_base: /home
modules:
  monitoring:
    enabled: true
    hub:
      enabled: true
      data_dir: /var/lib/rootfiles/monitoring
      targets_file: /etc/rootfiles/monitoring/discovery/targets.json
      listen_address: 127.0.0.1
YAML
cat >"$PUSH_DIR/target-up.yaml" <<'YAML'
users:
  home_base: /home
modules:
  monitoring:
    enabled: true
    node_exporter: true
    node_exporter_port: 9090
YAML
cat >"$PUSH_DIR/target-down.yaml" <<'YAML'
users:
  home_base: /home
modules:
  monitoring:
    enabled: true
    node_exporter: true
    node_exporter_port: 1
YAML
cat >"$PUSH_DIR/fleet.yaml" <<'YAML'
defaults:
  sudo: root
  parallel: 1
hosts:
  hub:
    ssh: monitoring-hub
    sudo: root
    config: hub.yaml
  monitoring-scenario-prometheus-updated:
    ssh: unused-target-up
    address: 127.0.0.1
    groups: [scenario]
    config: target-up.yaml
  monitoring-scenario-unreachable:
    ssh: unused-target-down
    address: 127.0.0.1
    groups: [scenario]
    config: target-down.yaml
YAML
if ! PATH="$SSH_BIN_DIR:$PATH" HOME="$SSH_HOME" XDG_CONFIG_HOME="$PUSH_DIR/config" \
    XDG_STATE_HOME="$PUSH_DIR/state" "$CONTROLLER_BIN" fleet targets \
    --inventory "$PUSH_DIR/fleet.yaml" --push hub --yes >"$PUSH_DIR/targets-push.log" 2>&1; then
    cat "$PUSH_DIR/targets-push.log" >&2
    echo "rootfiles fleet targets --push failed through the trusted SSH fixture" >&2
    exit 1
fi
grep -q 'monitoring-hub' "$SSH_ARGS_LOG" || { echo "fleet targets --push did not call the SSH adapter" >&2; exit 1; }
echo "PASS: rootfiles fleet targets --push completed through real key-authenticated SSH as root"
REMOTE_OWNER_MODE=$(docker exec "$SSH_CONTAINER" stat -c '%u:%g:%a' /etc/rootfiles/monitoring/discovery/targets.json)
[[ "$REMOTE_OWNER_MODE" == 0:0:644 ]] || { echo "pushed discovery file has unexpected owner/mode: $REMOTE_OWNER_MODE" >&2; exit 1; }
docker exec "$SSH_CONTAINER" cat /etc/rootfiles/monitoring/discovery/targets.json >"$PUSH_DIR/remote-targets.json"
cmp -s "$TARGETS" "$PUSH_DIR/remote-targets.json" || { echo "SSH target does not share Prometheus' mounted discovery directory" >&2; exit 1; }

file_sd_updated=0
for _ in $(seq 1 90); do
    curl --silent --show-error --fail 'http://127.0.0.1:9090/api/v1/targets?state=active' >"$WORK_TMP/targets-updated.json" || true
    if python3 -c 'import json,sys; p=json.load(open(sys.argv[1])); ts=p.get("data",{}).get("activeTargets",[]); good=any(t.get("health")=="up" and t.get("labels",{}).get("host")=="monitoring-scenario-prometheus-updated" for t in ts); down=any(t.get("health")=="down" and t.get("labels",{}).get("host")=="monitoring-scenario-unreachable" for t in ts); old=any(t.get("labels",{}).get("host")=="monitoring-scenario-prometheus" for t in ts); sys.exit(0 if good and down and not old else 1)' "$WORK_TMP/targets-updated.json" 2>/dev/null; then
        file_sd_updated=1
        break
    fi
    sleep 2
done
[[ "$file_sd_updated" == 1 ]] || { echo "atomic file-discovery update did not replace labels and add the unreachable target" >&2; exit 1; }
PROM_CONTAINER_AFTER=$("${COMPOSE_CMD[@]}" ps -q prometheus)
PROM_STARTED_AFTER=$(docker inspect --format '{{.State.StartedAt}}' "$PROM_CONTAINER_AFTER")
if [[ "$PROM_CONTAINER_AFTER" != "$PROM_CONTAINER_BEFORE" || "$PROM_STARTED_AFTER" != "$PROM_STARTED_BEFORE" ]]; then
    echo "Prometheus restarted during atomic file-discovery update" >&2
    exit 1
fi
echo "PASS: atomic file-discovery replacement appeared without restarting Prometheus"

# Exercise the real generated FleetTargetDown rule against the unreachable
# target. The rule's production `for: 5m` hold and Alertmanager delivery are
# observed live; the separate promtool test above keeps the virtual-time check.
fleet_down_alert_verified=0
for _ in $(seq 1 210); do
    curl --silent --show-error --fail 'http://127.0.0.1:9090/api/v1/alerts' >"$WORK_TMP/prom-alerts.json" || true
    prom_firing=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1])); alerts=p.get("data",{}).get("alerts",[]); print("yes" if any(a.get("labels",{}).get("alertname")=="FleetTargetDown" and a.get("labels",{}).get("host")=="monitoring-scenario-unreachable" and a.get("state")=="firing" for a in alerts) else "no")' "$WORK_TMP/prom-alerts.json" 2>/dev/null || true)
    if [[ "$prom_firing" == yes ]]; then
        curl --silent --show-error --fail 'http://127.0.0.1:9093/api/v2/alerts' >"$WORK_TMP/am-alerts.json" || true
        if python3 -c 'import json,sys; alerts=json.load(open(sys.argv[1])); sys.exit(0 if any(a.get("labels",{}).get("alertname")=="FleetTargetDown" and a.get("labels",{}).get("host")=="monitoring-scenario-unreachable" for a in alerts) else 1)' "$WORK_TMP/am-alerts.json" 2>/dev/null; then
            fleet_down_alert_verified=1
            break
        fi
    fi
    sleep 2
done
[[ "$fleet_down_alert_verified" == 1 ]] || {
    echo "FleetTargetDown did not fire and reach Alertmanager for monitoring-scenario-unreachable within seven minutes" >&2
    exit 1
}
echo "PASS: live FleetTargetDown exceeded its five-minute hold and reached Alertmanager for the unreachable target"

alert_fired=0
for _ in $(seq 1 90); do
    curl --silent --show-error --fail 'http://127.0.0.1:9090/api/v1/alerts' >"$WORK_TMP/prom-alerts.json" || true
    if python3 -c 'import json,sys; p=json.load(open(sys.argv[1])); a=p.get("data",{}).get("alerts",[]); sys.exit(0 if any(x.get("labels",{}).get("alertname")=="RootfilesMonitoringScenarioAlwaysFires" and x.get("state")=="firing" for x in a) else 1)' "$WORK_TMP/prom-alerts.json" 2>/dev/null; then
        alert_fired=1
        break
    fi
    sleep 2
done
[[ "$alert_fired" == 1 ]] || { echo "Prometheus did not fire the synthetic acceptance rule" >&2; exit 1; }

alert_received=0
for _ in $(seq 1 90); do
    curl --silent --show-error --fail 'http://127.0.0.1:9093/api/v2/alerts' >"$WORK_TMP/am-alerts.json" || true
    if python3 -c 'import json,sys; a=json.load(open(sys.argv[1])); sys.exit(0 if any(x.get("labels",{}).get("alertname")=="RootfilesMonitoringScenarioAlwaysFires" for x in a) else 1)' "$WORK_TMP/am-alerts.json" 2>/dev/null; then
        alert_received=1
        break
    fi
    sleep 2
done
[[ "$alert_received" == 1 ]] || { echo "Alertmanager did not receive the synthetic acceptance alert" >&2; exit 1; }

curl --silent --show-error --fail 'http://127.0.0.1:3000/api/health' >"$WORK_TMP/grafana-health.json"
echo "PASS: generated Prometheus, Alertmanager, and Grafana configs are running; synthetic alert reached the no-op receiver"
echo "NOTE: file discovery targets Prometheus itself as a fixture only; it does not simulate GPU or DCGM telemetry."
