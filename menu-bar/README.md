# Menu Bar App

The menu bar app provides quick access to VPN status and controls from the macOS menu bar.

## Stability: DEVELOPING

Basic functionality works. Additional features being added.

## Features

- VPN connection status indicator
- Peer list with SSH access
- Quick connect/disconnect
- Version display

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Menu Bar App                       │
├─────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │   Status    │  │  Peer List  │  │   Actions   │ │
│  │  Indicator  │  │    Menu     │  │    Menu     │ │
│  └─────────────┘  └─────────────┘  └─────────────┘ │
│         │                │                │        │
│         └────────────────┼────────────────┘        │
│                          ▼                          │
│              System Tray (systray)                  │
└─────────────────────────────────────────────────────┘
```

## Files

| File | Description |
|------|-------------|
| `main.go` | Menu bar logic using systray |

## Build

```bash
cd menu-bar
go build -o family-vpn-menubar .
```

## Install Location

- Binary: `/usr/local/bin/family-vpn-menubar`
- Log: `/tmp/family-vpn-menubar.log`

## Menu Structure

```
┌─────────────────────────┐
│ 🟢 Family VPN           │
├─────────────────────────┤
│ Status: Connected       │
│ IP: 10.8.0.2            │
├─────────────────────────┤
│ Peers                  ▶│
│   └─ Mac-mini (10.8.0.3)│
│   └─ Anastasia (10.8.0.10)
├─────────────────────────┤
│ SSH to...              ▶│
│   └─ Mac-mini           │
│   └─ Anastasia          │
├─────────────────────────┤
│ Disconnect              │
│ Settings...             │
│ Quit                    │
└─────────────────────────┘
```

## Status Icons

| Icon | Meaning |
|------|---------|
| 🟢 | Connected to VPN |
| 🟡 | Connecting... |
| 🔴 | Disconnected |

## Dependencies

- `github.com/getlantern/systray` - System tray library

## Troubleshooting

### Menu Bar Icon Not Showing

```bash
# Check if process is running
pgrep -lf 'family-vpn-menubar'

# Check logs
tail -50 /tmp/family-vpn-menubar.log

# Restart via watchdog
pkill -f 'family-vpn-menubar'
# Watchdog will restart automatically
```

### SSH Not Working

```bash
# Test SSH manually
ssh miguel_lemos@miguel-lemoss-Mac-mini.local

# Check known_hosts
cat ~/.ssh/known_hosts | grep Mac-mini
```

## Related Documentation

- [Watchdog README](../watchdog/README.md) - Process supervision
- [Client README](../client/README.md) - VPN client
- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design
