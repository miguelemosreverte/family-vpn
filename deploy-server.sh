#!/bin/bash
# VPN Server Deployment Script
# Deploys the current local code to the remote VPN server

set -euo pipefail

# Load .env if exists
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

SERVER_HOST="${VPN_SERVER_HOST:-95.217.238.72}"
SERVER_DIR="family-vpn"

# Handle SSH key - if VPN_SSH_KEY is set and looks like base64, decode it
if [ -n "${VPN_SSH_KEY:-}" ] && [ "${#VPN_SSH_KEY}" -gt 100 ]; then
    # VPN_SSH_KEY is base64 encoded - decode to temp file
    SSH_KEY_FILE="/tmp/vpn_deploy_key_$$"
    echo "$VPN_SSH_KEY" | base64 -d > "$SSH_KEY_FILE"
    chmod 600 "$SSH_KEY_FILE"
    SSH_KEY="$SSH_KEY_FILE"
    trap "rm -f $SSH_KEY_FILE" EXIT
else
    # Fall back to file path
    SSH_KEY="${VPN_SSH_KEY:-~/.ssh/id_ed25519_hetzner}"
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  VPN Server Deployment${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Check if we're on a clean commit
if ! git diff-index --quiet HEAD -- server/; then
    echo -e "${YELLOW}Warning: Uncommitted changes in server/ directory${NC}"
    echo -e "${YELLOW}Consider committing first for reproducibility${NC}"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

CURRENT_COMMIT=$(git rev-parse --short HEAD)
echo -e "${BLUE}Current commit: ${CURRENT_COMMIT}${NC}"

echo -e "\n${YELLOW}→${NC} Pushing code to remote..."
git push origin main || echo -e "${YELLOW}Note: Push failed or already up to date${NC}"

echo -e "\n${YELLOW}→${NC} Deploying to server ${SERVER_HOST}..."

ssh -i "$SSH_KEY" "root@$SERVER_HOST" "
set -e
echo '→ Pulling latest code...'
cd $SERVER_DIR
git fetch
git reset --hard origin/main

echo '→ Building server...'
cd server
/usr/local/go/bin/go build -o vpn-server main.go

echo '→ Stopping old server...'
pkill vpn-server || true
sleep 1

echo '→ Starting new server...'
cd ..
nohup ./vpn-server -port 443 -webhook-port 9000 -tls -tls-cert certs/server.crt -tls-key certs/server.key > /var/log/vpn-server.log 2>&1 &
sleep 2

if pgrep vpn-server > /dev/null; then
    echo '✓ Server deployed and running'
    pgrep vpn-server
else
    echo '✗ Server failed to start'
    exit 1
fi
"

echo -e "\n${GREEN}✓ Deployment complete!${NC}"
echo -e "${BLUE}Server is running commit: ${CURRENT_COMMIT}${NC}"
echo -e "\n${YELLOW}→${NC} Checking server logs..."
ssh -i "$SSH_KEY" "root@$SERVER_HOST" "tail -5 /var/log/vpn-server.log"

echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}Ready to test! Run: ./test-doctor.sh${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
