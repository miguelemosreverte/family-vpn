#!/bin/bash
# Fix routing if VPN crashed without cleanup
#
# DESIGN PRINCIPLE: This script should work WITHOUT sudo when possible
# The Wi-Fi restart method works without sudo and properly restores DHCP routing

echo "🔧 Fixing routing table..."

if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    WIFI_INTERFACE=$(networksetup -listallhardwareports | awk '/Wi-Fi/{getline; print $2}')

    echo "Checking network connectivity..."

    # First, check if we already have internet
    if ping -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; then
        echo "✅ Internet already working!"
        exit 0
    fi

    echo "⚠️  No internet connectivity detected"

    # ==========================================================================
    # METHOD 1: Wi-Fi Restart (NO SUDO REQUIRED - PREFERRED)
    # This is the most reliable method as it forces DHCP renewal
    # ==========================================================================
    echo ""
    echo "Method 1: Restarting Wi-Fi (no sudo required)..."

    # Turn Wi-Fi off
    networksetup -setairportpower "$WIFI_INTERFACE" off 2>/dev/null
    sleep 2

    # Turn Wi-Fi back on
    networksetup -setairportpower "$WIFI_INTERFACE" on 2>/dev/null

    # Wait for connection to establish
    echo "  Waiting for Wi-Fi to reconnect..."
    for i in {1..10}; do
        sleep 1
        if ping -c 1 -W 1 8.8.8.8 >/dev/null 2>&1; then
            echo "  ✅ Internet restored after Wi-Fi restart!"

            # Also restore IPv6 (doesn't need sudo)
            networksetup -setv6automatic Wi-Fi 2>/dev/null
            echo "  ✅ IPv6 enabled"

            echo ""
            echo "✅ Routing cleanup complete!"
            exit 0
        fi
        echo "  ... waiting ($i/10)"
    done

    echo "  ⚠️  Wi-Fi restart didn't restore connectivity"

    # ==========================================================================
    # METHOD 2: Manual Route Fix (REQUIRES SUDO)
    # Only try this if Wi-Fi restart failed
    # ==========================================================================
    echo ""
    echo "Method 2: Manual route fix (requires sudo)..."

    # Try to find router IP from DHCP info (doesn't need sudo)
    ROUTER=$(ipconfig getsummary "$WIFI_INTERFACE" 2>/dev/null | grep "router" | awk '{print $3}')

    # Fallback: try common router IPs
    if [ -z "$ROUTER" ]; then
        for TEST_IP in 192.168.0.1 192.168.1.1 192.168.1.254 10.0.0.1; do
            # Can ping on local network without default route
            if ping -c 1 -W 1 $TEST_IP >/dev/null 2>&1; then
                ROUTER=$TEST_IP
                echo "  Found router via ping: $ROUTER"
                break
            fi
        done
    fi

    if [ -n "$ROUTER" ] && [[ "$ROUTER" =~ ^[0-9]+\. ]]; then
        echo "  Router: $ROUTER"

        # Remove any stale routes through utun interfaces
        for utun in $(ifconfig 2>/dev/null | grep "^utun" | cut -d: -f1); do
            sudo route -n delete default -ifscope $utun 2>/dev/null
        done

        # Remove VPN server specific routes
        VPN_SERVER_ROUTES=$(netstat -rn -f inet 2>/dev/null | grep -E "^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\s+192\.168\." | awk '{print $1}')
        for route in $VPN_SERVER_ROUTES; do
            sudo route -n delete "$route" 2>/dev/null && echo "  Removed VPN route: $route"
        done

        # Delete and re-add default route
        sudo route -n delete default 2>/dev/null
        sudo route -n add default "$ROUTER" 2>/dev/null && echo "  ✅ Added default route via $ROUTER"

        # Restore IPv6
        sudo networksetup -setv6automatic Wi-Fi 2>/dev/null && echo "  ✅ IPv6 enabled"
    else
        echo "  ⚠️  Could not find router IP"
        echo "  Try manually: sudo route add default YOUR_ROUTER_IP"
    fi

else
    # Linux
    echo "Removing stale VPN routes..."

    # Check if we have internet
    if ping -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; then
        echo "✅ Internet already working!"
        exit 0
    fi

    # Remove tun0 route
    sudo ip route del default dev tun0 2>/dev/null && echo "  ✅ Removed default route via tun0"

    # Try to get gateway from DHCP
    GATEWAY=$(ip route | grep -v tun | grep default | awk '{print $3}' | head -1)

    if [ -n "$GATEWAY" ]; then
        echo "  Adding default route via $GATEWAY"
        sudo ip route add default via "$GATEWAY"
    else
        echo "  ⚠️  You may need to manually add default route:"
        echo "     sudo ip route add default via YOUR_GATEWAY"
    fi
fi

echo ""
echo "Testing connectivity..."
if ping -c 1 -W 2 8.8.8.8 >/dev/null 2>&1; then
    echo "✅ Routing cleanup complete! Internet working."
else
    echo "⚠️  Internet still not working. Manual intervention may be required."
    echo "   Try: networksetup -setairportpower en0 off && sleep 2 && networksetup -setairportpower en0 on"
fi
