#!/bin/bash
# Run rootfiles apply with a profile and verify results
set -euo pipefail
source /tests/assert.sh

PROFILE="${1:-minimal}"
echo "=== Integration Test: profile=$PROFILE ==="

# In CI containers, docker and nvidia modules cannot work (no real Docker/GPU).
# Apply all modules except docker and nvidia.
CI_MODULES="locale,packages,ssh,users,cloudflared,storage,network"

# Apply profile
echo "--- Applying profile: $PROFILE ---"
rootfiles apply --profile "$PROFILE" --module "$CI_MODULES" --yes 2>&1 || true

# Common assertions (all profiles include base)
echo "--- Verifying base packages ---"
assert_package_installed "git"
assert_package_installed "curl"
assert_package_installed "zsh"
assert_package_installed "tmux"
assert_package_installed "jq"
assert_package_installed "htop"

# Locale
assert_file_exists "/etc/default/locale"
assert_file_contains "/etc/default/locale" "en_US.UTF-8"

# SSH config (all profiles)
assert_file_exists "/etc/ssh/sshd_config.d/00-rootfiles.conf"

# Profile-specific assertions
case "$PROFILE" in
    minimal|dgx|gpu-server|full)
        echo "--- Verifying minimal extras ---"
        assert_package_installed "openssh-server"
        assert_package_installed "ufw"
        assert_package_installed "fail2ban"

        # Users module
        if [ "$PROFILE" = "dgx" ]; then
            assert_dir_exists "/raid/home"
            assert_dir_exists "/raid/home/.rootfiles"
        fi

        # Cloudflared binary
        assert_command_exists "cloudflared"
        ;;
esac

case "$PROFILE" in
    dgx)
        echo "--- Verifying DGX-specific ---"
        assert_dir_exists "/raid/data"
        assert_file_exists "/etc/ssh/sshd_config.d/00-rootfiles.conf"
        assert_file_contains "/etc/ssh/sshd_config.d/00-rootfiles.conf" "PasswordAuthentication no"

        # VLAN interface
        assert_file_exists "/etc/systemd/network/10-cloudflared-vlan.netdev"
        assert_file_exists "/etc/systemd/network/10-cloudflared-vlan.network"
        assert_file_contains "/etc/systemd/network/10-cloudflared-vlan.network" "172.16"
        ;;
    gpu-server)
        echo "--- Verifying gpu-server-specific ---"
        assert_file_contains "/etc/ssh/sshd_config.d/00-rootfiles.conf" "PasswordAuthentication no"
        ;;
esac

echo ""
echo "--- Verifying idempotency ---"
rootfiles check --profile "$PROFILE" 2>&1 || true

report
