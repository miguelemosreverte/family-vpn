#!/bin/bash
echo "Starting VPN..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-direct-curl.log 2>&1 &
sleep 5

echo "Test 1: Direct curl (no capture)"
curl -s --max-time 15 -w 'Speed: %{speed_download} bytes/sec\n' -o /dev/null http://speedtest.tele2.net/100KB.zip
echo "Exit code: $?"

echo ""
echo "Test 2: Captured in variable"
output=$(curl -s --max-time 15 -w '%{speed_download}' -o /dev/null http://speedtest.tele2.net/100KB.zip 2>&1)
echo "Exit code: $?"
echo "Output: '$output'"

sudo pkill -INT vpn-client 2>/dev/null || true
sleep 2
