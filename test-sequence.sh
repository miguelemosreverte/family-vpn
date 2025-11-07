#!/bin/bash

echo "========================================="
echo "VPN TEST SEQUENCE"
echo "========================================="
echo ""

echo "TEST 1: VPN WITHOUT ENCRYPTION"
echo "-------------------------------"
echo "Starting VPN (no encryption)..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/vpn-no-encrypt.log 2>&1 &
VPN_PID=$!
sleep 8

echo "Checking connection..."
cat /tmp/vpn-no-encrypt.log | head -10
echo ""

if ps -p $VPN_PID > /dev/null; then
    echo "✓ VPN is RUNNING"
    echo "Your IP: $(curl -s --max-time 10 ifconfig.me)"
    echo ""
    echo "Press ENTER when you've confirmed it works..."
    read

    echo "Stopping VPN..."
    echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null
    sleep 3
else
    echo "✗ VPN failed to start"
    cat /tmp/vpn-no-encrypt.log
    exit 1
fi

echo ""
echo "========================================="
echo ""
echo "TEST 2: VPN WITH ENCRYPTION"
echo "-------------------------------"
echo "Starting VPN (WITH encryption)..."
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt > /tmp/vpn-encrypt.log 2>&1 &
VPN_PID=$!
sleep 8

echo "Checking connection..."
cat /tmp/vpn-encrypt.log | head -15
echo ""

if ps -p $VPN_PID > /dev/null; then
    echo "✓ VPN is RUNNING"
    echo "Your IP: $(curl -s --max-time 10 ifconfig.me 2>&1)"
    echo ""
    echo "Press ENTER when you've confirmed it works (or doesn't)..."
    read

    echo "Stopping VPN..."
    echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null
    sleep 3
else
    echo "✗ VPN failed to start"
    cat /tmp/vpn-encrypt.log
fi

echo ""
echo "========================================="
echo "TESTS COMPLETE"
echo "========================================="
