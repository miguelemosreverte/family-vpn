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

    # Method 1: Try to get router from existing routes
    ROUTER=$(netstat -nr -f inet | grep default | grep -v utun | awk '{print $2}' | grep -E '^[0-9]+\.' | head -1)

    # Method 2: Try DHCP info
    if [ -z "$ROUTER" ] || [ "$ROUTER" == "link#" ]; then
        ROUTER=$(ipconfig getsummary $WIFI_INTERFACE 2>/dev/null | grep "router" | awk '{print $3}')
    fi

    # Method 3: Try getting it from route get
    if [ -z "$ROUTER" ] || [ "$ROUTER" == "link#" ]; then
        ROUTER=$(route -n get default 2>/dev/null | grep gateway | awk '{print $2}' | grep -E '^[0-9]+\.')
    fi

    # Method 4: Common router IPs as fallback
    if [ -z "$ROUTER" ] || [ "$ROUTER" == "link#" ]; then
        for TEST_IP in 192.168.0.1 192.168.1.1 192.168.1.254 10.0.0.1; do
            if ping -c 1 -W 1 $TEST_IP >/dev/null 2>&1; then
                ROUTER=$TEST_IP
                echo "  Found router via ping: $ROUTER"
                break
            fi
        done
    fi

    if [ -n "$ROUTER" ] && [ "$ROUTER" != "link#" ] && [[ "$ROUTER" =~ ^[0-9]+\. ]]; then
        echo "  Router: $ROUTER via $WIFI_INTERFACE"
        sudo route -n delete default 2>/dev/null
        sudo route -n add default $ROUTER 2>/dev/null && echo "  ✅ Added IPv4 default route to $ROUTER"
    else
        echo "  ⚠️  Could not find IPv4 router - trying Wi-Fi restart..."
        networksetup -setairportpower $WIFI_INTERFACE off
        sleep 2
        networksetup -setairportpower $WIFI_INTERFACE on
        echo "  ✅ Wi-Fi restarted"
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
