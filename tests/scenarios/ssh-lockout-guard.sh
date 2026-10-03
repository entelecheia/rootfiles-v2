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

# Step 2b: a password-only account blocks hardening until it gets a key or an
# exception; the exception keeps password login for that user only.
echo "--- Step 2b: password-only accounts ---"
rm -f "$CONF"
useradd -m -s /bin/bash pwuser
echo "pwuser:Ci-Passw0rd-2b" | chpasswd
if out=$(rootfiles apply --profile dgx --module ssh --yes 2>&1); then
    fail "apply should refuse while pwuser can only log in with a password"
elif echo "$out" | grep -q "pwuser"; then
    pass "guard names the password-only account"
else
    fail "guard did not name pwuser: $out"
fi
cat > /tmp/site-pw.yaml <<'EOF'
extends: dgx
ssh:
  password_auth_users: [pwuser]
EOF
rootfiles apply --config /tmp/site-pw.yaml --module ssh --yes 2>&1 || true
assert_file_contains "$CONF" "PasswordAuthentication no"
assert_file_contains "$CONF" "Match User pwuser"
mkdir -p /run/sshd
if sshd -T -C user=pwuser,host=ci,addr=10.0.0.1 | grep -qx "passwordauthentication yes" \
   && sshd -T -C user=opsuser,host=ci,addr=10.0.0.1 | grep -qx "passwordauthentication no"; then
    pass "password login kept for pwuser only"
else
    fail "effective sshd config does not scope the exception to pwuser"
fi

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
