#!/bin/bash
# Family VPN Uninstall Script
# Removes all installed components for fresh reinstallation
#
# SAFETY FIRST: This script restores network routing BEFORE removing
# any VPN components to prevent leaving the machine without internet.
# Fully automated - reads sudo password from .env file

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
    export $(grep -v '^#' "$SCRIPT_DIR/.env" | xargs)
    echo -e "${GREEN}✓ Loaded configuration from .env${NC}"
else
    echo -e "${YELLOW}⚠️  No .env file found - sudo operations may require manual password entry${NC}"
fi

# Helper function to run sudo commands with password from .env
run_sudo() {
    if [ -n "$SUDO_PASSWORD" ]; then
        echo "$SUDO_PASSWORD" | sudo -S "$@" 2>/dev/null
    else
        sudo "$@"
    fi
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
