#!/bin/bash
rm -f /tmp/throughput-only-debug.log

echo "Starting VPN..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-throughput-only.log 2>&1 &
sleep 5

echo "Testing throughput immediately..."
output=$(curl -s --max-time 20 -w '%{speed_download}' -o /dev/null http://speedtest.tele2.net/100KB.zip 2>&1)
curl_exit=$?

echo "curl_exit=$curl_exit" | tee -a /tmp/throughput-only-debug.log
echo "output='$output'" | tee -a /tmp/throughput-only-debug.log

if [[ $curl_exit -eq 0 ]] && [[ -n "$output" ]] && [[ "$output" =~ ^[0-9]+$ ]] && [[ "$output" -gt 1000 ]]; then
    mbps=$(echo "scale=2; $output * 8 / 1000000" | bc)
    echo "SUCCESS: $mbps Mbps"
else
    echo "FAILED"
fi

sudo pkill -INT vpn-client 2>/dev/null || true
sleep 2
