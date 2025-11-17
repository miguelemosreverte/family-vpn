#!/bin/bash
# VPN Doctor - Comprehensive E2E Test
# Tests all three modes: VPN Off (raw), Plain VPN, Encrypted VPN
# Measures: connectivity, latency, throughput, and encryption validation

set -euo pipefail

# Get script directory to find vpn-client
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VPN_CLIENT="${SCRIPT_DIR}/client/vpn-client"

SERVER_ADDR="${VPN_SERVER_ADDR:-95.217.238.72:8888}"
SERVER_IP="${VPN_SERVER_IP:-95.217.238.72}"
TREASURE_TOKEN="${TREASURE_TOKEN:-TREASURE_HUNT_FLAG_SECRET_12345}"
HTTP_TARGET="${TREASURE_HTTP_TARGET:-http://neverssl.com/?treasure=${TREASURE_TOKEN}}"
CAPTURE_SECONDS=${CAPTURE_SECONDS:-3}  # Enough time to capture HTTP request
CONNECT_TIMEOUT=${CONNECT_TIMEOUT:-20}
SUDO_PASS="${SUDO_PASS:-osopanda}"
LOG_DIR="${LOG_DIR:-/tmp}"
PING_COUNT=${PING_COUNT:-3}  # 3 pings for quick average
IPERF_DURATION=${IPERF_DURATION:-20}  # 20 seconds per iperf3 test (allows TCP ramp-up)
IPERF_PORT=${IPERF_PORT:-5201}  # iperf3 default port

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

vpn_wrapper_pid=""
vpn_child_pid=""
tcpdump_wrapper_pid=""
tcpdump_child_pid=""

# Results storage (bash 3.2 compatible - no associative arrays)
off_connectivity=""
off_latency=""
off_throughput=""
off_encryption=""
plain_connectivity=""
plain_latency=""
plain_throughput=""
plain_encryption=""
encrypted_connectivity=""
encrypted_latency=""
encrypted_throughput=""
encrypted_encryption=""

for cmd in tcpdump curl ping iperf3; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo -e "${RED}Missing dependency: $cmd${NC}" >&2
        exit 1
    fi
done

sudo_run() {
    echo "$SUDO_PASS" | sudo -S "$@"
}

cleanup() {
    stop_capture
    stop_vpn
}

trap cleanup EXIT

wait_for_connection() {
    local log_file="$1"
    local waited=0
    while (( waited < CONNECT_TIMEOUT )); do
        if grep -q "All traffic now routed through VPN" "$log_file" 2>/dev/null; then
            return 0
        fi
        if [[ -n "$vpn_wrapper_pid" ]] && ! ps -p "$vpn_wrapper_pid" >/dev/null 2>&1; then
            break
        fi
        sleep 1
        waited=$((waited + 1))
    done
    return 1
}

start_capture() {
    local capture_file="$1"
    local interface="$2"

    # Capture on physical network interface to see encrypted tunnel traffic
    # Filter for VPN server traffic only
    if [[ -z "$interface" ]]; then
        echo "Error: No interface specified for capture" >> "$capture_file"
        return 1
    fi

    # Capture packets to/from VPN server to see if payload is encrypted
    sudo_run tcpdump -i "$interface" -n -s 0 -X "host $SERVER_IP" >"$capture_file" 2>&1 &
    tcpdump_wrapper_pid=$!
    sleep 1
    tcpdump_child_pid=$(pgrep -P "$tcpdump_wrapper_pid" -n 2>/dev/null || true)
}

stop_capture() {
    # Kill all tcpdump processes to ensure cleanup
    sudo_run pkill -INT tcpdump >/dev/null 2>&1 || true
    sleep 0.5

    if [[ -n "$tcpdump_wrapper_pid" ]]; then
        wait "$tcpdump_wrapper_pid" 2>/dev/null || true
        tcpdump_wrapper_pid=""
    fi
    tcpdump_child_pid=""
}

stop_vpn() {
    if [[ -n "$vpn_child_pid" ]]; then
        sudo_run kill -INT "$vpn_child_pid" >/dev/null 2>&1 || true
        vpn_child_pid=""
    fi
    if [[ -n "$vpn_wrapper_pid" ]]; then
        sudo_run kill -INT "$vpn_wrapper_pid" >/dev/null 2>&1 || true
        wait "$vpn_wrapper_pid" 2>/dev/null || true
        vpn_wrapper_pid=""
    fi
}

