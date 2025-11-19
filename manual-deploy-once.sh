#!/bin/bash
#
# One-Time Manual Deployment Script
# Run this ONCE to set up the server, then use ./deploy.sh for all future deployments
#

set -e

echo "========================================"
echo "Family VPN - Initial Server Deployment"
echo "========================================"
echo ""
echo "This script will guide you through the one-time manual setup."
echo "After this, you'll use ./deploy.sh for all future deployments."
echo ""
echo "You need SSH access to: root@95.217.238.72"
echo ""
read -p "Press Enter to continue..."
echo ""

echo "Copy and paste these commands into your SSH session:"
echo ""
cat << 'EOF'
# SSH to server
ssh root@95.217.238.72

# Once connected, run these commands:
cd /root/family-vpn
git pull origin main
chmod +x server-update.sh
cd server
go build -o /root/family-vpn/vpn-server main.go
pkill vpn-server || true
cd /root/family-vpn
nohup ./vpn-server -port 8888 -webhook-port 9000 > /var/log/vpn-server.log 2>&1 &
echo $! > /var/run/vpn-server.pid
echo "✅ Server deployed! Checking logs..."
sleep 2
tail -20 /var/log/vpn-server.log

# You should see:
# Starting HTTP server on port 9000
#   - POST /webhook - GitHub webhook endpoint
#   - POST /update/init - Trigger server and client updates
# VPN server listening on :8888 (encryption: false)
EOF

echo ""
echo "========================================"
echo ""
read -p "Have you completed the above steps? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Deployment cancelled. Run this script again when ready."
    exit 0
fi

echo ""
echo "✅ Great! Now testing the deployment endpoint..."
echo ""

# Test the /update/init endpoint
echo "Testing POST to http://95.217.238.72:9000/update/init"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "http://95.217.238.72:9000/update/init" 2>&1)
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" = "200" ]; then
    echo "✅ Deployment endpoint is working!"
    echo ""
    echo "Server response:"
    echo "$BODY"
    echo ""
    echo "========================================"
    echo "🎉 Initial deployment complete!"
    echo "========================================"
    echo ""
    echo "From now on, use: ./deploy.sh"
    echo ""
    echo "This will automatically:"
    echo "  1. Trigger server to update itself"
    echo "  2. Notify all connected clients"
    echo "  3. Clients will auto-update"
    echo ""
else
    echo "❌ Deployment endpoint test failed!"
    echo "HTTP Code: $HTTP_CODE"
    echo "Response: $BODY"
    echo ""
    echo "Please check:"
    echo "  - Server is running: ssh root@95.217.238.72 'ps aux | grep vpn-server'"
    echo "  - Port 9000 is listening: ssh root@95.217.238.72 'netstat -tlnp | grep 9000'"
    echo "  - Check logs: ssh root@95.217.238.72 'tail -50 /var/log/vpn-server.log'"
    exit 1
fi
