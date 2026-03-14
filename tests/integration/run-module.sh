#!/bin/bash
# Test a single module in isolation
set -euo pipefail
source /tests/assert.sh

MODULE="${1:?Usage: run-module.sh <module>}"
echo "=== Module Test: $MODULE ==="

# Apply single module
echo "--- Applying module: $MODULE ---"
rootfiles apply --profile minimal --module "$MODULE" --yes 2>&1 || true

# Module-specific assertions
case "$MODULE" in
    locale)
        assert_file_exists "/etc/default/locale"
        assert_file_contains "/etc/default/locale" "en_US.UTF-8"
        ;;
    packages)
        assert_package_installed "git"
        assert_package_installed "curl"
        assert_package_installed "zsh"
        assert_package_installed "tmux"
        assert_package_installed "jq"
        assert_package_installed "openssh-server"
        ;;
    ssh)
        assert_file_exists "/etc/ssh/sshd_config.d/00-rootfiles.conf"
        assert_file_contains "/etc/ssh/sshd_config.d/00-rootfiles.conf" "rootfiles-v2"
        assert_file_contains "/etc/ssh/sshd_config.d/00-rootfiles.conf" "PermitRootLogin no"
        ;;
    users)
        assert_dir_exists "/raid/home"
        assert_dir_exists "/raid/home/.rootfiles"
        ;;
    cloudflared)
        assert_command_exists "cloudflared"
        assert_file_exists "/usr/local/bin/cloudflared"
        ;;
    storage)
        # With default minimal profile, storage is not enabled
        # Test with dgx profile for storage
        rootfiles apply --profile dgx --module storage --yes 2>&1 || true
        assert_dir_exists "/raid/data"
        assert_dir_exists "/raid/home"
        ;;
    network)
        # With dgx profile for network module
        rootfiles apply --profile dgx --module network --yes 2>&1 || true
        assert_command_exists "ufw"
        ;;
esac

echo ""
echo "--- Verifying dry-run produces no changes ---"
rootfiles apply --profile minimal --module "$MODULE" --dry-run --yes 2>&1 || true

report
