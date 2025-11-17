#!/bin/bash
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/throughput-test.log 2>&1 &
sleep 5

echo "Testing with 20s timeout..."
output=$(curl -s --max-time 20 -w '%{speed_download}' -o /dev/null http://speedtest.tele2.net/100KB.zip 2>/dev/null)
exit_code=$?

echo "Exit code: $exit_code"
echo "Output: '$output'"

if [[ -n "$output" ]] && [[ "$output" =~ ^[0-9]+$ ]] && [[ "$output" -gt 1000 ]]; then
    mbps=$(echo "scale=2; $output * 8 / 1000000" | bc)
    echo "SUCCESS: $mbps Mbps"
else
    echo "FAILED - likely timeout or connection issue"
fi

sudo pkill -INT vpn-client 2>/dev/null || true
sleep 2
