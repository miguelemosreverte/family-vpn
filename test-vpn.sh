#!/bin/bash

echo "VPN Test Script"
echo "==============="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (sudo ./test-vpn.sh)"
    exit 1
fi

# Check current IP
echo "1. Current IP before VPN:"
ORIGINAL_IP=$(curl -s --max-time 5 ifconfig.me)
echo "   $ORIGINAL_IP"
echo ""

# Start VPN client in background
echo "2. Starting VPN client (no encryption)..."
./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-test.log 2>&1 &
VPN_PID=$!
echo "   VPN PID: $VPN_PID"
echo ""

# Wait for connection
echo "3. Waiting for VPN to establish (10 seconds)..."
sleep 10

# Check if VPN is still running
if ! ps -p $VPN_PID > /dev/null; then
    echo "   ERROR: VPN client died. Logs:"
    cat /tmp/vpn-test.log
    exit 1
fi

# Check new IP
echo "4. Checking IP through VPN:"
VPN_IP=$(curl -s --max-time 10 ifconfig.me)
echo "   $VPN_IP"
echo ""

# Verify IP changed
if [ "$VPN_IP" == "95.217.238.72" ]; then
    echo "✓ SUCCESS: Traffic is routing through VPN server!"
    echo ""

    # Show some stats
    echo "5. VPN Status:"
    echo "   TUN interface:"
    ip addr show tun0 2>/dev/null | grep -E "inet |tun0:" || echo "   Could not find tun0"
    echo ""
    echo "   Routing table (VPN routes):"
    ip route | grep tun0 || echo "   No tun0 routes"
    echo ""
else
    echo "✗ FAILED: IP did not change to VPN server IP"
    echo "   Expected: 95.217.238.72"
    echo "   Got: $VPN_IP"
    echo ""
fi

echo "6. VPN client logs:"
echo "---"
cat /tmp/vpn-test.log
echo "---"
echo ""

# Ask user to stop VPN
echo "VPN is still running (PID: $VPN_PID)"
echo "Press Enter to stop VPN and restore routing..."
read

# Kill VPN client
echo "Stopping VPN..."
kill -SIGINT $VPN_PID 2>/dev/null
sleep 3

# Force kill if still running
if ps -p $VPN_PID > /dev/null 2>&1; then
    kill -9 $VPN_PID 2>/dev/null
fi

echo "Waiting for cleanup..."
sleep 2

# Verify IP restored
echo ""
echo "7. IP after VPN disconnect:"
RESTORED_IP=$(curl -s --max-time 5 ifconfig.me)
echo "   $RESTORED_IP"

if [ "$RESTORED_IP" == "$ORIGINAL_IP" ]; then
    echo "✓ SUCCESS: Routing restored to original IP"
else
    echo "⚠ WARNING: IP changed (may need to manually restore routing)"
fi

echo ""
echo "Test complete!"
