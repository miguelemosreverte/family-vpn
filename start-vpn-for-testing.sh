#!/bin/bash

echo "==================================="
echo "Starting VPN for Manual Testing"
echo "==================================="
echo ""
echo "Your IP before VPN: $(curl -s --max-time 5 ifconfig.me)"
echo ""
echo "Starting VPN client..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-running.log 2>&1 &
VPN_PID=$!

sleep 6

echo ""
echo "=== VPN Connection Log ==="
cat /tmp/vpn-running.log | head -10

echo ""
echo "=== Your IP Now ==="
NEW_IP=$(curl -s --max-time 10 ifconfig.me)
echo "$NEW_IP"

echo ""
if [ "$NEW_IP" = "95.217.238.72" ]; then
    echo "✅ VPN IS WORKING! IP changed to server."
else
    echo "⚠️  IP didn't change. Check logs above."
fi

echo ""
echo "==================================="
echo "VPN is running (PID: $VPN_PID)"
echo ""
echo "Test it yourself:"
echo "  1. Open browser → whatismyipaddress.com"
echo "  2. Should show Helsinki, Finland"
echo ""
echo "To stop VPN, run:"
echo "  echo 'osopanda' | sudo -S kill $VPN_PID"
echo "==================================="
