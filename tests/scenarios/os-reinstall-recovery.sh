#!/bin/bash
# Simulate OS reinstall: users deleted but /raid/home preserved, then restore
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: os-reinstall-recovery ==="
export ROOTFILES_HOME_BASE=/raid/home

# Setup
rootfiles apply --profile minimal --module packages --yes 2>&1 || true

# Phase 1: Initial setup (before OS reinstall)
echo "--- Phase 1: Initial setup ---"
rootfiles user add devuser --profile dgx --yes --home-base /raid/home 2>&1 || true
rootfiles user add admin --profile dgx --yes --home-base /raid/home 2>&1 || true

assert_user_exists "devuser"
assert_user_exists "admin"
assert_user_home "devuser" "/raid/home/devuser"

# Create important files
echo "my-project-data" > /raid/home/devuser/project.txt
mkdir -p /raid/home/devuser/.ssh
echo "ssh-ed25519 TESTKEY" > /raid/home/devuser/.ssh/authorized_keys

DEVUSER_UID=$(id -u devuser)

# Apply full system config
echo "--- Apply full DGX config ---"
rootfiles apply --profile dgx --yes 2>&1 || true

# Verify metadata saved
assert_file_exists "/raid/home/.rootfiles/users.json"

# Phase 2: Simulate OS reinstall
echo ""
echo "--- Phase 2: Simulate OS reinstall ---"
echo "(Deleting users from /etc/passwd but preserving /raid/home)"
userdel devuser 2>/dev/null || true
userdel admin 2>/dev/null || true

# Remove system configs (simulates fresh OS)
rm -f /etc/ssh/sshd_config.d/00-rootfiles.conf
rm -f /etc/default/locale

assert_user_not_exists "devuser"
assert_user_not_exists "admin"

# But /raid/home is preserved!
assert_dir_exists "/raid/home/devuser"
assert_file_exists "/raid/home/devuser/project.txt"
assert_file_exists "/raid/home/.rootfiles/users.json"

# Phase 3: Recovery on new OS
echo ""
echo "--- Phase 3: Recovery ---"

# Re-apply system config
rootfiles apply --profile dgx --yes 2>&1 || true

# Restore users from preserved metadata
rootfiles user restore --profile dgx --yes 2>&1 || true

# Verify users restored
assert_user_exists "devuser"
assert_user_exists "admin"
assert_user_home "devuser" "/raid/home/devuser"
assert_user_uid "devuser" "$DEVUSER_UID"

# Verify data preserved
assert_file_exists "/raid/home/devuser/project.txt"
assert_file_contains "/raid/home/devuser/project.txt" "my-project-data"
assert_file_exists "/raid/home/devuser/.ssh/authorized_keys"

# Verify system config restored
assert_file_exists "/etc/ssh/sshd_config.d/00-rootfiles.conf"
assert_file_exists "/etc/default/locale"

report
