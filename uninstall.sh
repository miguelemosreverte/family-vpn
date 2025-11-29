#!/bin/bash
# Family VPN Uninstall Script
# Removes all installed components for fresh reinstallation

echo "🗑️  Family VPN Uninstall Script"
echo "================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "📍 Working directory: $SCRIPT_DIR"
echo ""

# Stop running processes
echo "🛑 Stopping running processes..."
echo "--------------------------------"

pkill -f family-vpn-menubar 2>/dev/null && echo -e "${GREEN}✓ Stopped menu bar app${NC}" || echo -e "${YELLOW}• Menu bar not running${NC}"
pkill -f "Family VPN.app" 2>/dev/null && echo -e "${GREEN}✓ Stopped desktop app${NC}" || echo -e "${YELLOW}• Desktop app not running${NC}"
pkill -f vpn-client 2>/dev/null && echo -e "${GREEN}✓ Stopped VPN client${NC}" || echo -e "${YELLOW}• VPN client not running${NC}"

sleep 2

echo ""
echo "🗑️  Removing installed applications..."
echo "--------------------------------------"

# Remove desktop app
if [ -d "/Applications/Family VPN.app" ]; then
    sudo rm -rf "/Applications/Family VPN.app"
    echo -e "${GREEN}✓ Removed /Applications/Family VPN.app${NC}"
else
    echo -e "${YELLOW}• Desktop app not installed${NC}"
fi

# Remove menu bar app
if [ -f "/usr/local/bin/family-vpn-menubar" ]; then
    sudo rm -f "/usr/local/bin/family-vpn-menubar"
    echo -e "${GREEN}✓ Removed /usr/local/bin/family-vpn-menubar${NC}"
else
    echo -e "${YELLOW}• Menu bar app not installed${NC}"
fi

# Remove VPN client
if [ -f "/usr/local/bin/family-vpn-client" ]; then
    sudo rm -f "/usr/local/bin/family-vpn-client"
    echo -e "${GREEN}✓ Removed /usr/local/bin/family-vpn-client${NC}"
else
    echo -e "${YELLOW}• VPN client not installed${NC}"
fi

# Remove routing fix script
if [ -f "/usr/local/bin/family-vpn-fix-routing" ]; then
    sudo rm -f "/usr/local/bin/family-vpn-fix-routing"
    echo -e "${GREEN}✓ Removed /usr/local/bin/family-vpn-fix-routing${NC}"
else
    echo -e "${YELLOW}• Routing fix script not installed${NC}"
fi

echo ""
echo "🧹 Cleaning build artifacts..."
echo "------------------------------"

# Remove built binaries (optional - for fresh build)
if [ "$1" == "--clean" ] || [ "$1" == "-c" ]; then
    echo "Cleaning build artifacts..."

    # Remove VPN client binary
    if [ -f "$SCRIPT_DIR/client/vpn-client" ]; then
        rm -f "$SCRIPT_DIR/client/vpn-client"
        echo -e "${GREEN}✓ Removed client/vpn-client${NC}"
    fi

    # Remove menu bar binary
    if [ -f "$SCRIPT_DIR/menu-bar/family-vpn-menubar" ]; then
        rm -f "$SCRIPT_DIR/menu-bar/family-vpn-menubar"
        echo -e "${GREEN}✓ Removed menu-bar/family-vpn-menubar${NC}"
    fi

    # Remove desktop app build
    if [ -d "$SCRIPT_DIR/desktop-app/dist" ]; then
        rm -rf "$SCRIPT_DIR/desktop-app/dist"
        echo -e "${GREEN}✓ Removed desktop-app/dist/${NC}"
    fi

    # Remove node_modules (optional, takes long to reinstall)
    if [ "$1" == "--clean-all" ]; then
        if [ -d "$SCRIPT_DIR/desktop-app/node_modules" ]; then
            rm -rf "$SCRIPT_DIR/desktop-app/node_modules"
            echo -e "${GREEN}✓ Removed desktop-app/node_modules/${NC}"
        fi
    fi

    echo -e "${GREEN}✓ Build artifacts cleaned${NC}"
else
    echo -e "${YELLOW}• Keeping build artifacts (use --clean to remove)${NC}"
fi

echo ""
echo "✅ Uninstall Complete!"
echo "======================"
echo ""
echo "To reinstall:"
echo "  ./install.sh"
echo ""
echo "To clean build artifacts and reinstall fresh:"
echo "  ./uninstall.sh --clean && ./install.sh"
echo ""
