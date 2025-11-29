#!/bin/bash
#
# Diagnose Video Signal Flow
# Traces where the signal breaks in the delivery chain
#

set -e

echo "╔══════════════════════════════════════════════════════════╗"
echo "║         Video Signal Flow Diagnostic Tool               ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# Get peer info
PEERS=$(curl -s http://localhost:8889/peers)
THIS_IP=$(echo "$PEERS" | python3 -c "import sys, json; peers = json.load(sys.stdin); print([p['vpn_address'] for p in peers if 'Anastasiia' in p['hostname']][0])")
MIGUEL_IP=$(echo "$PEERS" | python3 -c "import sys, json; peers = json.load(sys.stdin); print([p['vpn_address'] for p in peers if 'Miguel' in p['hostname']][0])")

echo "This machine: $THIS_IP"
echo "Miguel's machine: $MIGUEL_IP"
echo ""

# Step 1: Check VPN client versions
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Step 1: Check VPN Client Binaries"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "This machine:"
ls -la ~/Desktop/family-vpn/client/vpn-client | awk '{print "  Built: " $6 " " $7 " " $8}'
echo ""
echo "Miguel's machine:"
ssh miguel_lemos@"$MIGUEL_IP" "ls -la ~/Desktop/family-vpn/client/vpn-client" | awk '{print "  Built: " $6 " " $7 " " $8}'
echo ""

# Step 2: Check if binaries have getRealUserHomeDir
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Step 2: Verify getRealUserHomeDir in Code"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "This machine:"
if grep -q "getRealUserHomeDir" ~/Desktop/family-vpn/client/main.go; then
    echo "  ✓ getRealUserHomeDir found in source"
else
    echo "  ✗ getRealUserHomeDir NOT in source"
fi

echo ""
echo "Miguel's machine:"
if ssh miguel_lemos@"$MIGUEL_IP" "grep -q 'getRealUserHomeDir' ~/Desktop/family-vpn/client/main.go"; then
    echo "  ✓ getRealUserHomeDir found in source"
else
    echo "  ✗ getRealUserHomeDir NOT in source"
fi
echo ""

# Step 3: Check VPN client process start times
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Step 3: Check VPN Client Process Start Times"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "This machine:"
ps aux | grep vpn-client | grep -v grep | head -1 | awk '{print "  Started: " $9}'

echo ""
echo "Miguel's machine:"
ssh miguel_lemos@"$MIGUEL_IP" "ps aux | grep vpn-client | grep -v grep | head -1" | awk '{print "  Started: " $9}'
echo ""

# Step 4: Send signal and trace
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Step 4: Send Signal and Trace Flow"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "Cleaning old signal files..."
rm -f ~/.family-vpn-video-* 2>/dev/null
ssh miguel_lemos@"$MIGUEL_IP" "rm -f ~/.family-vpn-video-* 2>/dev/null"

echo "Sending video call signal..."
RESPONSE=$(curl -s -X POST http://localhost:8889/signal/send \
    -H "Content-Type: application/json" \
    -d "{\"peer\":\"$MIGUEL_IP\",\"data\":\"{\\\"type\\\":\\\"call-start\\\",\\\"from\\\":\\\"$THIS_IP\\\",\\\"fromName\\\":\\\"Anastasiia\\\"}\"}")

echo "  IPC Response: $RESPONSE"
echo ""

# Wait for signal to propagate
sleep 2

echo "Checking signal files..."
echo ""

# Check outgoing signal on this machine
if ls ~/.family-vpn-video-out-* 1> /dev/null 2>&1; then
    OUTFILE=$(ls ~/.family-vpn-video-out-* | head -1)
    echo "  ✓ Outgoing signal file created: $OUTFILE"
    echo "    Content: $(cat $OUTFILE)"
else
    echo "  ⚠ No outgoing signal file (may have been processed already)"
fi

echo ""

# Check incoming signal on Miguel's machine
MIGUEL_SIGNAL=$(ssh miguel_lemos@"$MIGUEL_IP" "ls ~/.family-vpn-video-signal 2>/dev/null || echo 'NOT_FOUND'")
if [ "$MIGUEL_SIGNAL" != "NOT_FOUND" ]; then
    echo "  ✓ Incoming signal file on Miguel's machine: $MIGUEL_SIGNAL"
    MIGUEL_CONTENT=$(ssh miguel_lemos@"$MIGUEL_IP" "cat ~/.family-vpn-video-signal 2>/dev/null")
    echo "    Content: $MIGUEL_CONTENT"
else
    echo "  ✗ NO incoming signal file on Miguel's machine"
    echo "    Expected: /Users/miguel_lemos/.family-vpn-video-signal"
fi

echo ""

# Step 5: Check if VPN client is monitoring for outgoing signals
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Step 5: Check Signal Monitoring Goroutines"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "The VPN client should have monitorOutgoingVideoSignals() goroutine"
echo "This should be logging when it finds outgoing signal files"
echo ""

# Summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Summary"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ "$MIGUEL_SIGNAL" != "NOT_FOUND" ]; then
    echo "✓ Signal delivery WORKING!"
    echo "  The signal reached Miguel's machine successfully"
else
    echo "✗ Signal delivery BROKEN"
    echo ""
    echo "Possible causes:"
    echo "  1. Server not forwarding VIDEO_CALL messages"
    echo "  2. Miguel's client not receiving/handling CTRL:VIDEO_CALL:"
    echo "  3. Miguel's client using wrong home directory"
    echo "  4. Miguel's client not running the latest code"
    echo ""
    echo "Recommendation:"
    echo "  Deploy the latest code to Miguel's machine using ./deploy.sh"
    echo "  This will ensure Miguel's VPN client has the getRealUserHomeDir fix"
fi

echo ""
