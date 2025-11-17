#!/bin/bash
# Quick bottleneck analysis script

set -euo pipefail

echo "=== VPN Bottleneck Analysis ==="
echo ""

# Start VPN in background
echo "→ Starting VPN..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt=false > /tmp/vpn-measure.log 2>&1 &
VPN_PID=$!
sleep 5

echo "→ Testing with different methods..."
echo ""

# Test 1: Small packets (ping)
echo "1. Small packets (ICMP):"
ping -c 10 -i 0.2 8.8.8.8 | grep "bytes from" | wc -l
echo ""

# Test 2: HTTP small file
echo "2. HTTP 1KB download:"
time curl -o /dev/null -s http://speedtest.tele2.net/1KB.zip
echo ""

# Test 3: Monitor packet rate
echo "3. Packet rate test (5 seconds):"
sudo tcpdump -i any -n host 95.217.238.72 -c 100 2>&1 | grep "packets captured" &
TCPDUMP_PID=$!
sleep 5
sudo pkill tcpdump 2>/dev/null || true
echo ""

# Test 4: Check CPU usage
echo "4. VPN client CPU usage:"
ps aux | grep vpn-client | grep -v grep | awk '{print $3"%"}'
echo ""

# Stop VPN
echo "osopanda" | sudo -S pkill -INT vpn-client 2>/dev/null || true
wait $VPN_PID 2>/dev/null || true

echo "→ Analysis complete. Check /tmp/vpn-measure.log for details"
