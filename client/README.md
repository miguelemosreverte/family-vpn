# VPN Client

The VPN client establishes encrypted tunnels and maintains WebSocket connections for remote management.

## Stability: MATURING

Core VPN tunnel is stable. WebSocket updates and auto-update features are maturing.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    VPN Client                       │
├─────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │  VPN Tunnel │  │  WebSocket  │  │ Auto-Update │ │
│  │   (tun0)    │  │  Connection │  │   Handler   │ │
│  └─────────────┘  └─────────────┘  └─────────────┘ │
│         │                │                │        │
│         └────────────────┼────────────────┘        │
│                          ▼                          │
│                   Event Bus                         │
│              (protocol/events.go)                   │
└─────────────────────────────────────────────────────┘
```

## Files

| File | Description |
|------|-------------|
| `main.go` | Client entry point and VPN logic |
| `auto-update.sh` | Self-update script triggered by server |

## Build

```bash
cd client
go build -o vpn-client .
```

## Install Location

- Binary: `/usr/local/bin/vpn-client`
- Config: `~/Desktop/family-vpn/.env`
- Log: `/tmp/family-vpn-client.log`

## Command Line Options

```bash
vpn-client [options]

Options:
  --no-timeout    Disable connection timeout
  --server HOST   VPN server address (default: 95.217.238.72)
  --port PORT     VPN server port (default: 443)
```

## WebSocket Events

### Subscribed Events

| Event | Action |
|-------|--------|
| `updates.available` | Triggers auto-update.sh |
| `system.snapshot` | Reports current version |

### Published Events

| Event | Data |
|-------|------|
| `versions.client` | Hostname, git commit hash |
| `health.ping` | Latency measurement |

## Auto-Update Flow

```
1. Server broadcasts updates.available
2. Client receives via WebSocket
3. Client runs auto-update.sh
4. Script: git pull && rebuild && restart
5. Client reports new version
```

## Troubleshooting

### VPN Tunnel Not Establishing

```bash
# Check if client is running
pgrep -lf 'vpn-client'

# Check logs
tail -50 /tmp/family-vpn-client.log

# Check tun0 interface
ifconfig tun0
```

### WebSocket Disconnected

```bash
# Check server reachability
ping -c 1 95.217.238.72

# Check logs for WebSocket errors
grep -i websocket /tmp/family-vpn-client.log | tail -20
```

### Manual Update

```bash
cd ~/Desktop/family-vpn
./client/auto-update.sh
```

## Related Documentation

- [Protocol README](../protocol/README.md) - Event Bus specification
- [Watchdog README](../watchdog/README.md) - Process supervision
- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design
