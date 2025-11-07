#!/bin/bash
echo "Testing VPN WITH encryption..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt > /tmp/vpn-enc.log 2>&1 &
VPN_PID=$!
sleep 8
cat /tmp/vpn-enc.log
echo ""
echo "=== IP with encryption (should be 95.217.238.72) ==="
curl -s --max-time 10 ifconfig.me
echo ""
echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null
sleep 2
