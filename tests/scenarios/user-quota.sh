#!/bin/bash
# Per-user quota on an ext4 home_base with user quotas (loop device)
set -euo pipefail
source /tests/assert.sh

echo "=== Scenario: user-quota ==="
apt-get update -qq >/dev/null && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq quota >/dev/null
truncate -s 400M /q.img
mkfs.ext4 -q -F /q.img
mkdir -p /qhome
mount -o loop /q.img /qhome
export ROOTFILES_HOME_BASE=/qhome

rootfiles user add qa --profile minimal --yes --home-base /qhome 2>&1 || true
assert_user_exists "qa"

echo "--- refuses without quota mount options ---"
out=$(rootfiles user quota set qa 50M --profile minimal --home-base /qhome 2>&1 || true)
if grep -q "not mounted with usrquota" <<<"$out"; then pass "explains missing usrquota"; else fail "unexpected: $out"; fi

echo "--- operator enables quotas (remount usrquota, quotacheck, quotaon) ---"
umount /qhome
if ! { mount -o loop,usrquota /q.img /qhome && quotacheck -cum /qhome && quotaon /qhome; } 2>/dev/null; then
    echo "kernel has no quota support here; skipping enforcement checks"
    report
    exit 0
fi

rootfiles user quota set qa 50M --profile minimal --home-base /qhome 2>&1 || true
out=$(rootfiles user quota show --profile minimal --home-base /qhome 2>&1 || true)
if grep -Eq "^qa .*(50M|51200K)" <<<"$out"; then pass "quota visible in report"; else fail "quota report: $out"; fi

if su qa -c "dd if=/dev/zero of=/qhome/qa/big bs=1M count=80 status=none" 2>/dev/null; then
    fail "quota not enforced"
else
    pass "quota enforced"
fi
assert_file_contains "/qhome/.rootfiles/users.json" '"quota": "50M"'

rootfiles user quota rm qa --profile minimal --home-base /qhome 2>&1 || true
rm -f /qhome/qa/big
if su qa -c "dd if=/dev/zero of=/qhome/qa/big bs=1M count=80 status=none" 2>/dev/null; then
    pass "quota removed"
else
    fail "quota still enforced after rm"
fi

umount /qhome || true
report
