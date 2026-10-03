#!/usr/bin/env bash
# Run the opt-in real Prometheus/Alertmanager/Grafana acceptance scenario.
# First generate fixtures with:
#   ROOTFILES_MONITORING_SCENARIO_DIR="$PWD/hub-fixtures" \
#     go test ./internal/module -run '^TestWriteMonitoringScenarioFixtures$' -count=1
# Then run: tests/scenarios/monitoring-hub.sh hub-fixtures
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
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

for command in docker curl python3; do
    command -v "$command" >/dev/null 2>&1 || { echo "required command not found: $command" >&2; exit 2; }
done
docker compose version >/dev/null
for file in "$COMPOSE" "$PROM_CONFIG" "$ALERT_RULES" "$TARGETS" \
    "$RECEIVER" "$CONFIG_DIR/grafana-admin-password" "$CONFIG_DIR/telegram-bot-token" \
    "$FIXTURE_ROOT/fixture-root.txt"; do
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
WORK_TMP="$(mktemp -d)"
COMPOSE_CMD=(docker compose --project-name "$PROJECT" --file "$COMPOSE")
cleanup() {
    local status=$?
    if [[ "$status" -ne 0 ]]; then
        "${COMPOSE_CMD[@]}" logs --no-color >&2 2>/dev/null || true
    fi
    "${COMPOSE_CMD[@]}" down --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$WORK_TMP"
    return "$status"
}
trap cleanup EXIT

# Rendered hub services run as container root with capabilities dropped. Set
# only their disposable data tree to root ownership for writes.
mkdir -p "$DATA_DIR/prometheus" "$DATA_DIR/alertmanager" "$DATA_DIR/grafana"
docker run --rm --volume "$DATA_DIR:/scenario-data" busybox:1.36.1 sh -c \
    'chown -R 0:0 /scenario-data && chmod 0755 /scenario-data /scenario-data/prometheus /scenario-data/alertmanager /scenario-data/grafana'

"${COMPOSE_CMD[@]}" config --quiet
"${COMPOSE_CMD[@]}" pull
"${COMPOSE_CMD[@]}" run --rm --no-deps --entrypoint promtool prometheus \
    check config /etc/prometheus/prometheus.yaml
"${COMPOSE_CMD[@]}" run --rm --no-deps --entrypoint promtool prometheus \
    check rules /etc/prometheus/alert-rules.yaml
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

curl --silent --show-error --fail 'http://127.0.0.1:9090/api/v1/targets?state=active' >"$WORK_TMP/targets.json"
python3 -c 'import json,sys; p=json.load(open(sys.argv[1])); ts=p.get("data",{}).get("activeTargets",[]); ok=any(t.get("health")=="up" and t.get("labels",{}).get("host")=="monitoring-scenario-prometheus" and t.get("labels",{}).get("exporter")=="scenario" and "prometheus:9090" in t.get("scrapeUrl","") for t in ts); sys.exit(0 if ok else 1)' "$WORK_TMP/targets.json" || {
    echo "file-discovery target did not become healthy with the expected labels" >&2
    exit 1
}
echo "PASS: generated file-discovery target is healthy with host/exporter labels"

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
