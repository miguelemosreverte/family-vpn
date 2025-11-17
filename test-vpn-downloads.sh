#!/bin/bash
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/downloads-test.log 2>&1 &
sleep 5

echo "=== Testing DNS ==="
nslookup example.com | head -10

echo ""
echo "=== Testing small HTTP request ==="
curl -s --max-time 5 -w 'Time: %{time_total}s, Size: %{size_download} bytes\n' -o /dev/null http://example.com

echo ""
echo "=== Testing small download from httpbin.org ==="
curl -s --max-time 10 -w 'Speed: %{speed_download} bytes/sec, Size: %{size_download} bytes\n' -o /dev/null http://httpbin.org/bytes/10000

echo ""
echo "=== Testing 100KB from httpbin.org ==="
curl -s --max-time 15 -w 'Speed: %{speed_download} bytes/sec, Size: %{size_download} bytes\n' -o /dev/null http://httpbin.org/bytes/102400

echo ""
echo "=== Testing speedtest.tele2.net ==="
curl -s --max-time 15 -w 'Speed: %{speed_download} bytes/sec, Size: %{size_download} bytes\n' -o /dev/null http://speedtest.tele2.net/100KB.zip

sudo pkill -INT vpn-client 2>/dev/null || true
sleep 2
