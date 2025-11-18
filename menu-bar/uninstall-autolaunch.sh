#!/bin/bash

# Family VPN Manager Auto-Launch Uninstaller
# This script removes the auto-launch configuration

set -e

echo "Family VPN Manager Auto-Launch Uninstaller"
echo "==========================================="
echo ""

LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
INSTALLED_PLIST="$LAUNCH_AGENTS_DIR/com.family.vpnmanager.plist"

# Check if launch agent is installed
if [ ! -f "$INSTALLED_PLIST" ]; then
    echo "Launch agent is not installed."
    echo "Nothing to do."
    exit 0
fi

# Unload the launch agent
echo "Unloading launch agent..."
launchctl unload "$INSTALLED_PLIST" 2>/dev/null || true

# Remove the plist file
echo "Removing launch agent file..."
rm "$INSTALLED_PLIST"

echo ""
echo "✅ Success! Family VPN Manager will no longer launch automatically on startup."
echo ""
echo "To re-enable auto-launch, run:"
echo "  ./install-autolaunch.sh"
echo ""