measure_latency() {
    local target="${1:-8.8.8.8}"
    local output
    output=$(ping -c "$PING_COUNT" "$target" 2>&1 || echo "FAILED")

    if echo "$output" | grep -q "FAILED\|100.0% packet loss\|Unknown host"; then
        echo "FAILED"
        return 1
    fi

    # Extract average latency (works on both macOS and Linux)
    local avg
    if [[ "$OSTYPE" == "darwin"* ]]; then
        avg=$(echo "$output" | grep "round-trip" | awk -F'/' '{print $5}')
    else
        avg=$(echo "$output" | grep "rtt min/avg/max" | awk -F'/' '{print $5}')
    fi

    if [[ -z "$avg" ]]; then
        echo "FAILED"
        return 1
    fi

    echo "$avg"
}

measure_throughput() {
    local target="$1"
    local output

    # Use iperf3 to measure bandwidth
    output=$(iperf3 -c "$target" -p "$IPERF_PORT" -t "$IPERF_DURATION" -J 2>/dev/null)
    local iperf_exit=$?

    # Debug logging
    echo "DEBUG: iperf_exit=$iperf_exit, target=$target" >> /tmp/doctor-debug.log

    # Check if iperf3 failed
    if [[ $iperf_exit -ne 0 ]]; then
        echo "FAILED (exit:$iperf_exit)"
        return 1
    fi

    if [[ -z "$output" ]]; then
        echo "FAILED (empty)"
        return 1
    fi

    # Extract bits_per_second from JSON and convert to Mbps
    # Using grep/sed instead of jq for portability
    # Use tail -2 | head -1 to get sender's final average (second-to-last value)
    local bps=$(echo "$output" | grep -o '"bits_per_second"[[:space:]]*:[[:space:]]*[0-9.]*' | tail -2 | head -1 | grep -o '[0-9.]*$')

    if [[ -z "$bps" ]]; then
        echo "FAILED (no bps)"
        return 1
    fi

    # Convert bits/sec to Mbps
    local mbps=$(echo "scale=2; $bps / 1000000" | bc)
    echo "$mbps"
}

test_connectivity() {
    local mode="$1"
    local encrypt_flag="$2"
    echo -n "  Testing connectivity... "

    if [[ "$mode" != "off" ]]; then
        if ! start_vpn "$mode" "$encrypt_flag"; then
            echo -e "${RED}✗${NC}"
            eval "${mode}_connectivity=FAILED"
            return 1
        fi
        sleep 1
    fi

    if curl -s --max-time 5 http://example.com >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        eval "${mode}_connectivity=OK"
        local result=0
    else
        echo -e "${RED}✗${NC}"
        eval "${mode}_connectivity=FAILED"
        local result=1
    fi

    if [[ "$mode" != "off" ]]; then
        stop_vpn
    fi

    return $result
}

test_latency() {
    local mode="$1"
    local encrypt_flag="$2"
    echo -n "  Measuring latency... "

    if [[ "$mode" != "off" ]]; then
        if ! start_vpn "$mode" "$encrypt_flag"; then
            echo -e "${RED}✗${NC}"
            eval "${mode}_latency=FAILED"
            return 1
        fi
        sleep 1
    fi

    local latency
    latency=$(measure_latency "8.8.8.8")

    if [[ "$latency" == "FAILED" ]]; then
        echo -e "${RED}✗${NC}"
        eval "${mode}_latency=FAILED"
        local result=1
    else
        echo -e "${GREEN}${latency}ms${NC}"
        eval "${mode}_latency='$latency'"
        local result=0
    fi

    if [[ "$mode" != "off" ]]; then
        stop_vpn
    fi

    return $result
}

test_throughput() {
    local mode="$1"
    local encrypt_flag="$2"
    echo -n "  Measuring throughput... "

    # Determine target for iperf3
    local target="$SERVER_IP"  # Default: test to public IP
    if [[ "$mode" != "off" ]]; then
        target="10.8.0.1"  # VPN mode: test to VPN server IP
        if ! start_vpn "$mode" "$encrypt_flag"; then
            echo -e "${RED}✗${NC}"
            eval "${mode}_throughput=FAILED"
            return 1
        fi
        sleep 1
    fi

    local throughput
    throughput=$(measure_throughput "$target")

    if [[ "$throughput" =~ ^FAILED ]]; then
        echo -e "${RED}✗${NC}"
        eval "${mode}_throughput=FAILED"
        local result=1
    else
        echo -e "${GREEN}${throughput} Mbps${NC}"
        eval "${mode}_throughput='$throughput'"
        local result=0
    fi

    if [[ "$mode" != "off" ]]; then
        stop_vpn
    fi

    return $result
}

