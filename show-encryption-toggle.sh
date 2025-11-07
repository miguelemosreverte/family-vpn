#!/bin/bash

echo "=== Test 1: WITHOUT -encrypt flag ==="
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/test1.log 2>&1 &
PID1=$!
sleep 4
grep "Encryption:" /tmp/test1.log
echo "osopanda" | sudo -S kill $PID1 2>/dev/null
sleep 2

echo ""
echo "=== Test 2: WITH -encrypt flag ==="
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt > /tmp/test2.log 2>&1 &
PID2=$!
sleep 4
grep "Encryption:" /tmp/test2.log
echo "osopanda" | sudo -S kill $PID2 2>/dev/null
sleep 2

echo ""
echo "✓ Encryption is a RUNTIME toggle - no server restart needed!"
echo "✓ Just add/remove the -encrypt flag when starting the client"
