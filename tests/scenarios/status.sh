#!/bin/bash
# Test the unified `rootfiles status` dashboard.
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: status ==="
export ROOTFILES_HOME_BASE=/raid/home

# Seed a modest environment: a profile's worth of packages + one managed user
# + one GPU allocation so the Users / GPU Allocations sections have content
# rather than only rendering their empty-state hints.
echo "--- Setup ---"
mkdir -p /raid/home/.rootfiles
rootfiles apply --profile minimal --module packages --yes 2>&1 || true
rootfiles user add statususer --profile dgx --yes --home-base /raid/home 2>&1 || true

if command -v setup-fake-gpus >/dev/null 2>&1; then
    setup-fake-gpus
    useradd -m -d /raid/home/gpuseed -s /bin/bash gpuseed 2>/dev/null || true
    rootfiles gpu assign gpuseed --gpus 0 --method env --yes 2>&1 || true
fi

# Run the command; status must always exit 0 regardless of how many modules
# are satisfied — it's a dashboard, not a gate.
echo "--- Running status ---"
OUTPUT=$(rootfiles status --profile dgx --yes 2>&1)
echo "$OUTPUT"

# Six canonical sections must always render. A missing one means either the
# rendering code dropped the section or a data source panicked silently.
for section in "rootfiles status" "System" "Profile" "Modules" "GPU Allocations" "Tunnel" "Users"; do
    if echo "$OUTPUT" | grep -qF "$section"; then
        pass "section present: $section"
    else
        fail "section missing: $section"
    fi
done

# Data content checks — these guard against regressions where status renders
# a header but silently stops filling in the body.
if echo "$OUTPUT" | grep -qE "OS:"; then
    pass "System reports OS"
else
    fail "System OS row missing"
fi

if echo "$OUTPUT" | grep -qE "Active:"; then
    pass "Profile reports active profile"
else
    fail "Profile Active row missing"
fi

if echo "$OUTPUT" | grep -qE "Home base:"; then
    pass "Users reports home base"
else
    fail "Users Home base row missing"
fi

# --- Verify NO_COLOR / non-TTY strips ANSI ---
echo "--- NO_COLOR sanitises output ---"
NO_COLOR_OUTPUT=$(NO_COLOR=1 rootfiles status --profile dgx --yes 2>&1)
if echo "$NO_COLOR_OUTPUT" | grep -q $'\x1b\['; then
    fail "NO_COLOR=1 still produced ANSI escape sequences"
else
    pass "NO_COLOR=1 produces plain output"
fi

report
