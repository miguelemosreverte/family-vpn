#!/bin/bash
# Emergency VPN Cleanup Script
# Use this if VPN client died and left your network in broken state

echo "🚨 VPN Emergency Cleanup"
echo "========================"

# Kill VPN client
echo ""
echo "1. Killing VPN client process..."
sudo pkill -9 vpn-client 2>/dev/null && echo "   ✓ VPN client killed" || echo "   - No VPN client running"

# Check and fix routing
echo ""
echo "2. Checking routing..."
CURRENT_DEFAULT=$(netstat -rn | grep "^default" | grep -v "utun" | head -1 | awk '{print $2}')
echo "   Current default gateway: $CURRENT_DEFAULT"

if [[ "$CURRENT_DEFAULT" == "10.8.0.1" ]] || [[ "$CURRENT_DEFAULT" == *"utun"* ]]; then
    echo "   ⚠ Routing is broken! Fixing..."
    
    # Delete broken VPN route
    sudo route -n delete default 2>/dev/null
    
    # Try to restore to common router IPs
    echo "   Trying common router IPs..."
    for ROUTER_IP in 192.168.100.1 192.168.1.1 192.168.0.1 10.0.0.1; do
        if ping -c 1 -W 1 $ROUTER_IP >/dev/null 2>&1; then
            echo "   Found router at: $ROUTER_IP"
            sudo route -n add -net default $ROUTER_IP
            echo "   ✓ Default route restored"
            break
        fi
    done
else
    echo "   ✓ Routing looks OK"
fi

# Fix DNS
echo ""
echo "3. Restoring DNS to automatic..."
sudo networksetup -setdnsservers Wi-Fi Empty 2>/dev/null && echo "   ✓ DNS restored" || echo "   ⚠ Failed to restore DNS"

# Cleanup TUN interface if stuck
echo ""
echo "4. Checking TUN interfaces..."
UTUN_DEVICES=$(ifconfig | grep "^utun" | cut -d: -f1)
if [[ -n "$UTUN_DEVICES" ]]; then
    echo "   Found TUN devices (normal, system managed):"
    echo "$UTUN_DEVICES" | sed 's/^/     /'
else
    echo "   ✓ No TUN devices"
fi

# Verify connectivity
echo ""
echo "5. Testing connectivity..."
if ping -c 2 8.8.8.8 >/dev/null 2>&1; then
    echo "   ✓ Internet connectivity: OK"
    
    # Check real IP
    REAL_IP=$(curl -s --max-time 3 ifconfig.me)
    if [[ -n "$REAL_IP" ]]; then
        echo "   ✓ Your IP: $REAL_IP"
    fi
else
    echo "   ✗ Internet still not working!"
    echo ""
    echo "Manual steps needed:"
    echo "  1. Open System Settings → Network"
    echo "  2. Check your Wi-Fi/Ethernet connection"
    echo "  3. May need to turn Wi-Fi off/on"
fi

# Final status
echo ""
echo "========================"
echo "✓ Cleanup complete!"
echo ""
echo "If internet still doesn't work:"
echo "  • Restart Wi-Fi: networksetup -setairportpower en0 off && sleep 2 && networksetup -setairportpower en0 on"
echo "  • Check System Settings → Network"
echo "  • Worst case: Restart computer"
