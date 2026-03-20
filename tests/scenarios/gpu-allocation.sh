#!/bin/bash
# Test GPU allocation assign / list / status / revoke workflow
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: gpu-allocation ==="

# Create test users
useradd -m -d /raid/home/gpuuser1 -s /bin/bash gpuuser1
useradd -m -d /raid/home/gpuuser2 -s /bin/bash gpuuser2

# --- Assign GPUs ---
echo "--- Assigning GPUs ---"

rootfiles gpu assign gpuuser1 --gpus 0,1,2,3 --method env --yes 2>&1
assert_file_exists "/etc/profile.d/rootfiles-gpu-gpuuser1.sh"
assert_file_contains "/etc/profile.d/rootfiles-gpu-gpuuser1.sh" "CUDA_VISIBLE_DEVICES=0,1,2,3"
assert_file_contains "/etc/profile.d/rootfiles-gpu-gpuuser1.sh" "NVIDIA_VISIBLE_DEVICES=0,1,2,3"
assert_file_contains "/etc/profile.d/rootfiles-gpu-gpuuser1.sh" "gpuuser1"
pass "gpuuser1 assigned GPUs 0-3"

rootfiles gpu assign gpuuser2 --gpus 4,5,6,7 --method env --yes 2>&1
assert_file_exists "/etc/profile.d/rootfiles-gpu-gpuuser2.sh"
assert_file_contains "/etc/profile.d/rootfiles-gpu-gpuuser2.sh" "CUDA_VISIBLE_DEVICES=4,5,6,7"
pass "gpuuser2 assigned GPUs 4-7"

# --- List allocations ---
echo ""
echo "--- Listing allocations ---"

list_output=$(rootfiles gpu list --yes 2>&1)
echo "$list_output"

if echo "$list_output" | grep -q "gpuuser1"; then
    pass "list shows gpuuser1"
else
    fail "list missing gpuuser1"
fi

if echo "$list_output" | grep -q "gpuuser2"; then
    pass "list shows gpuuser2"
else
    fail "list missing gpuuser2"
fi

if echo "$list_output" | grep -q "0,1,2,3"; then
    pass "list shows GPU indices for gpuuser1"
else
    fail "list missing GPU indices for gpuuser1"
fi

# --- Conflict detection ---
echo ""
echo "--- Testing conflict detection ---"

if rootfiles gpu assign gpuuser2 --gpus 0 --method env --yes 2>&1; then
    fail "assigning already-taken GPU should fail"
else
    pass "conflict detected: GPU 0 already assigned"
fi

# --- GPU status ---
echo ""
echo "--- GPU status ---"

status_output=$(rootfiles gpu status --yes 2>&1)
echo "$status_output"

if echo "$status_output" | grep -q "gpuuser1"; then
    pass "status shows gpuuser1"
else
    fail "status missing gpuuser1"
fi

if echo "$status_output" | grep -q "gpuuser2"; then
    pass "status shows gpuuser2"
else
    fail "status missing gpuuser2"
fi

# --- Update assignment ---
echo ""
echo "--- Updating assignment ---"

rootfiles gpu assign gpuuser1 --gpus 0,1 --method env --yes 2>&1
assert_file_contains "/etc/profile.d/rootfiles-gpu-gpuuser1.sh" "CUDA_VISIBLE_DEVICES=0,1"
pass "gpuuser1 updated to GPUs 0,1"

# --- Revoke ---
echo ""
echo "--- Revoking allocation ---"

rootfiles gpu revoke gpuuser1 --yes 2>&1

if [ ! -f "/etc/profile.d/rootfiles-gpu-gpuuser1.sh" ]; then
    pass "gpuuser1 env script removed"
else
    fail "gpuuser1 env script still exists after revoke"
fi

# Verify gpuuser2 still allocated
list_after=$(rootfiles gpu list --yes 2>&1)
if echo "$list_after" | grep -q "gpuuser2"; then
    pass "gpuuser2 allocation preserved after revoking gpuuser1"
else
    fail "gpuuser2 allocation lost"
fi

if echo "$list_after" | grep -q "gpuuser1"; then
    fail "gpuuser1 still in list after revoke"
else
    pass "gpuuser1 removed from list"
fi

# --- Dry-run ---
echo ""
echo "--- Dry-run mode ---"

rootfiles gpu assign gpuuser1 --gpus 0 --method env --dry-run --yes 2>&1
if [ ! -f "/etc/profile.d/rootfiles-gpu-gpuuser1.sh" ]; then
    pass "dry-run did not create env script"
else
    fail "dry-run created env script"
fi

# --- Check via apply ---
echo ""
echo "--- Module check/apply ---"

rootfiles gpu assign gpuuser1 --gpus 0,1 --method env --yes 2>&1

check_output=$(rootfiles check --profile dgx --module gpu --yes 2>&1)
echo "$check_output"

apply_output=$(rootfiles apply --profile dgx --module gpu --yes 2>&1)
echo "$apply_output"

report
