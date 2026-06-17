#!/bin/bash
# Remote Management - macOS Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/crpr777/Brew-Developer-Test/main/install.sh | sudo bash
#
# Downloads the latest .pkg from GitHub Releases and installs it.
# Because the file is fetched via curl (not a browser), macOS does NOT
# set the quarantine attribute — no Gatekeeper "Open Anyway" prompt.
set -e

REPO="crpr777/Brew-Developer-Test"
APP_NAME="RemoteMgmt"
PKG_ID="com.remotemgmt.client"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[*]${NC} $1"; }
ok()    { echo -e "${GREEN}[✓]${NC} $1"; }
err()   { echo -e "${RED}[✗]${NC} $1"; exit 1; }

# Must run as root (installer requires it)
[ "$(id -u)" -eq 0 ] || err "This script must be run with sudo"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    arm64)  PKG_SUFFIX="darwin-arm64" ;;
    x86_64) PKG_SUFFIX="darwin-amd64" ;;
    *)      err "Unsupported architecture: $ARCH" ;;
esac

info "Detected architecture: $ARCH ($PKG_SUFFIX)"

# Get latest release download URL from GitHub
info "Fetching latest release from github.com/$REPO..."
RELEASE_URL=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep "browser_download_url.*${PKG_SUFFIX}\.pkg" \
    | head -1 \
    | cut -d '"' -f 4)

[ -n "$RELEASE_URL" ] || err "No .pkg found for $PKG_SUFFIX in latest release"

PKG_FILE=$(basename "$RELEASE_URL")
TMP_DIR=$(mktemp -d /tmp/rmgmt-install-XXXXXX)
TMP_PKG="$TMP_DIR/$PKG_FILE"

info "Downloading $PKG_FILE..."
curl -fsSL -o "$TMP_PKG" "$RELEASE_URL" || err "Download failed"

ok "Downloaded $(du -h "$TMP_PKG" | cut -f1 | xargs) to $TMP_PKG"

# Install the .pkg (no quarantine = no Gatekeeper prompt)
info "Installing..."
installer -pkg "$TMP_PKG" -target / || err "Installation failed"

ok "Installed $APP_NAME.app to /Applications/"

# Cleanup
rm -rf "$TMP_DIR"

ok "Installation complete!"
echo ""
info "The agent is now running. Grant permissions when prompted:"
info "  System Settings → Privacy & Security → Screen Recording"
info "  System Settings → Privacy & Security → Accessibility"
echo ""
info "Look for 'Remote Management' in each list and enable it."
