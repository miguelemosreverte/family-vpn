#!/bin/bash
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/debug-vpn.log 2>&1 &
VPN_PID=$!
sleep 5

echo "Testing throughput..."
output=$(curl -s --max-time 8 -w '%{speed_download}' -o /dev/null http://speedtest.tele2.net/1MB.zip 2>/dev/null)
curl_exit=$?

echo "Curl exit code: $curl_exit"
echo "Output: '$output'"
echo "Output length: ${#output}"

if [[ $curl_exit -ne 0 ]]; then
    echo "Curl FAILED"
elif [[ -z "$output" ]]; then
    echo "Output is EMPTY"
elif ! [[ "$output" =~ ^[0-9]+$ ]]; then
    echo "Output is NOT a number"
    echo "$output" | od -c | head -3
elif [[ "$output" -lt 1000 ]]; then
    echo "Output is too small: $output bytes/sec"
else
    mbps=$(echo "scale=2; $output * 8 / 1000000" | bc)
    echo "SUCCESS: $mbps Mbps"
fi

sudo pkill -INT vpn-client 2>/dev/null || true
wait $VPN_PID 2>/dev/null || true