test_encryption() {
    local mode="$1"
    local encrypt_flag="$2"
    local capture_file="$LOG_DIR/${mode}-capture.log"

    echo -n "  Testing encryption... "

    # Get physical network interface
    local physical_if=$(route -n get default 2>/dev/null | grep interface | awk '{print $2}')

    if [[ -z "$physical_if" ]]; then
        echo -e "${RED}✗ (no interface)${NC}"
        eval "${mode}_encryption=FAILED"
        return 1
    fi

    # Start capturing on physical interface BEFORE starting VPN
    start_capture "$capture_file" "$physical_if"
    sleep 1

    if ! start_vpn "$mode" "$encrypt_flag"; then
        echo -e "${RED}✗${NC}"
        eval "${mode}_encryption=FAILED"
        stop_capture
        return 1
    fi

    sleep 1

    # Send HTTP request with treasure token
    curl -s --max-time 5 "$HTTP_TARGET" >/dev/null 2>&1 || true

    # Continue capturing
    sleep "$CAPTURE_SECONDS"
    stop_capture
    stop_vpn

    # Check if treasure token is visible in network capture
    # Search for partial token since tcpdump -X output may split across lines
    if grep -q "TREASURE" "$capture_file"; then
        echo -e "${YELLOW}LEAKED${NC}"
        eval "${mode}_encryption=LEAKED"
    else
        echo -e "${GREEN}PROTECTED${NC}"
        eval "${mode}_encryption=PROTECTED"
    fi
}

start_vpn() {
    local mode="$1"
    local encrypt_flag="$2"

    local log_file="$LOG_DIR/vpn-${mode}.log"
    local profile_file="$LOG_DIR/vpn-${mode}-cpu.prof"
    : >"$log_file"

    local encrypt_arg=""
    if [[ "$encrypt_flag" == "1" ]]; then
        encrypt_arg="-encrypt"
    fi

    # Enable CPU profiling for VPN modes (not for "off" mode)
    echo "$SUDO_PASS" | sudo -S "$VPN_CLIENT" -server "$SERVER_ADDR" $encrypt_arg -cpuprofile="$profile_file" >"$log_file" 2>&1 &
    vpn_wrapper_pid=$!
    sleep 1
    vpn_child_pid=$(pgrep -P "$vpn_wrapper_pid" -n 2>/dev/null || true)

    if ! wait_for_connection "$log_file"; then
        echo -e "${RED}✗ VPN failed to establish${NC}"
        cat "$log_file" >&2
        return 1
    fi

    echo -e "${GREEN}✓ VPN connected (profiling to: $profile_file)${NC}"
    return 0
}

test_vpn_off() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}Testing: VPN OFF (Baseline)${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    test_connectivity "off" ""
    test_latency "off" ""
    test_throughput "off" ""
    off_encryption="N/A"
}

test_vpn_plain() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}Testing: VPN Plain (No Encryption)${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    test_connectivity "plain" "0" || true
    test_latency "plain" "0" || true
    test_throughput "plain" "0" || true
    test_encryption "plain" "0" || true
}

test_vpn_encrypted() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}Testing: VPN Encrypted (AES-256-GCM)${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    test_connectivity "encrypted" "1" || true
    test_latency "encrypted" "1" || true
    test_throughput "encrypted" "1" || true
    test_encryption "encrypted" "1" || true
}

