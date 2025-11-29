#!/bin/bash
# Family VPN Uninstall Script
# Removes all installed components for fresh reinstallation
#
# SAFETY FIRST: This script restores network routing BEFORE removing
# any VPN components to prevent leaving the machine without internet.
# Fully automated - reads sudo password from .env file

# Set up PATH for non-interactive SSH sessions (Homebrew on macOS)
if [[ "$(uname -s)" == "Darwin" ]]; then
    # Add Homebrew paths for both Intel and Apple Silicon
    export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
fi

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

# Load environment variables from .env file
if [ -f "$SCRIPT_DIR/.env" ]; then
    # Only export valid KEY=VALUE lines, skip comments and empty lines
    while IFS='=' read -r key value; do
        # Skip empty lines, comments, and lines without =
        if [[ -n "$key" && ! "$key" =~ ^# && -n "$value" ]]; then
            # Remove leading/trailing whitespace from key
            key=$(echo "$key" | xargs)
            # Only export if key is a valid variable name
            if [[ "$key" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
                export "$key=$value"
            fi
        fi
    done < "$SCRIPT_DIR/.env"
    echo -e "${GREEN}✓ Loaded configuration from .env${NC}"
else
    echo -e "${YELLOW}⚠️  No .env file found - sudo operations may require manual password entry${NC}"
fi

# Helper function to run sudo commands with password from .env
# Uses expect as fallback when sudo -S fails (e.g., in sandboxed environments like Claude Code)
run_sudo() {
    local cmd="$*"

    # Method 1: Try piped password (works in Terminal and SSH)
    if [ -n "$SUDO_PASSWORD" ]; then
        if echo "$SUDO_PASSWORD" | sudo -S "$@" 2>/dev/null; then
            return 0
        fi
    fi

    # Method 2: Use expect to handle sudo (works in sandboxed environments)
    if [ -n "$SUDO_PASSWORD" ] && command -v expect &> /dev/null; then
        expect -c "
            spawn sudo $cmd
            expect \"Password:\"
            send \"$SUDO_PASSWORD\r\"
            expect eof
        " 2>/dev/null && return 0
    fi

    # No interactive fallback - automation requires SUDO_PASSWORD in .env
    echo -e "${RED}❌ sudo failed - ensure SUDO_PASSWORD is set in .env${NC}"
    return 1
}

echo ""

# =============================================================================
# STEP 1: RESTORE NETWORK ROUTING FIRST (SAFETY CRITICAL)
# =============================================================================
echo "🛡️  Restoring network routing (safety first)..."
echo "------------------------------------------------"

restore_routing() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS - Get the Wi-Fi interface
        WIFI_INTERFACE=$(networksetup -listallhardwareports | awk '/Wi-Fi/{getline; print $2}')

        # Remove any VPN-specific routes (like route to VPN server)
        # These are routes that go through the normal gateway to reach VPN server
        VPN_SERVER_ROUTES=$(netstat -rn -f inet | grep -E "^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\s+192\.168\." | awk '{print $1}')
        for route in $VPN_SERVER_ROUTES; do
            run_sudo route -n delete "$route" && echo -e "${GREEN}✓ Removed VPN server route: $route${NC}"
        done

        # Remove any routes through utun interfaces
        for utun in $(ifconfig 2>/dev/null | grep "^utun" | cut -d: -f1); do
            run_sudo route -n delete default -ifscope $utun && echo -e "${GREEN}✓ Removed default route via $utun${NC}"
        done

        # Check if we have a working default route
        if ! route -n get default 2>/dev/null | grep -q "gateway:"; then
            echo -e "${YELLOW}⚠️  No default route found, restoring...${NC}"

            # Try to find the router
            ROUTER=""

            # Method 1: Get from DHCP
            ROUTER=$(ipconfig getsummary "$WIFI_INTERFACE" 2>/dev/null | grep "router" | awk '{print $3}')

            # Method 2: Common router IPs
            if [ -z "$ROUTER" ]; then
                for TEST_IP in 192.168.0.1 192.168.1.1 192.168.1.254 10.0.0.1; do
                    if ping -c 1 -W 1 $TEST_IP >/dev/null 2>&1; then
                        ROUTER=$TEST_IP
                        break
                    fi
                done
            fi

            if [ -n "$ROUTER" ]; then
                run_sudo route -n add default "$ROUTER" && echo -e "${GREEN}✓ Restored default route via $ROUTER${NC}"
            else
                # Last resort: restart Wi-Fi to get fresh DHCP
                echo -e "${YELLOW}⚠️  Restarting Wi-Fi to restore routing...${NC}"
                networksetup -setairportpower "$WIFI_INTERFACE" off 2>/dev/null
                sleep 2
                networksetup -setairportpower "$WIFI_INTERFACE" on 2>/dev/null
                sleep 3
                echo -e "${GREEN}✓ Wi-Fi restarted${NC}"
            fi
        else
            echo -e "${GREEN}✓ Default route already exists${NC}"
        fi

        # Restore IPv6
        run_sudo networksetup -setv6automatic Wi-Fi && echo -e "${GREEN}✓ IPv6 restored${NC}"

    else
        # Linux
        run_sudo ip route del default dev tun0
        echo -e "${GREEN}✓ Removed any tun0 default route${NC}"
    fi
}

restore_routing

# Verify internet connectivity before proceeding
echo ""
echo "🔍 Verifying internet connectivity..."
if ping -c 1 -W 3 8.8.8.8 >/dev/null 2>&1; then
    echo -e "${GREEN}✓ Internet connectivity confirmed${NC}"
else
    echo -e "${YELLOW}⚠️  Internet may be limited, but proceeding with uninstall${NC}"
fi

echo ""

# =============================================================================
# STEP 2: STOP RUNNING PROCESSES
# =============================================================================
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
    run_sudo rm -rf "/Applications/Family VPN.app"
    echo -e "${GREEN}✓ Removed /Applications/Family VPN.app${NC}"
else
    echo -e "${YELLOW}• Desktop app not installed${NC}"
fi

# Remove menu bar app
if [ -f "/usr/local/bin/family-vpn-menubar" ]; then
    run_sudo rm -f "/usr/local/bin/family-vpn-menubar"
    echo -e "${GREEN}✓ Removed /usr/local/bin/family-vpn-menubar${NC}"
else
    echo -e "${YELLOW}• Menu bar app not installed${NC}"
fi

# Remove VPN client
if [ -f "/usr/local/bin/family-vpn-client" ]; then
    run_sudo rm -f "/usr/local/bin/family-vpn-client"
    echo -e "${GREEN}✓ Removed /usr/local/bin/family-vpn-client${NC}"
else
    echo -e "${YELLOW}• VPN client not installed${NC}"
fi

# Remove routing fix script
if [ -f "/usr/local/bin/family-vpn-fix-routing" ]; then
    run_sudo rm -f "/usr/local/bin/family-vpn-fix-routing"
    echo -e "${GREEN}✓ Removed /usr/local/bin/family-vpn-fix-routing${NC}"
else
    echo -e "${YELLOW}• Routing fix script not installed${NC}"
fi

echo ""
echo "🧹 Cleaning build artifacts..."
echo "------------------------------"

# Always clean build artifacts for a fresh reinstall
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

# Remove watchdog binary
if [ -f "$SCRIPT_DIR/watchdog/family-vpn-watchdog" ]; then
    rm -f "$SCRIPT_DIR/watchdog/family-vpn-watchdog"
    echo -e "${GREEN}✓ Removed watchdog/family-vpn-watchdog${NC}"
fi

# Remove desktop app build
if [ -d "$SCRIPT_DIR/desktop-app/dist" ]; then
    rm -rf "$SCRIPT_DIR/desktop-app/dist"
    echo -e "${GREEN}✓ Removed desktop-app/dist/${NC}"
fi

echo -e "${GREEN}✓ Build artifacts cleaned${NC}"

echo ""
echo "🎯 Removing from Dock and Login Items..."
echo "----------------------------------------"

# Remove from Dock
if command -v dockutil &> /dev/null; then
    dockutil --remove "Family VPN" --no-restart 2>/dev/null && echo -e "${GREEN}✓ Removed from Dock${NC}" || echo -e "${YELLOW}• Not in Dock${NC}"
    killall Dock 2>/dev/null
else
    # Use Python to remove dock entries
    python3 << 'DOCK_CLEANUP'
import subprocess
import os

dock_plist = f"{os.environ['HOME']}/Library/Preferences/com.apple.dock.plist"

# Convert binary plist to XML for processing
subprocess.run(['plutil', '-convert', 'xml1', dock_plist], capture_output=True)

# Read, filter, and write back
with open(dock_plist, 'r') as f:
    content = f.read()

# Simple approach: if the plist contains Family VPN, we need manual cleanup
if 'Family VPN' in content:
    print("⚠️  Family VPN found in Dock - please remove manually from System Preferences")
    print("   System Preferences → Dock & Menu Bar → Remove from Dock")

# Convert back to binary
subprocess.run(['plutil', '-convert', 'binary1', dock_plist], capture_output=True)
DOCK_CLEANUP
fi

# Remove from Login Items
osascript -e 'tell application "System Events" to delete login item "Family VPN"' 2>/dev/null && \
    echo -e "${GREEN}✓ Removed from Login Items${NC}" || \
    echo -e "${YELLOW}• Not in Login Items${NC}"

# Unload watchdog service
LAUNCHD_PLIST="$HOME/Library/LaunchAgents/com.family.vpn.watchdog.plist"
if [ -f "$LAUNCHD_PLIST" ]; then
    launchctl unload "$LAUNCHD_PLIST" 2>/dev/null
    rm -f "$LAUNCHD_PLIST"
    echo -e "${GREEN}✓ Removed watchdog service${NC}"
else
    echo -e "${YELLOW}• Watchdog service not installed${NC}"
fi

# Also check for old menubar service
OLD_MENUBAR_PLIST="$HOME/Library/LaunchAgents/com.family.vpnmanager.plist"
if [ -f "$OLD_MENUBAR_PLIST" ]; then
    launchctl unload "$OLD_MENUBAR_PLIST" 2>/dev/null
    rm -f "$OLD_MENUBAR_PLIST"
    echo -e "${GREEN}✓ Removed old menubar service${NC}"
fi

echo ""
echo "✅ Uninstall Complete!"
echo "======================"
echo ""
echo "To reinstall, run:"
echo "  ./install.sh"
echo ""
