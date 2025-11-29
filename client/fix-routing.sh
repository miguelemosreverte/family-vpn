#!/bin/bash
# Fix routing if VPN crashed without cleanup

echo "🔧 Fixing routing table..."

if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    echo "Removing stale VPN routes..."

    # Delete any routes through utun interfaces
    for utun in $(ifconfig | grep "^utun" | cut -d: -f1); do
        echo "  Checking $utun..."
        sudo route -n delete default -ifscope $utun 2>/dev/null && echo "    ✅ Removed default route via $utun"
    done

    # Restore default route via Wi-Fi
    WIFI_INTERFACE=$(networksetup -listallhardwareports | awk '/Wi-Fi/{getline; print $2}')
    ROUTER=$(netstat -nr | grep default | grep $WIFI_INTERFACE | awk '{print $2}' | head -1)

    if [ -n "$ROUTER" ] && [ "$ROUTER" != "link#" ]; then
        echo "  Router: $ROUTER via $WIFI_INTERFACE"
    else
        # Try to find the router IP from DHCP
        ROUTER=$(ipconfig getsummary $WIFI_INTERFACE | grep -E "router|gateway" | awk '{print $NF}' | tr -d ' ')
        echo "  Router from DHCP: $ROUTER"
    fi

    if [ -n "$ROUTER" ] && [ "$ROUTER" != "link#" ]; then
        sudo route -n delete default 2>/dev/null
        sudo route -n add default $ROUTER 2>/dev/null && echo "  ✅ Added default route to $ROUTER"
    fi

    # Restore IPv6 if it was disabled
    echo "Restoring IPv6..."
    sudo networksetup -setv6automatic Wi-Fi && echo "  ✅ IPv6 enabled"

else
    # Linux
    echo "Removing stale VPN routes..."
    sudo ip route del default dev tun0 2>/dev/null && echo "  ✅ Removed default route via tun0"

    # Try to restore default route (you may need to adjust gateway)
    echo "  ⚠️  You may need to manually add default route:"
    echo "     sudo ip route add default via YOUR_GATEWAY"
fi

echo ""
echo "✅ Routing cleanup complete!"
echo "   Try: ping 8.8.8.8"
