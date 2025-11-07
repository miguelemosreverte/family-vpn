#!/bin/bash

# VPN Test Suite - Safe & Fast
# Runs in ~5 seconds, cleans up automatically

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    echo "osopanda" | sudo -S pkill -f vpn-client 2>/dev/null || true
    sleep 1
}

trap cleanup EXIT

echo "========================================="
echo "VPN TEST SUITE (Safe & Fast)"
echo "========================================="
echo ""

# Test 1: Connection without encryption
echo "Test 1: VPN Connection (No Encryption)"
echo "---------------------------------------"
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 > /tmp/test-plain.log 2>&1 &
VPN_PID=$!
sleep 4

if grep -q "All traffic now routed through VPN" /tmp/test-plain.log; then
    IP=$(curl -s --max-time 3 ifconfig.me 2>/dev/null || echo "timeout")
    if [ "$IP" = "95.217.238.72" ]; then
        echo -e "${GREEN}✓ PASS${NC} - VPN connected, IP changed to server"
        TEST1="PASS"
    else
        echo -e "${RED}✗ FAIL${NC} - VPN connected but IP not changed (got: $IP)"
        TEST1="FAIL"
    fi
else
    echo -e "${RED}✗ FAIL${NC} - VPN failed to connect"
    cat /tmp/test-plain.log | head -10
    TEST1="FAIL"
fi

echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null || true
sleep 2
echo ""

# Test 2: Connection with encryption
echo "Test 2: VPN Connection (WITH Encryption)"
echo "---------------------------------------"
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 -encrypt > /tmp/test-encrypt.log 2>&1 &
VPN_PID=$!
sleep 4

if grep -q "All traffic now routed through VPN" /tmp/test-encrypt.log; then
    if grep -q "Encryption: true" /tmp/test-encrypt.log; then
        # Check if it stays connected
        if ps -p $VPN_PID > /dev/null 2>&1; then
            IP=$(curl -s --max-time 3 ifconfig.me 2>/dev/null || echo "timeout")
            if [ "$IP" = "95.217.238.72" ]; then
                echo -e "${GREEN}✓ PASS${NC} - VPN with encryption working, IP changed"
                TEST2="PASS"
            else
                echo -e "${YELLOW}⚠ PARTIAL${NC} - Connected but IP check failed (got: $IP)"
                TEST2="PARTIAL"
            fi
        else
            echo -e "${RED}✗ FAIL${NC} - VPN connected but died immediately"
            grep -E "(error|Error|failed|Failed)" /tmp/test-encrypt.log | head -5
            TEST2="FAIL"
        fi
    else
        echo -e "${RED}✗ FAIL${NC} - Encryption not enabled"
        TEST2="FAIL"
    fi
else
    echo -e "${RED}✗ FAIL${NC} - VPN failed to connect with encryption"
    grep -E "(error|Error|failed|Failed)" /tmp/test-encrypt.log | head -5
    TEST2="FAIL"
fi

echo "osopanda" | sudo -S kill $VPN_PID 2>/dev/null || true
sleep 2
echo ""

# Summary
echo "========================================="
echo "TEST RESULTS"
echo "========================================="
echo -e "Test 1 (Plain):     ${TEST1}"
echo -e "Test 2 (Encrypted): ${TEST2}"
echo ""

if [ "$TEST1" = "PASS" ] && [ "$TEST2" = "PASS" ]; then
    echo -e "${GREEN}✓ ALL TESTS PASSED${NC}"
    exit 0
elif [ "$TEST1" = "PASS" ]; then
    echo -e "${YELLOW}⚠ PLAIN VPN WORKS, ENCRYPTION NEEDS FIXING${NC}"
    exit 1
else
    echo -e "${RED}✗ TESTS FAILED${NC}"
    exit 1
fi
