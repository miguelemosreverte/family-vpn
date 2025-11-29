#!/bin/bash
# VPN Watchdog - Monitors VPN client and cleans up routing if it crashes

PIDFILE="/tmp/vpn-client.pid"
CLEANUP_SCRIPT="$(dirname "$0")/fix-routing.sh"

echo "🐕 VPN Watchdog started (PID $$)"

# Function to cleanup routing
cleanup_routing() {
    echo "⚠️  VPN client died, cleaning up routing..."

    # Run the routing cleanup script
    if [ -f "$CLEANUP_SCRIPT" ]; then
        bash "$CLEANUP_SCRIPT"
    else
        echo "🔧 Manual cleanup required - run: cd client && bash fix-routing.sh"
    fi

    # Remove PID file
    rm -f "$PIDFILE"
}

# Monitor the VPN client process
while true; do
    if [ -f "$PIDFILE" ]; then
        VPN_PID=$(cat "$PIDFILE")

        # Check if process is still running
        if ! kill -0 "$VPN_PID" 2>/dev/null; then
            echo "❌ VPN client (PID $VPN_PID) is no longer running!"
            cleanup_routing
            exit 0
        fi
    fi

    sleep 2
done
