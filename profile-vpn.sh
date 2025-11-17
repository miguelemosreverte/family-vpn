#!/bin/bash
# Profile VPN Performance
# Runs both client and server with CPU profiling enabled, generates load, and collects profiles

set -euo pipefail

SERVER_HOST="${VPN_SERVER_HOST:-95.217.238.72}"
SSH_KEY="${VPN_SSH_KEY:-~/.ssh/id_ed25519_hetzner}"
SUDO_PASS="${SUDO_PASS:-osopanda}"
TEST_DURATION=20  # seconds

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  VPN Performance Profiling${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Step 1: Start server with profiling
echo -e "\n${YELLOW}→${NC} Starting server with CPU profiling..."
ssh -i "$SSH_KEY" "root@$SERVER_HOST" "
cd vpn-first-steps/server
pkill vpn-server || true
sleep 1
nohup ./vpn-server -port 8888 -cpuprofile=/tmp/server-cpu.prof > /var/log/vpn-server.log 2>&1 &
sleep 2
pgrep vpn-server && echo '✓ Server running with profiling'
"

# Step 2: Build client
echo -e "\n${YELLOW}→${NC} Building client..."
cd client
go build -o vpn-client main.go
cd ..

# Step 3: Start client with profiling
echo -e "\n${YELLOW}→${NC} Starting client with CPU profiling..."
echo "$SUDO_PASS" | sudo -S ./client/vpn-client \
    -server ${SERVER_HOST}:8888 \
    -encrypt=false \
    -cpuprofile=/tmp/client-cpu.prof \
    > /tmp/vpn-client-profile.log 2>&1 &
VPN_PID=$!

# Wait for VPN to connect
sleep 5

if ! pgrep -P $VPN_PID vpn-client >/dev/null 2>&1; then
    echo -e "${RED}✗ VPN client failed to start${NC}"
    cat /tmp/vpn-client-profile.log
    exit 1
fi

echo -e "${GREEN}✓ Client running with profiling${NC}"

# Step 4: Generate load with iperf3
echo -e "\n${YELLOW}→${NC} Generating load with iperf3 (${TEST_DURATION}s)..."
iperf3 -c 10.8.0.1 -t $TEST_DURATION -P 4 || echo "iperf3 test completed"

# Step 5: Stop client
echo -e "\n${YELLOW}→${NC} Stopping client..."
echo "$SUDO_PASS" | sudo -S pkill vpn-client || true
sleep 2

# Step 6: Stop server and collect profile
echo -e "\n${YELLOW}→${NC} Stopping server and collecting profiles..."
ssh -i "$SSH_KEY" "root@$SERVER_HOST" "
pkill -INT vpn-server || true
sleep 2
cat /tmp/server-cpu.prof
" > /tmp/server-cpu.prof

echo -e "\n${GREEN}✓ Profiling complete!${NC}"
echo -e "\n${BLUE}Profile files:${NC}"
echo "  Client: /tmp/client-cpu.prof"
echo "  Server: /tmp/server-cpu.prof"

echo -e "\n${BLUE}Analyze with:${NC}"
echo "  go tool pprof -http=:8080 /tmp/client-cpu.prof"
echo "  go tool pprof -http=:8081 /tmp/server-cpu.prof"

echo -e "\n${BLUE}Or generate text reports:${NC}"
echo "  go tool pprof -top /tmp/client-cpu.prof"
echo "  go tool pprof -top /tmp/server-cpu.prof"
