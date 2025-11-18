#!/bin/bash
#
# Server Auto-Update Script
# This script is executed on the VPN server when /update/init is called
# It pulls latest code, rebuilds the server, and restarts it
#

set -e

REPO_DIR="/root/family-vpn"
SERVER_BINARY="$REPO_DIR/vpn-server"
PID_FILE="/var/run/vpn-server.pid"

echo "========================================"
echo "VPN Server Auto-Update"
echo "========================================"

cd "$REPO_DIR"

# Pull latest code from GitHub
echo "📥 Pulling latest code from GitHub..."
git pull origin main

# Generate TLS certificates if they don't exist
if [ ! -f "$REPO_DIR/certs/server.crt" ] || [ ! -f "$REPO_DIR/certs/server.key" ]; then
    echo "🔐 Generating TLS certificates..."
    cd "$REPO_DIR"
    ./generate-certs.sh
fi

# Rebuild server binary
echo "🔨 Building server..."
cd "$REPO_DIR/server"
/usr/local/go/bin/go build -o "$SERVER_BINARY" main.go

if [ ! -f "$SERVER_BINARY" ]; then
    echo "❌ Build failed - binary not found"
    exit 1
fi

echo "✅ Server built successfully"

# Kill old server process
echo "🔄 Restarting server..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Killing old server process (PID: $OLD_PID)..."
        kill "$OLD_PID"
        sleep 2
    fi
fi

# Set up port forwarding 443→8888 (if not already set up)
echo "🔀 Setting up port forwarding 443→8888..."
if ! iptables -t nat -L PREROUTING -n | grep -q 'tcp dpt:443'; then
    # Enable IP forwarding
    echo 1 > /proc/sys/net/ipv4/ip_forward

    # Add PREROUTING rule to redirect 443→8888
    iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8888

    # Allow port 8888 in firewall
    iptables -I INPUT -p tcp --dport 8888 -j ACCEPT || true

    echo "✅ Port forwarding configured: external 443 → internal 8888"
else
    echo "✅ Port forwarding already configured"
fi

# Start new server in background with TLS on port 8888 (externally accessible via 443)
echo "🚀 Starting new server with TLS on port 8888 (forwarded from 443)..."
cd "$REPO_DIR"
nohup "$SERVER_BINARY" -port 8888 -webhook-port 9000 -tls -tls-cert certs/server.crt -tls-key certs/server.key > /var/log/vpn-server.log 2>&1 &
NEW_PID=$!

# Save new PID
echo "$NEW_PID" > "$PID_FILE"

echo "✅ Server restarted successfully (PID: $NEW_PID)"
echo "========================================"
