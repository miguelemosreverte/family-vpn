# /status

Get comprehensive status of the Family VPN system.

## Usage

```
/status
```

## What This Command Does

1. Checks local git version and uncommitted changes
2. Queries connected clients and their versions
3. Checks local VPN processes
4. Verifies VPN tunnel status
5. Reports overall system health

## Execution Steps

```bash
# 1. Git version
cd /Users/miguel_lemos/Desktop/family-vpn
echo "Git Version: $(git log -1 --format='%h %s')"
echo "Local Changes: $(git status --short | wc -l | tr -d ' ') files"

# 2. Client versions via EventBus
./bin/eventbus-cli versions

# 3. Local processes
pgrep -lf 'family-vpn' 2>/dev/null
pgrep -lf 'Family VPN' 2>/dev/null

# 4. VPN tunnel
ifconfig tun0 2>/dev/null | head -3 || echo "VPN tunnel not active"

# 5. Server ping
ping -c 1 -t 2 10.8.0.1 2>/dev/null && echo "VPN Server: Reachable" || echo "VPN Server: Unreachable"
```

## Expected Output

```
Family VPN Status
=================

Git Version: abc1234 - Latest commit message
Local Changes: 0 files

Connected Clients: 3
  - MacBook-Air (10.8.0.2)  - abc1234 ✓
  - Mac-mini (10.8.0.3)     - abc1234 ✓
  - Anastasia (10.8.0.10)   - abc1234 ✓

Local Processes:
  - VPN Client: Running (PID 1234)
  - Watchdog: Running (PID 5678)
  - Menu Bar: Running (PID 9012)
  - Desktop App: Running (PID 3456)

VPN Tunnel: Active (10.8.0.2)
Server: Reachable (23ms)
```

## Health Indicators

| Status | Meaning |
|--------|---------|
| ✓ | Healthy, up-to-date |
| ⚠ | Warning (behind version, high latency) |
| ✗ | Error (disconnected, crashed) |
