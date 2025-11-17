#!/bin/bash
# Test timing under sustained iperf3 load

echo "Starting VPN with timing instrumentation..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt=false > /tmp/vpn-timing-iperf.log 2>&1 &

sleep 3
echo "Starting iperf3 for 30 seconds..."
iperf3 -c 10.8.0.1 -t 30 > /dev/null 2>&1

echo "Waiting for final stats..."
sleep 3

echo "Stopping VPN..."
echo "osopanda" | sudo -S pkill vpn-client

sleep 1
echo ""
echo "=== TIMING RESULTS (during iperf3 load) ===="
grep -E "EGRESS|TIMING" /tmp/vpn-timing-iperf.log | grep -v "0.00 Mbps"
