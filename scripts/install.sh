#!/bin/bash
# rootfiles-v2 installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/entelecheia/rootfiles-v2/main/scripts/install.sh | sudo bash
#   curl -fsSL ... | sudo bash -s -- --version v0.1.0
#   curl -fsSL ... | sudo bash -s -- --channel dev
set -euo pipefail

REPO="entelecheia/rootfiles-v2"
INSTALL_DIR="/usr/local/bin"
BINARY="rootfiles"
VERSION=""
CHANNEL="stable"  # stable or dev

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version|-v)
            VERSION="$2"
            shift 2
            ;;
        --channel|-c)
            CHANNEL="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: install.sh [--version v0.1.0] [--channel stable|dev]"
            echo ""
            echo "Options:"
            echo "  --version, -v    Install a specific version (e.g., v0.1.0)"
            echo "  --channel, -c    Release channel: stable (default) or dev"
            echo ""
            echo "Channels:"
            echo "  stable    Latest tagged release (recommended)"
            echo "  dev       Latest commit on main (build from source)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        exit 1
        ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    echo "Error: rootfiles-v2 only supports Linux (got: $OS)"
    exit 1
fi

echo "rootfiles-v2 installer"
echo "  OS:   $OS"
echo "  Arch: $ARCH"

# --- Dev channel: build from source ---
if [ "$CHANNEL" = "dev" ]; then
    echo "  Channel: dev (building from source)"

    # Check Go is installed
    if ! command -v go &>/dev/null; then
        echo "Error: Go is required for dev channel. Install from https://go.dev/dl/"
        exit 1
    fi

    TMPDIR=$(mktemp -d)
    trap "rm -rf $TMPDIR" EXIT

    echo "Cloning repository..."
    git clone --depth 1 "https://github.com/${REPO}.git" "$TMPDIR/rootfiles-v2"

    echo "Building..."
    cd "$TMPDIR/rootfiles-v2"
    COMMIT=$(git rev-parse --short HEAD)
    go build -ldflags "-s -w -X main.version=dev-${COMMIT} -X main.commit=${COMMIT}" \
        -o "$INSTALL_DIR/$BINARY" ./cmd/rootfiles/

    echo ""
    echo "Installed: $INSTALL_DIR/$BINARY (dev-${COMMIT})"
    "$INSTALL_DIR/$BINARY" --version
    exit 0
fi

# --- Stable channel: download release binary ---
echo "  Channel: stable"

# Resolve version
if [ -z "$VERSION" ]; then
    echo "Fetching latest release..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest version."
        echo "Try: install.sh --channel dev"
        exit 1
    fi
fi

echo "  Version: $VERSION"

# Strip leading 'v' for archive name
VERSION_NUM="${VERSION#v}"
ARCHIVE="rootfiles_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

# Download
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "Downloading ${URL}..."
if ! curl -fsSL -o "$TMPDIR/$ARCHIVE" "$URL"; then
    echo "Error: download failed. Check that version $VERSION exists."
    echo "Available releases: https://github.com/${REPO}/releases"
    exit 1
fi

# Verify checksum
echo "Verifying checksum..."
curl -fsSL -o "$TMPDIR/checksums.txt" "$CHECKSUM_URL"
cd "$TMPDIR"
if command -v sha256sum &>/dev/null; then
    grep "$ARCHIVE" checksums.txt | sha256sum -c --quiet
elif command -v shasum &>/dev/null; then
    grep "$ARCHIVE" checksums.txt | shasum -a 256 -c --quiet
else
    echo "Warning: no checksum tool found, skipping verification"
fi

# Extract and install
echo "Installing to $INSTALL_DIR/$BINARY..."
tar xzf "$ARCHIVE"
install -m 755 "$BINARY" "$INSTALL_DIR/$BINARY"

echo ""
echo "Installed: $INSTALL_DIR/$BINARY"
"$INSTALL_DIR/$BINARY" --version

echo ""
echo "Next steps:"
echo "  sudo rootfiles apply              # interactive setup"
echo "  sudo rootfiles apply --profile dgx --yes  # unattended DGX setup"
