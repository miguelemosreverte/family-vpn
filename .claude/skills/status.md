# Skill: System Status

Get comprehensive status of the Family VPN system.

## When to Use

Use this skill when:
- User asks "what's the status?", "how are things?", "check health"
- User wants to know if clients are connected
- User asks about versions, uptime, or connectivity
- Before making changes (to establish baseline)

## Information to Gather

### 1. Client Versions

Check what version each client is running:
```bash
./bin/eventbus-cli versions
```

Or via API:
```bash
curl -s -k https://95.217.238.72:443/api/versions
```

### 2. Connected Peers

Get list of connected VPN peers:
```bash
curl -s -k https://95.217.238.72:443/api/peers
```

### 3. Local Processes

Check local Family VPN processes:
```bash
pgrep -lf 'family-vpn' 2>/dev/null
pgrep -lf 'Family VPN' 2>/dev/null
```

### 4. Log Files

Recent logs from each component:
```bash
# Client log
tail -20 /tmp/family-vpn-client.log

# Watchdog log
tail -20 /tmp/family-vpn-watchdog.log

# Menu bar log
tail -20 /tmp/family-vpn-menubar.log
```

### 5. Git Status

Current version and uncommitted changes:
```bash
cd /Users/miguel_lemos/Desktop/family-vpn
git log -1 --format="%h %s"
git status --short
```

### 6. Network Connectivity

Check VPN tunnel status:
```bash
# Check if tun0 interface exists
ifconfig tun0 2>/dev/null || echo "VPN tunnel not active"

# Ping VPN server
ping -c 1 10.8.0.1 2>/dev/null && echo "VPN reachable" || echo "VPN unreachable"
```

## Output Format

Present status in a clear table:

```
Family VPN Status
=================

Git Version: abc1234 - Latest commit message
Local Changes: None / 3 files modified

Connected Clients: 4
  - 10.8.0.2 (MacBook-Air) - abc1234 ✓
  - 10.8.0.3 (Mac-mini)    - abc1234 ✓
  - 10.8.0.4 (iPhone)      - def5678 ⚠ (behind)
  - 10.8.0.10 (Anastasia)  - abc1234 ✓

Local Processes:
  - VPN Client: Running (PID 1234)
  - Watchdog: Running (PID 5678)
  - Menu Bar: Running (PID 9012)
  - Desktop App: Running (PID 3456)

VPN Tunnel: Active (10.8.0.2)
Server: Reachable (45ms)
```

## Health Indicators

| Status | Meaning |
|--------|---------|
| ✓ | Healthy, up-to-date |
| ⚠ | Warning (behind version, high latency) |
| ✗ | Error (disconnected, crashed) |

## Troubleshooting from Status

| Observation | Action |
|-------------|--------|
| Client version behind | Trigger update for that client |
| Process not running | Check watchdog, restart manually |
| VPN tunnel down | Check client log, reconnect |
| High latency | Check network, server load |
