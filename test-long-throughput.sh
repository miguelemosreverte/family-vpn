#!/bin/bash
# Longer throughput test to allow TCP slow start

echo "Starting VPN with diagnostics..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt=false > /tmp/vpn-long-test.log 2>&1 &

sleep 6
echo "Running iperf3 for 30 seconds..."
iperf3 -c 10.8.0.1 -t 30

sleep 2
echo "osopanda" | sudo -S pkill vpn-client

echo ""
echo "=== DIAGNOSTIC OUTPUT ==="
grep EGRESS /tmp/vpn-long-test.log
