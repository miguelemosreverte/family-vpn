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

# Start new server in background
echo "🚀 Starting new server..."
cd "$REPO_DIR"
nohup "$SERVER_BINARY" -port 8888 -webhook-port 9000 > /var/log/vpn-server.log 2>&1 &
NEW_PID=$!

# Save new PID
echo "$NEW_PID" > "$PID_FILE"

echo "✅ Server restarted successfully (PID: $NEW_PID)"
echo "========================================"
