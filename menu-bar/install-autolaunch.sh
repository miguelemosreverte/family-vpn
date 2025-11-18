#!/bin/bash

# Family VPN Manager Auto-Launch Installer
# This script sets up the VPN Manager to launch automatically on system startup

set -e

echo "Family VPN Manager Auto-Launch Installer"
echo "========================================="
echo ""

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
BINARY_PATH="$SCRIPT_DIR/family-vpn-menubar"
PLIST_FILE="$SCRIPT_DIR/com.family.vpnmanager.plist"
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
INSTALLED_PLIST="$LAUNCH_AGENTS_DIR/com.family.vpnmanager.plist"

# Check if binary exists
if [ ! -f "$BINARY_PATH" ]; then
    echo "Error: Family VPN Manager binary not found at $BINARY_PATH"
    echo "Please build the app first with: cd menu-bar && go build -o family-vpn-menubar"
    exit 1
fi

# Check if plist file exists
if [ ! -f "$PLIST_FILE" ]; then
    echo "Error: Launch agent plist file not found"
    exit 1
fi

# Create LaunchAgents directory if it doesn't exist
mkdir -p "$LAUNCH_AGENTS_DIR"

# Unload existing launch agent if it exists
if [ -f "$INSTALLED_PLIST" ]; then
    echo "Unloading existing launch agent..."
    launchctl unload "$INSTALLED_PLIST" 2>/dev/null || true
fi

# Copy plist file to LaunchAgents
echo "Installing launch agent..."
cp "$PLIST_FILE" "$INSTALLED_PLIST"

# Load the launch agent
echo "Loading launch agent..."
launchctl load "$INSTALLED_PLIST"

echo ""
echo "✅ Success! Family VPN Manager will now launch automatically on startup."
echo ""
echo "The VPN will auto-connect 2 seconds after the app launches."
echo ""
echo "To test: Log out and log back in, or run:"
echo "  launchctl start com.family.vpnmanager"
echo ""
echo "To uninstall auto-launch, run:"
echo "  ./uninstall-autolaunch.sh"
echo ""
