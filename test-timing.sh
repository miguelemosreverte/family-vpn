#!/bin/bash
# Test with timing instrumentation

echo "Starting VPN with timing instrumentation..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt=false > /tmp/vpn-timing-test.log 2>&1 &

sleep 6
echo "Running iperf3..."
iperf3 -c 10.8.0.1 -t 15 > /dev/null 2>&1 &

sleep 17
echo "Stopping VPN..."
echo "osopanda" | sudo -S pkill vpn-client

sleep 1
echo ""
echo "=== TIMING RESULTS ==="
grep -E "EGRESS|TIMING" /tmp/vpn-timing-test.log
