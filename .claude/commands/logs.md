# /logs [component] [client]

View logs from any Family VPN component.

## Usage

```
/logs                    # All local logs
/logs client             # VPN client log
/logs watchdog           # Watchdog log
/logs menubar            # Menu bar log
/logs client mac-mini    # Remote client log
```

## Log Locations

| Component | Log File |
|-----------|----------|
| VPN Client | `/tmp/family-vpn-client.log` |
| Watchdog | `/tmp/family-vpn-watchdog.log` |
| Menu Bar | `/tmp/family-vpn-menubar.log` |
| Desktop App | Electron DevTools Console |

## Execution Steps

### Local Logs

```bash
# VPN Client log
tail -50 /tmp/family-vpn-client.log

# Watchdog log
tail -50 /tmp/family-vpn-watchdog.log

# Menu bar log
tail -50 /tmp/family-vpn-menubar.log

# All logs combined (last 20 lines each)
echo "=== VPN Client ===" && tail -20 /tmp/family-vpn-client.log
echo "=== Watchdog ===" && tail -20 /tmp/family-vpn-watchdog.log
echo "=== Menu Bar ===" && tail -20 /tmp/family-vpn-menubar.log
```

### Remote Logs

```bash
# Mac mini client log
ssh miguel_lemos@miguel-lemoss-Mac-mini.local "tail -50 /tmp/family-vpn-client.log"

# Anastasia's watchdog log
ssh anastasiia@192.168.0.14 "tail -50 /tmp/family-vpn-watchdog.log"
```

## Filtering Logs

```bash
# Show only errors
tail -100 /tmp/family-vpn-client.log | grep -i error

# Show WebSocket events
tail -100 /tmp/family-vpn-client.log | grep -i websocket

# Show update events
tail -100 /tmp/family-vpn-client.log | grep -i update
```

## Log Rotation

Logs are not rotated automatically. To clear logs:

```bash
> /tmp/family-vpn-client.log
> /tmp/family-vpn-watchdog.log
> /tmp/family-vpn-menubar.log
```
