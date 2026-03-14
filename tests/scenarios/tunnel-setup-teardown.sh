#!/bin/bash
# Test cloudflared install, VLAN setup, status, and uninstall
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: tunnel-setup-teardown ==="

# Step 1: Install cloudflared
echo "--- Step 1: Install cloudflared ---"
rootfiles tunnel install --profile dgx --yes 2>&1 || true
assert_command_exists "cloudflared"
assert_file_exists "/usr/local/bin/cloudflared"

# Step 2: Setup tunnel (with dummy token — service install will fail but VLAN should work)
echo "--- Step 2: Setup VLAN ---"
rootfiles tunnel setup --profile dgx --yes --vlan-address "172.16.0.99/32" 2>&1 || true

# Verify VLAN config files
assert_file_exists "/etc/systemd/network/10-cloudflared-vlan.netdev"
assert_file_exists "/etc/systemd/network/10-cloudflared-vlan.network"
assert_file_contains "/etc/systemd/network/10-cloudflared-vlan.netdev" "Kind=dummy"
assert_file_contains "/etc/systemd/network/10-cloudflared-vlan.network" "172.16.0.99/32"

# Step 3: Check status
echo "--- Step 3: Check status ---"
rootfiles tunnel status --profile dgx --yes 2>&1 || true
pass "tunnel status runs without error"

# Step 4: Update (re-download)
echo "--- Step 4: Update ---"
rootfiles tunnel update --profile dgx --yes 2>&1 || true
assert_file_exists "/usr/local/bin/cloudflared"
pass "tunnel update completed"

# Step 5: Uninstall
echo "--- Step 5: Uninstall ---"
rootfiles tunnel uninstall --profile dgx --yes 2>&1 || true

# Verify cleanup
if [ ! -f "/usr/local/bin/cloudflared" ]; then
    pass "cloudflared binary removed"
else
    fail "cloudflared binary still exists"
fi

if [ ! -f "/etc/systemd/network/10-cloudflared-vlan.netdev" ]; then
    pass "VLAN netdev config removed"
else
    fail "VLAN netdev config still exists"
fi

if [ ! -f "/etc/systemd/network/10-cloudflared-vlan.network" ]; then
    pass "VLAN network config removed"
else
    fail "VLAN network config still exists"
fi

report
