#!/bin/bash
# apply --user/--ssh-pubkey creates the operator (users runs before ssh),
# so hardening SSH in the same run is safe; re-runs converge keys/groups.
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: declarative-accounts ==="
KEY1="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl one"
KEY2="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHkRJcXi3BRu5lYx2L2Umn2k1vwRHNyxq1Y8r5yIS6xN two"

rootfiles apply --profile minimal --module packages --yes 2>&1 || true

echo "--- Step 1: fresh host, apply with --user/--ssh-pubkey ---"
rootfiles apply --profile dgx --module users,ssh --yes --home-base /raid/home \
    --user opsadmin --ssh-pubkey "$KEY1" 2>&1 || true
assert_user_exists "opsadmin"
assert_user_home "opsadmin" "/raid/home/opsadmin"
assert_file_contains "/raid/home/opsadmin/.ssh/authorized_keys" "one"
assert_file_contains "/etc/ssh/sshd_config.d/00-rootfiles.conf" "PasswordAuthentication no"

echo "--- Step 2: add a second key via config file ---"
cat > /tmp/site.yaml <<YAML
extends: dgx
users:
  home_base: /raid/home
  accounts:
    - name: opsadmin
      ssh_pubkeys:
        - "$KEY1"
        - "$KEY2"
YAML
rootfiles apply --config /tmp/site.yaml --module users,ssh --yes 2>&1 || true
assert_file_contains "/raid/home/opsadmin/.ssh/authorized_keys" "one"
assert_file_contains "/raid/home/opsadmin/.ssh/authorized_keys" "two"

echo "--- Step 3: converged ---"
out=$(rootfiles check --config /tmp/site.yaml --module users,ssh 2>&1 || true)
if echo "$out" | grep -q "PENDING"; then
    fail "check still reports pending changes: $out"
else
    pass "users/ssh converged"
fi

report
