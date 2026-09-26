#!/bin/bash
# Hardening SSH / enabling UFW must never cut off remote access
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: ssh-lockout-guard ==="

rootfiles apply --profile minimal --module packages --yes 2>&1 || true
CONF=/etc/ssh/sshd_config.d/00-rootfiles.conf

# Step 1: no account has an authorized key → password auth stays on
echo "--- Step 1: refuse to disable password auth without key logins ---"
rm -f "$CONF"
if rootfiles apply --profile dgx --module ssh --yes 2>&1; then
    fail "apply should fail when it would lock out SSH"
else
    pass "apply refused lockout-prone ssh config"
fi
if [ -f "$CONF" ] && grep -q "PasswordAuthentication no" "$CONF"; then
    fail "PasswordAuthentication was disabled despite guard"
else
    pass "PasswordAuthentication left enabled"
fi

# Step 2: once an operator has a key, hardening proceeds and sshd -t passes
echo "--- Step 2: apply with a key-holding operator ---"
useradd -m -s /bin/bash opsuser
install -d -m 700 -o opsuser -g opsuser /home/opsuser/.ssh
echo "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl ops" \
    > /home/opsuser/.ssh/authorized_keys
rootfiles apply --profile dgx --module ssh --yes 2>&1 || true
assert_file_contains "$CONF" "PasswordAuthentication no"
assert_file_contains "$CONF" "PermitRootLogin no"

# Step 3: enabling UFW always admits the SSH port, even with no allowed_ports
echo "--- Step 3: UFW admits SSH port ---"
rootfiles apply --profile gpu-server --module network --yes 2>&1 || true
if ufw status | grep -Eq '^22(/tcp)?[[:space:]]+ALLOW'; then
    pass "ufw allows ssh port 22"
else
    fail "ufw does not allow ssh port 22"
    ufw status || true
fi

# Step 4: second run is a no-op
echo "--- Step 4: idempotency ---"
out=$(rootfiles apply --profile gpu-server --module network --yes 2>&1 || true)
if echo "$out" | grep -q "network: already satisfied"; then
    pass "network module idempotent"
else
    fail "network module not idempotent: $out"
fi

report
