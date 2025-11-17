#!/bin/bash
# Proves encryption by searching captured VPN packets for a known treasure token.
# Plain mode should leak the treasure, encrypted mode should hide it.

set -euo pipefail

SERVER_ADDR="${VPN_SERVER_ADDR:-95.217.238.72:8888}"
SERVER_IP="${VPN_SERVER_IP:-95.217.238.72}"
TREASURE_TOKEN="${TREASURE_TOKEN:-TREASURE_HUNT_FLAG}"
HTTP_TARGET="${TREASURE_HTTP_TARGET:-http://neverssl.com/?treasure=${TREASURE_TOKEN}}"
CAPTURE_SECONDS=${CAPTURE_SECONDS:-6}
CONNECT_TIMEOUT=${CONNECT_TIMEOUT:-20}
SUDO_PASS="${SUDO_PASS:-osopanda}"
LOG_DIR="${LOG_DIR:-/tmp}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

vpn_wrapper_pid=""
vpn_child_pid=""
tcpdump_wrapper_pid=""
tcpdump_child_pid=""

for cmd in tcpdump curl; do
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
        if [[ -n "$vpn_pid" ]] && ! ps -p "$vpn_pid" >/dev/null 2>&1; then
            break
        fi
        sleep 1
        waited=$((waited + 1))
    done
    return 1
}

start_capture() {
    local capture_file="$1"
    sudo_run tcpdump -i any -n host "$SERVER_IP" and port 8888 -s 0 -X >"$capture_file" 2>&1 &
    tcpdump_wrapper_pid=$!
    sleep 1
    tcpdump_child_pid=$(pgrep -P "$tcpdump_wrapper_pid" -n 2>/dev/null || true)
}

stop_capture() {
    if [[ -n "$tcpdump_child_pid" ]]; then
        sudo_run kill -INT "$tcpdump_child_pid" >/dev/null 2>&1 || true
        tcpdump_child_pid=""
    fi
    if [[ -n "$tcpdump_wrapper_pid" ]]; then
        sudo_run kill -INT "$tcpdump_wrapper_pid" >/dev/null 2>&1 || true
        wait "$tcpdump_wrapper_pid" 2>/dev/null || true
        tcpdump_wrapper_pid=""
    fi
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

run_treasure_hunt() {
    local mode="$1"       # plain or encrypted
    local encrypt_flag="$2" # 0 or 1
    local result_var="$3"

    local log_file="$LOG_DIR/vpn-${mode}.log"
    local capture_file="$LOG_DIR/${mode}-capture.log"
    : >"$log_file"

    echo "--------------------------------------------"
    echo "Running treasure hunt in $mode mode"

    local encrypt_arg=""
    if [[ "$encrypt_flag" == "1" ]]; then
        encrypt_arg="-encrypt"
    fi

    echo "$SUDO_PASS" | sudo -S ./client/vpn-client -server "$SERVER_ADDR" $encrypt_arg >"$log_file" 2>&1 &
    vpn_wrapper_pid=$!
    sleep 1
    vpn_child_pid=$(pgrep -P "$vpn_wrapper_pid" -n 2>/dev/null || true)

    if ! wait_for_connection "$log_file"; then
        echo -e "${RED}✗ VPN failed to establish in $mode mode${NC}"
        cat "$log_file" >&2
        return 2
    fi
    echo -e "${GREEN}✓ VPN ready ($mode)${NC}"

    start_capture "$capture_file"

    curl -s --max-time 8 "$HTTP_TARGET" >/dev/null || true

    sleep "$CAPTURE_SECONDS"
    stop_capture

    stop_vpn

    if grep -q "$TREASURE_TOKEN" "$capture_file"; then
        echo -e "${YELLOW}Treasure token FOUND in $mode capture${NC}"
        printf -v "$result_var" "FOUND"
    else
        echo -e "${YELLOW}Treasure token NOT FOUND in $mode capture${NC}"
        printf -v "$result_var" "NOT_FOUND"
    fi
}
plain_result=""
encrypted_result=""
run_treasure_hunt plain 0 plain_result
run_treasure_hunt encrypted 1 encrypted_result

echo ""
echo "============= SUMMARY ============="
printf "Plain mode:      %s\n" "$plain_result"
printf "Encrypted mode:  %s\n" "$encrypted_result"

echo ""
if [[ "$plain_result" == "FOUND" && "$encrypted_result" == "NOT_FOUND" ]]; then
    echo -e "${GREEN}Encryption proof succeeded${NC}"
    exit 0
else
    echo -e "${RED}Encryption proof FAILED - investigate captures in $LOG_DIR${NC}"
    exit 1
fi
