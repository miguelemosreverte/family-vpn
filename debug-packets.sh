#!/bin/bash

# Quick test to see packet sizes
echo "Starting VPN with encryption and checking packet flow..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt > /tmp/debug.log 2>&1 &
VPN_PID=$!
sleep 3

echo "Client log:"
cat /tmp/debug.log

echo ""
echo "Server log:"
ssh -i ~/.ssh/id_ed25519_hetzner root@95.217.238.72 "tail -20 /var/log/vpn-server.log | tail -10"

echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null
