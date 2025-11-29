#!/bin/bash
# Auto-Update Script for Family VPN Clients
# Triggered by WebSocket "update_available" message from server
# This script runs autonomously without manual intervention

set -e

LOG_FILE="/tmp/family-vpn-auto-update.log"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

log "======================================"
log "Auto-update triggered via WebSocket"
log "Repository: $REPO_DIR"
log "======================================"

cd "$REPO_DIR"

# 1. Fetch latest changes
log "Fetching latest changes from Git..."
git fetch origin main 2>&1 | tee -a "$LOG_FILE"

# 2. Check if update is needed
LOCAL_COMMIT=$(git rev-parse HEAD)
REMOTE_COMMIT=$(git rev-parse origin/main)

if [ "$LOCAL_COMMIT" == "$REMOTE_COMMIT" ]; then
    log "Already up-to-date (commit: ${LOCAL_COMMIT:0:8})"
    exit 0
fi

log "Update available: ${LOCAL_COMMIT:0:8} -> ${REMOTE_COMMIT:0:8}"

# 3. Pull changes
log "Pulling changes..."
git pull origin main 2>&1 | tee -a "$LOG_FILE"

# 4. Run automated installation
log "Running install.sh..."
if [ -f "install.sh" ]; then
    bash install.sh 2>&1 | tee -a "$LOG_FILE"
    log "✅ Update completed successfully"
else
    log "❌ install.sh not found"
    exit 1
fi

# 5. Report success back to server (will be implemented later)
log "Update complete - services will restart automatically"
log "======================================"
