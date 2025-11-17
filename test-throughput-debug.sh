#!/bin/bash
set -x

echo "Starting VPN..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-debug.log 2>&1 &
VPN_PID=$!

echo "Waiting for VPN to establish..."
sleep 5

echo "Testing throughput..."
output=$(curl -s --max-time 5 -w '%{speed_download}' -o /dev/null http://speedtest.tele2.net/100KB.zip 2>&1 || echo "0")

echo "Raw output: '$output'"

if [[ "$output" == "0" ]] || [[ -z "$output" ]]; then
    echo "FAILED"
else
    mbps=$(echo "scale=2; $output * 8 / 1000000" | bc)
    echo "Throughput: $mbps Mbps"
fi

echo "Stopping VPN..."
sudo kill -INT $(pgrep -P $VPN_PID) 2>/dev/null || true
wait $VPN_PID 2>/dev/null || true

echo "Done"
