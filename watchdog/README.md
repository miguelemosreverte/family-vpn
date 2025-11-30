# Watchdog

The watchdog is a process supervisor that ensures all Family VPN components stay running.

## Stability: MATURING

Core supervision is stable. Desktop app management is maturing.

## Responsibilities

1. **Monitor Processes**: VPN client, menu bar, desktop app
2. **Auto-Restart**: Restart crashed processes
3. **Health Reporting**: Log status periodically
4. **Update Coordination**: Restart processes after updates

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Watchdog                         │
├─────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │  VPN Client │  │  Menu Bar   │  │ Desktop App │ │
│  │   Monitor   │  │   Monitor   │  │   Monitor   │ │
│  └─────────────┘  └─────────────┘  └─────────────┘ │
│         │                │                │        │
│         └────────────────┼────────────────┘        │
│                          ▼                          │
│              Process Management Loop                │
│           (check every 5 seconds)                   │
└─────────────────────────────────────────────────────┘
```

## Files

| File | Description |
|------|-------------|
| `main.go` | Watchdog logic and process management |

## Build

```bash
cd watchdog
go build -o family-vpn-watchdog .
```

## Install Location

- Binary: `/usr/local/bin/family-vpn-watchdog`
- LaunchAgent: `~/Library/LaunchAgents/com.family.vpn.watchdog.plist`
- Log: `/tmp/family-vpn-watchdog.log`
- PID: `/tmp/family-vpn-watchdog.pid`

## LaunchAgent Configuration

The watchdog is managed by macOS launchd:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "...">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.family.vpn.watchdog</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/family-vpn-watchdog</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

## Process Check Interval

The watchdog checks processes every 5 seconds:

```
[check] → Is vpn-client running? → No → Start vpn-client
[check] → Is menubar running? → Yes → Continue
[check] → Is desktop app running? → No → Start desktop app
```

## Manual Control

```bash
# Start watchdog
launchctl load ~/Library/LaunchAgents/com.family.vpn.watchdog.plist

# Stop watchdog
launchctl unload ~/Library/LaunchAgents/com.family.vpn.watchdog.plist

# Check status
pgrep -lf 'family-vpn-watchdog'

# View logs
tail -50 /tmp/family-vpn-watchdog.log
```

## Troubleshooting

### Watchdog Not Starting

```bash
# Check if plist exists
ls -la ~/Library/LaunchAgents/com.family.vpn.watchdog.plist

# Check launchd status
launchctl list | grep watchdog

# Manually start
/usr/local/bin/family-vpn-watchdog
```

### Processes Not Restarting

```bash
# Check watchdog log
tail -50 /tmp/family-vpn-watchdog.log

# Kill and let watchdog restart
pkill -f 'vpn-client'
sleep 5
pgrep -lf 'vpn-client'  # Should be running again
```

## Related Documentation

- [Client README](../client/README.md) - VPN client
- [Desktop App README](../desktop-app/README.md) - Electron dashboard
- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design
