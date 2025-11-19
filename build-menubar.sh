#!/bin/bash

# Family VPN - Menu Bar Application Builder
# Builds both the VPN client and the menu bar application

set -e

echo "Family VPN - Menu Bar Application Builder"
echo "=========================================="
echo ""

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Build VPN client
echo "Building VPN client..."
cd "$SCRIPT_DIR/client"
go build -o vpn-client .
echo "✅ VPN client built successfully"
echo ""

# Build menu bar application
echo "Building menu bar application..."
cd "$SCRIPT_DIR/menu-bar"
go build -o family-vpn-menubar .
echo "✅ Menu bar application built successfully"
echo ""

# Build extensions
echo "Building extensions..."

# Video extension
cd "$SCRIPT_DIR/extensions/video"
if [ -f "main.go" ]; then
    go build -o video-extension .
    echo "✅ Video extension built successfully"
else
    echo "⚠️  Video extension source not found (skipping)"
fi

# SSH extension
cd "$SCRIPT_DIR/extensions/ssh"
if [ -f "main.go" ]; then
    go build -o ssh-extension .
    echo "✅ SSH extension built successfully"
else
    echo "⚠️  SSH extension source not found (skipping)"
fi

echo ""

echo "=========================================="
echo "Build complete!"
echo ""
echo "To run the menu bar app:"
echo "  cd menu-bar"
echo "  ./family-vpn-menubar"
echo ""
echo "To install auto-launch (starts on login):"
echo "  cd menu-bar"
echo "  ./install-autolaunch.sh"
echo ""