print_summary() {
    echo -e "\n${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║                      VPN DOCTOR REPORT                         ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"

    printf "\n%-20s %-15s %-15s %-15s\n" "Metric" "VPN OFF" "Plain VPN" "Encrypted VPN"
    printf "%-20s %-15s %-15s %-15s\n" "────────────────────" "─────────────" "─────────────" "──────────────"

    # Connectivity
    printf "%-20s " "Connectivity"
    for mode in off plain encrypted; do
        eval "local status=\$${mode}_connectivity"
        if [[ "$status" == "OK" ]]; then
            printf "${GREEN}%-15s${NC} " "✓ OK"
        else
            printf "${RED}%-15s${NC} " "✗ FAILED"
        fi
    done
    printf "\n"

    # Latency
    printf "%-20s " "Latency (avg)"
    for mode in off plain encrypted; do
        eval "local latency=\$${mode}_latency"
        if [[ "$latency" == "FAILED" ]]; then
            printf "${RED}%-15s${NC} " "✗ FAILED"
        else
            printf "%-15s " "${latency}ms"
        fi
    done
    printf "\n"

    # Throughput
    printf "%-20s " "Throughput"
    for mode in off plain encrypted; do
        eval "local throughput=\$${mode}_throughput"
        if [[ "$throughput" == "FAILED" ]]; then
            printf "${RED}%-15s${NC} " "✗ FAILED"
        else
            printf "%-15s " "${throughput} Mbps"
        fi
    done
    printf "\n"

    # Encryption
    printf "%-20s " "Data Protection"
    for mode in off plain encrypted; do
        eval "local encryption=\$${mode}_encryption"
        if [[ "$encryption" == "N/A" ]]; then
            printf "%-15s " "N/A"
        elif [[ "$encryption" == "PROTECTED" ]]; then
            printf "${GREEN}%-15s${NC} " "✓ Protected"
        elif [[ "$encryption" == "LEAKED" ]]; then
            printf "${YELLOW}%-15s${NC} " "⚠ Leaked"
        else
            printf "${RED}%-15s${NC} " "✗ Failed"
        fi
    done
    printf "\n"

    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"

    # Overall assessment
    echo -e "\n${CYAN}Assessment:${NC}"

    local all_ok=true

    # Check basic connectivity
    if [[ "$off_connectivity" != "OK" ]]; then
        echo -e "${RED}✗ Baseline connectivity failed - check your internet connection${NC}"
        all_ok=false
    fi

    if [[ "$plain_connectivity" != "OK" ]]; then
        echo -e "${RED}✗ Plain VPN connectivity failed - check server and routing${NC}"
        all_ok=false
    fi

    if [[ "$encrypted_connectivity" != "OK" ]]; then
        echo -e "${RED}✗ Encrypted VPN connectivity failed - check encryption implementation${NC}"
        all_ok=false
    fi

    # Check encryption
    if [[ "$plain_encryption" != "LEAKED" ]]; then
        echo -e "${RED}✗ Plain mode should leak data but doesn't - check treasure hunt${NC}"
        all_ok=false
    fi

    if [[ "$encrypted_encryption" != "PROTECTED" ]]; then
        echo -e "${RED}✗ Encrypted mode should protect data but doesn't - encryption broken${NC}"
        all_ok=false
    fi

    if $all_ok; then
        echo -e "${GREEN}✓ All systems operational${NC}"
        echo -e "${GREEN}✓ Encryption is working correctly${NC}"
        echo -e "${GREEN}✓ VPN is ready for use${NC}"
        return 0
    else
        echo -e "${RED}⚠ Issues detected - review logs in $LOG_DIR${NC}"
        return 1
    fi
}

# Main execution
echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                     VPN DOCTOR v1.0                            ║"
echo "║          Comprehensive E2E Testing & Diagnostics               ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

test_vpn_off
test_vpn_plain
test_vpn_encrypted

print_summary

exit_code=$?

# Show profiling information
echo -e "\n${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                  CPU PROFILING RESULTS                         ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${CYAN}Profile files saved:${NC}"
if [[ -f "$LOG_DIR/vpn-plain-cpu.prof" ]]; then
    echo -e "  ${GREEN}✓${NC} Plain VPN:     $LOG_DIR/vpn-plain-cpu.prof"
fi
if [[ -f "$LOG_DIR/vpn-encrypted-cpu.prof" ]]; then
    echo -e "  ${GREEN}✓${NC} Encrypted VPN: $LOG_DIR/vpn-encrypted-cpu.prof"
fi

echo ""
echo -e "${CYAN}Analyze profiles with:${NC}"
echo -e "  ${YELLOW}# Top functions (sorted by CPU time):${NC}"
echo "  go tool pprof -top $LOG_DIR/vpn-plain-cpu.prof"
echo ""
echo -e "  ${YELLOW}# Interactive web UI:${NC}"
echo "  go tool pprof -http=:8080 $LOG_DIR/vpn-plain-cpu.prof"
echo ""
echo -e "  ${YELLOW}# Compare plain vs encrypted:${NC}"
echo "  go tool pprof -base=$LOG_DIR/vpn-plain-cpu.prof $LOG_DIR/vpn-encrypted-cpu.prof"
echo ""

exit $exit_code
