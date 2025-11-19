#!/bin/bash
#
# Deployment Script
# This script triggers the server to update itself and notify all clients
#
# Usage: ./deploy.sh
#

set -e

VPN_SERVER="95.217.238.72"
UPDATE_PORT="9000"
UPDATE_ENDPOINT="http://$VPN_SERVER:$UPDATE_PORT/update/init"

echo "========================================"
echo "Family VPN - Deployment Script"
echo "========================================"
echo ""
echo "This will trigger the VPN server to:"
echo "  1. Pull latest code from GitHub"
echo "  2. Rebuild and restart itself"
echo "  3. Notify all connected clients to update"
echo ""
echo "Server: $VPN_SERVER"
echo "Endpoint: $UPDATE_ENDPOINT"
echo ""
read -p "Continue? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Deployment cancelled."
    exit 0
fi

echo ""
echo "🚀 Triggering server update..."

# Ping the update endpoint
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$UPDATE_ENDPOINT" 2>&1)
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" = "200" ]; then
    echo "✅ Update initiated successfully!"
    echo ""
    echo "Server response:"
    echo "$BODY"
    echo ""
    echo "The server will:"
    echo "  1. Broadcast UPDATE_AVAILABLE to all clients"
    echo "  2. Pull latest code and rebuild"
    echo "  3. Restart with new version"
    echo ""
    echo "Connected clients will:"
    echo "  1. Receive update notification via VPN"
    echo "  2. Pull latest code and rebuild"
    echo "  3. Restart automatically"
    echo ""
    echo "Deployment complete! 🎉"
else
    echo "❌ Update request failed!"
    echo "HTTP Code: $HTTP_CODE"
    echo "Response: $BODY"
    exit 1
fi

echo "========================================"
