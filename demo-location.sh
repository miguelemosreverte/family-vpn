#!/bin/bash
echo "=== Starting VPN ==="
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-demo.log 2>&1 &
VPN_PID=$!
sleep 8

echo ""
echo "=== Your IP appears as: ==="
MY_IP=$(curl -s --max-time 10 ifconfig.me)
echo "$MY_IP"
echo ""

echo "=== Location Details ==="
curl -s --max-time 10 "https://ipapi.co/$MY_IP/json/" | python3 -m json.tool | grep -A1 -E '"(ip|city|region|country_name|org|latitude|longitude)"'

echo ""
echo "=== Stopping VPN ==="
echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null
sleep 2
echo "Done!"
