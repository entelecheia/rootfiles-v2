#!/bin/bash
# Test user list --names flag
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: user-list-names ==="
export ROOTFILES_HOME_BASE=/raid/home

# Setup: create home base and users
echo "--- Setup ---"
mkdir -p /raid/home/.rootfiles
rootfiles apply --profile dgx --module packages --yes 2>&1 || true
rootfiles user add alice --profile dgx --yes --home-base /raid/home 2>&1 || true
rootfiles user add bob --profile dgx --yes --home-base /raid/home 2>&1 || true

# Step 1: Test normal list (table output)
echo "--- Step 1: Normal list ---"
OUTPUT=$(rootfiles user list --profile dgx --yes 2>&1)
if echo "$OUTPUT" | grep -q "USER"; then
    pass "normal list has table header"
else
    fail "normal list missing table header"
fi

# Step 2: Test --names flag (names only)
echo "--- Step 2: List --names ---"
NAMES=$(rootfiles user list --names --profile dgx --yes 2>&1)
if echo "$NAMES" | grep -q "^alice$"; then
    pass "--names output contains alice"
else
    fail "--names output missing alice"
fi
if echo "$NAMES" | grep -q "^bob$"; then
    pass "--names output contains bob"
else
    fail "--names output missing bob"
fi
# Should NOT have table headers
if echo "$NAMES" | grep -q "USER"; then
    fail "--names should not have table header"
else
    pass "--names has no table header"
fi

# Step 3: Test script usage (pipe to wc)
echo "--- Step 3: Script usage ---"
COUNT=$(rootfiles user list --names --profile dgx --yes 2>/dev/null | wc -l | tr -d ' ')
if [ "$COUNT" -ge 2 ]; then
    pass "user count = $COUNT (>= 2)"
else
    fail "user count = $COUNT, expected >= 2"
fi

report
