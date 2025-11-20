#!/bin/bash
#
# VPN Health Check & Auto-Restart
# Checks if VPN is connected and restarts if needed
#

set -e

LOG_FILE="/Users/anastasiia/.vpn-health-check.log"
SUDO_PASSWORD="osopanda"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

# Check if VPN client process is running
if ! pgrep -f "vpn-client" > /dev/null; then
    log "VPN client process not running - restarting via menu-bar"
    echo "$SUDO_PASSWORD" | sudo -S killall -9 vpn-client 2>/dev/null || true
    sleep 2
    # Menu-bar will auto-reconnect
    exit 0
fi

# Check if VPN connection is actually working
if ! ping -c 1 -W 2 10.8.0.1 > /dev/null 2>&1; then
    log "VPN not connected (cannot ping server) - restarting"
    echo "$SUDO_PASSWORD" | sudo -S killall -9 vpn-client 2>/dev/null || true
    sleep 2
    # Menu-bar will auto-reconnect
    exit 0
fi

# Check if we can reach IPC
if ! curl -s --max-time 2 http://localhost:8889/peers > /dev/null 2>&1; then
    log "VPN IPC not responding - restarting"
    echo "$SUDO_PASSWORD" | sudo -S killall -9 vpn-client 2>/dev/null || true
    sleep 2
    exit 0
fi

log "VPN healthy - all checks passed"
