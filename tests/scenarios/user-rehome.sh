#!/bin/bash
# Test moving a user from /home to custom home base
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: user-rehome ==="

# Setup
rootfiles apply --profile minimal --module packages --yes 2>&1 || true

# Step 1: Create user at default /home
echo "--- Step 1: Create user at /home ---"
useradd --home-dir /home/rehomeuser --create-home --shell /usr/bin/zsh rehomeuser 2>/dev/null || true
echo "important-data" > /home/rehomeuser/myfile

assert_user_exists "rehomeuser"
assert_user_home "rehomeuser" "/home/rehomeuser"
assert_file_exists "/home/rehomeuser/myfile"

# Step 2: Rehome to /raid/home
echo "--- Step 2: Rehome to /raid/home ---"
rootfiles user rehome rehomeuser --profile dgx --yes --home-base /raid/home 2>&1 || true

# Step 3: Verify
echo "--- Step 3: Verify ---"
assert_user_home "rehomeuser" "/raid/home/rehomeuser"
assert_file_exists "/raid/home/rehomeuser/myfile"
assert_file_contains "/raid/home/rehomeuser/myfile" "important-data"
assert_symlink "/home/rehomeuser" "/raid/home/rehomeuser"

report
