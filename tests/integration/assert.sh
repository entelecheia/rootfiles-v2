#!/bin/bash
# Common assertion functions for integration tests
set -euo pipefail

PASS=0
FAIL=0

fail() {
    echo "  FAIL: $1"
    FAIL=$((FAIL + 1))
}

pass() {
    echo "  PASS: $1"
    PASS=$((PASS + 1))
}

assert_package_installed() {
    if dpkg -l "$1" 2>/dev/null | grep -q "^ii"; then
        pass "package $1 installed"
    else
        fail "package $1 not installed"
    fi
}

assert_package_not_installed() {
    if dpkg -l "$1" 2>/dev/null | grep -q "^ii"; then
        fail "package $1 should not be installed"
    else
        pass "package $1 not installed (expected)"
    fi
}

assert_user_exists() {
    if id "$1" >/dev/null 2>&1; then
        pass "user $1 exists"
    else
        fail "user $1 not found"
    fi
}

assert_user_not_exists() {
    if id "$1" >/dev/null 2>&1; then
        fail "user $1 should not exist"
    else
        pass "user $1 does not exist (expected)"
    fi
}

assert_user_home() {
    local actual
    actual=$(getent passwd "$1" | cut -d: -f6)
    if [ "$actual" = "$2" ]; then
        pass "user $1 home = $2"
    else
        fail "user $1 home = $actual, expected $2"
    fi
}

assert_user_shell() {
    local actual
    actual=$(getent passwd "$1" | cut -d: -f7)
    if [ "$actual" = "$2" ]; then
        pass "user $1 shell = $2"
    else
        fail "user $1 shell = $actual, expected $2"
    fi
}

assert_user_in_group() {
    if groups "$1" 2>/dev/null | grep -qw "$2"; then
        pass "user $1 in group $2"
    else
        fail "user $1 not in group $2"
    fi
}

assert_user_uid() {
    local actual
    actual=$(id -u "$1" 2>/dev/null)
    if [ "$actual" = "$2" ]; then
        pass "user $1 uid = $2"
    else
        fail "user $1 uid = $actual, expected $2"
    fi
}

assert_file_exists() {
    if [ -f "$1" ]; then
        pass "file $1 exists"
    else
        fail "file $1 missing"
    fi
}

assert_dir_exists() {
    if [ -d "$1" ]; then
        pass "dir $1 exists"
    else
        fail "dir $1 missing"
    fi
}

assert_file_contains() {
    if grep -q "$2" "$1" 2>/dev/null; then
        pass "file $1 contains '$2'"
    else
        fail "file $1 does not contain '$2'"
    fi
}

assert_symlink() {
    if [ -L "$1" ]; then
        local target
        target=$(readlink -f "$1")
        if [ "$target" = "$2" ]; then
            pass "symlink $1 → $2"
        else
            fail "symlink $1 → $target, expected $2"
        fi
    else
        fail "$1 is not a symlink"
    fi
}

assert_interface_exists() {
    if ip link show "$1" >/dev/null 2>&1; then
        pass "interface $1 exists"
    else
        fail "interface $1 missing"
    fi
}

assert_interface_addr() {
    if ip addr show "$1" 2>/dev/null | grep -q "$2"; then
        pass "interface $1 has addr $2"
    else
        fail "interface $1 missing addr $2"
    fi
}

assert_command_exists() {
    if command -v "$1" >/dev/null 2>&1; then
        pass "command $1 exists"
    else
        fail "command $1 not found"
    fi
}

assert_exit_code() {
    local desc="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        pass "$desc (exit 0)"
    else
        fail "$desc (non-zero exit)"
    fi
}

assert_no_changes() {
    local output
    output=$("$@" 2>&1)
    if echo "$output" | grep -q "already satisfied\|No modules"; then
        pass "idempotent: no changes on re-run"
    else
        fail "expected no changes on re-run, got: $output"
    fi
}

report() {
    echo ""
    echo "========================================="
    echo "Results: $PASS passed, $FAIL failed"
    echo "========================================="
    if [ "$FAIL" -gt 0 ]; then
        exit 1
    fi
}
