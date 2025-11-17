#!/bin/bash
echo "Starting VPN..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/simple-vpn.log 2>&1 &
VPN_PID=$!

echo "Waiting for VPN to establish..."
sleep 5

echo "Testing simple HTTP request..."
curl -v --max-time 10 http://example.com 2>&1 | head -20

echo ""
echo "Testing throughput download..."
time curl -s --max-time 15 -w 'Speed: %{speed_download} bytes/sec\n' -o /tmp/test-download.bin http://speedtest.tele2.net/100KB.zip

echo ""
echo "Download result:"
ls -lh /tmp/test-download.bin 2>/dev/null || echo "Download failed"

echo ""
echo "Killing VPN..."
sudo pkill -INT vpn-client 2>/dev/null || true
wait $VPN_PID 2>/dev/null || true

echo "VPN log:"
tail -10 /tmp/simple-vpn.log
