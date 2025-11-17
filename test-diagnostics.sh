#!/bin/bash
# Quick diagnostic test

echo "Starting VPN with diagnostics..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt=false > /tmp/vpn-diagnostics.log 2>&1 &
VPN_PID=$!

sleep 6
echo "Running iperf3 for 10 seconds..."
iperf3 -c 10.8.0.1 -t 10 > /dev/null 2>&1 &

sleep 12
echo "Stopping VPN..."
echo "osopanda" | sudo -S pkill vpn-client

sleep 1
echo ""
echo "=== DIAGNOSTIC OUTPUT ==="
tail -30 /tmp/vpn-diagnostics.log
