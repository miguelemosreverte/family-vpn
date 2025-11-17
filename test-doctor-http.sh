#!/bin/bash
# VPN Doctor with HTTP download testing

set -e

SERVER_IP="95.217.238.72"
VPN_SERVER_IP="10.8.0.1"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║              VPN DOCTOR - HTTP Download Test                   ║"
echo "║              Testing Real Browsing Performance                 ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Test function for HTTP downloads
test_http_download() {
    local url="$1"
    local size="$2"
    local timeout="${3:-10}"

    # Use --max-time for timeout, measure speed
    local result=$(curl -s -o /dev/null -w "%{http_code},%{time_total},%{speed_download}" --max-time $timeout "$url" 2>/dev/null || echo "000,0,0")

    local http_code=$(echo "$result" | cut -d',' -f1)
    local time_total=$(echo "$result" | cut -d',' -f2)
    local speed_bytes=$(echo "$result" | cut -d',' -f3)

    # Convert bytes/sec to Mbps
    local speed_mbps=$(echo "scale=2; $speed_bytes * 8 / 1000000" | bc 2>/dev/null || echo "0")

    if [ "$http_code" = "200" ] && [ "$speed_mbps" != "0" ]; then
        echo -e "${GREEN}✓ SUCCESS${NC} - ${speed_mbps} Mbps (${time_total}s for $size)"
        echo "$speed_mbps"
    else
        echo -e "${RED}✗ FAILED${NC} - Timeout or error (HTTP $http_code)"
        echo "0"
    fi
}

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${CYAN}Testing: VPN OFF (Baseline)${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

echo -n "  Small HTTP (100KB): "
baseline_small=$(test_http_download "http://speedtest.tele2.net/100KB.zip" "100KB" 10)

echo -n "  Medium HTTP (1MB): "
baseline_medium=$(test_http_download "http://speedtest.tele2.net/1MB.zip" "1MB" 15)

echo -n "  Large HTTP (10MB): "
baseline_large=$(test_http_download "http://speedtest.tele2.net/10MB.zip" "10MB" 30)

echo ""
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${CYAN}Testing: VPN Encrypted${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

echo "  Starting VPN..."
echo "osopanda" | sudo -S ./client/vpn-client -server $SERVER_IP:8888 -encrypt > /tmp/vpn-http-test.log 2>&1 &
VPN_PID=$!

sleep 5

echo -n "  Small HTTP (100KB): "
vpn_small=$(test_http_download "http://speedtest.tele2.net/100KB.zip" "100KB" 10)

echo -n "  Medium HTTP (1MB): "
vpn_medium=$(test_http_download "http://speedtest.tele2.net/1MB.zip" "1MB" 15)

echo -n "  Large HTTP (10MB): "
vpn_large=$(test_http_download "http://speedtest.tele2.net/10MB.zip" "10MB" 30)

echo ""
echo "  Stopping VPN..."
echo "osopanda" | sudo -S pkill vpn-client
wait $VPN_PID 2>/dev/null

sleep 2

echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                   DOWNLOAD PERFORMANCE REPORT                  ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""
printf "%-20s %-15s %-15s %-10s\n" "Test" "Baseline" "VPN" "% of Baseline"
echo "────────────────── ─────────────── ─────────────── ──────────"

# Calculate percentages
small_pct=$(echo "scale=0; ($vpn_small / $baseline_small) * 100" | bc 2>/dev/null || echo "0")
medium_pct=$(echo "scale=0; ($vpn_medium / $baseline_medium) * 100" | bc 2>/dev/null || echo "0")
large_pct=$(echo "scale=0; ($vpn_large / $baseline_large) * 100" | bc 2>/dev/null || echo "0")

printf "%-20s %-15s %-15s %-10s\n" "Small (100KB)" "${baseline_small} Mbps" "${vpn_small} Mbps" "${small_pct}%"
printf "%-20s %-15s %-15s %-10s\n" "Medium (1MB)" "${baseline_medium} Mbps" "${vpn_medium} Mbps" "${medium_pct}%"
printf "%-20s %-15s %-15s %-10s\n" "Large (10MB)" "${baseline_large} Mbps" "${vpn_large} Mbps" "${large_pct}%"

echo ""
echo -e "${CYAN}Diagnosis:${NC}"

# Check if VPN downloads are working
if [ "$vpn_small" = "0" ] && [ "$vpn_medium" = "0" ] && [ "$vpn_large" = "0" ]; then
    echo -e "${RED}✗ CRITICAL: All HTTP downloads failed through VPN${NC}"
    echo -e "${YELLOW}  This explains why browsing is broken despite good iperf3 results${NC}"
    echo -e "${YELLOW}  Issue: Server→Client path is broken (downloads don't work)${NC}"
    echo -e "${YELLOW}  iperf3 only tests Client→Server (uploads), not downloads!${NC}"
elif [ "$small_pct" -lt "10" ]; then
    echo -e "${RED}✗ SEVERE: VPN downloads are <10% of baseline${NC}"
    echo -e "${YELLOW}  Downloads are essentially non-functional${NC}"
else
    echo -e "${GREEN}✓ HTTP downloads working${NC}"
    echo -e "${GREEN}  Small: ${small_pct}% | Medium: ${medium_pct}% | Large: ${large_pct}%${NC}"
fi

echo ""
echo "Check /tmp/vpn-http-test.log for VPN client logs"
