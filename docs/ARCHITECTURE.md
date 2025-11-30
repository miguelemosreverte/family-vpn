# System Architecture

This document describes the complete architecture of the Family VPN system.

## Overview

Family VPN is a resilient, self-updating VPN system designed for family networks. It follows a **satellite programming model** where clients are deployed remotely and must be autonomous after deployment.

## Core Principles

1. **Resilience Through Layers** - Multiple fallback mechanisms at every critical junction
2. **Autonomous Clients** - Self-healing, self-updating, minimal manual intervention
3. **Connection Preservation** - WebSocket connection is sacred, never break it
4. **Event-Driven Communication** - All components communicate via Event Bus

## System Diagram

```
                              ┌─────────────────────────┐
                              │      VPN Server         │
                              │   (95.217.238.72:443)   │
                              │                         │
                              │  ┌───────────────────┐  │
                              │  │   Event Bus Hub   │  │
                              │  └───────────────────┘  │
                              │           │             │
                              │  ┌────────┴────────┐    │
                              │  │                 │    │
                              │  ▼                 ▼    │
                              │ VPN              HTTP   │
                              │ Tunnel           API    │
                              └──────────┬──────────────┘
                                         │
                      ┌──────────────────┼──────────────────┐
                      │                  │                  │
                      ▼                  ▼                  ▼
           ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
           │  Client Machine  │ │  Client Machine  │ │  Client Machine  │
           │   (MacBook Air)  │ │    (Mac mini)    │ │   (Anastasia)    │
           │                  │ │                  │ │                  │
           │ ┌──────────────┐ │ │ ┌──────────────┐ │ │ ┌──────────────┐ │
           │ │   Watchdog   │ │ │ │   Watchdog   │ │ │ │   Watchdog   │ │
           │ └──────────────┘ │ │ └──────────────┘ │ │ └──────────────┘ │
           │        │         │ │        │         │ │        │         │
           │ ┌──────┴───────┐ │ │ ┌──────┴───────┐ │ │ ┌──────┴───────┐ │
           │ │              │ │ │ │              │ │ │ │              │ │
           │ ▼              ▼ │ │ ▼              ▼ │ │ ▼              ▼ │
           │ VPN      Desktop │ │ VPN      Desktop │ │ VPN      Desktop │
           │ Client      App  │ │ Client      App  │ │ Client      App  │
           └──────────────────┘ └──────────────────┘ └──────────────────┘
```

## Component Responsibilities

### VPN Server (95.217.238.72)

| Responsibility | Description |
|----------------|-------------|
| VPN Routing | Route encrypted traffic between clients |
| Event Bus Hub | Central hub for all WebSocket connections |
| Update Broadcast | Send update notifications to all clients |
| Peer Registry | Track connected clients and their versions |

### VPN Client

| Responsibility | Description |
|----------------|-------------|
| VPN Tunnel | Establish encrypted tunnel (tun0) |
| WebSocket | Maintain persistent connection to server |
| Version Reporting | Report git commit hash to server |
| Auto-Update | Pull and rebuild when notified |

### Watchdog

| Responsibility | Description |
|----------------|-------------|
| Process Monitoring | Check VPN client, menu bar, desktop app |
| Auto-Restart | Restart crashed components |
| Health Logging | Log component status |
| Update Coordination | Restart processes after updates |

### Desktop App (Electron)

| Responsibility | Description |
|----------------|-------------|
| Dashboard | Visualize health, versions, peers |
| Real-time Updates | Receive events via WebSocket |
| User Actions | Trigger updates, view logs |
| Notifications | Alert user to issues |

### Menu Bar App

| Responsibility | Description |
|----------------|-------------|
| Status Indicator | Show VPN connection status |
| Quick Actions | Connect/disconnect, SSH to peers |
| Peer List | Show connected family members |

## Communication Layers

```
┌─────────────────────────────────────────────────────────────┐
│                    Layer 4: UI Components                   │
│              (HTML/CSS/JS - hot reload instantly)           │
├─────────────────────────────────────────────────────────────┤
│                    Layer 3: Extensions                      │
│             (Plugins - reload via IPC message)              │
├─────────────────────────────────────────────────────────────┤
│                  Layer 2: Desktop Shell                     │
│             (Electron - reload renderer only)               │
├─────────────────────────────────────────────────────────────┤
│                    Layer 1: VPN Core                        │
│          (Go binary - restart only for breaking)            │
├─────────────────────────────────────────────────────────────┤
│                    Layer 0: Event Bus                       │
│               (Always on, routes all messages)              │
└─────────────────────────────────────────────────────────────┘
```

## Event Flow

### Update Propagation

```
Developer                Server                 Clients              Dashboard
    │                       │                      │                     │
    │ git push main         │                      │                     │
    │──────────────────────►│                      │                     │
    │                       │                      │                     │
    │                       │ updates.available    │                     │
    │                       │─────────────────────►│                     │
    │                       │                      │                     │
    │                       │                      │ git pull            │
    │                       │                      │ rebuild             │
    │                       │                      │ restart             │
    │                       │                      │                     │
    │                       │   versions.client    │                     │
    │                       │◄─────────────────────│                     │
    │                       │                      │                     │
    │                       │                      │   versions.client   │
    │                       │─────────────────────────────────────────►│
    │                       │                      │                     │
```

### Health Monitoring

```
Client                    Server                Dashboard
   │                         │                      │
   │ health.ping             │                      │
   │────────────────────────►│                      │
   │                         │                      │
   │                         │ health.ping          │
   │                         │─────────────────────►│
   │                         │                      │
   │                         │                      │ Update charts
   │                         │                      │ Update tables
   │                         │                      │
```

## Data Flow

### Snapshots

Every 5 minutes, the server broadcasts a consolidated state snapshot:

```json
{
  "id": "20231129T123456Z",
  "ts": 1234567890123,
  "state": {
    "versions": {
      "macbook-air": "abc1234",
      "mac-mini": "abc1234",
      "anastasia": "abc1234"
    },
    "health": {
      "uptime": 99.9
    },
    "peers": [
      {"ip": "10.8.0.2", "hostname": "macbook-air"},
      {"ip": "10.8.0.3", "hostname": "mac-mini"},
      {"ip": "10.8.0.10", "hostname": "anastasia"}
    ]
  }
}
```

### Incremental Events

Between snapshots, individual events update state:

```json
{
  "ns": "versions.client",
  "ts": 1234567890123,
  "seq": 42,
  "data": {
    "hostname": "mac-mini",
    "version": "def5678"
  }
}
```

## Resilience Patterns

### Connection Recovery

```
Disconnect → Wait 1s → Reconnect
                │
                ▼
             Failed → Wait 2s → Reconnect
                         │
                         ▼
                      Failed → Wait 4s → Reconnect
                                   │
                                   ▼
                                Continue with exponential backoff...
```

### Process Recovery (Watchdog)

```
Check every 5s:
  ├─ VPN Client running? → No → Start VPN Client
  ├─ Menu Bar running?   → No → Start Menu Bar
  └─ Desktop App running? → No → Start Desktop App
```

### Update Recovery

```
Update received:
  ├─ git pull
  │     └─ Failed? → git stash && git pull
  ├─ go build
  │     └─ Failed? → Log error, keep running old version
  └─ restart
        └─ Failed? → Watchdog will restart
```

## Network Topology

```
                    Internet
                        │
                        ▼
              ┌─────────────────┐
              │  VPN Server     │
              │  95.217.238.72  │
              │                 │
              │  VPN: 10.8.0.1  │
              └────────┬────────┘
                       │
          ┌────────────┼────────────┐
          │            │            │
          ▼            ▼            ▼
     ┌─────────┐  ┌─────────┐  ┌─────────┐
     │10.8.0.2 │  │10.8.0.3 │  │10.8.0.10│
     │ MacBook │  │Mac mini │  │Anastasia│
     └─────────┘  └─────────┘  └─────────┘
```

## File System Layout

```
~/Desktop/family-vpn/
├── .claude/                 # Claude Code automation
│   ├── skills/              # Reusable skills
│   └── commands/            # Slash commands
├── protocol/                # Layer 0 Event Bus
├── client/                  # VPN Client (Go)
├── server/                  # VPN Server (Go)
├── desktop-app/             # Electron Dashboard
├── menu-bar/                # Menu Bar App (Go)
├── watchdog/                # Process Supervisor (Go)
├── docs/                    # Documentation
├── bin/                     # Built binaries
├── install.sh               # Installation script
├── uninstall.sh             # Uninstallation script
├── PROJECT.md               # Project map
└── CLAUDE.md                # AI guidelines

/usr/local/bin/
├── vpn-client               # Installed VPN client
├── family-vpn-watchdog      # Installed watchdog
└── family-vpn-menubar       # Installed menu bar

/Applications/
└── Family VPN.app           # Installed Electron app

~/Library/LaunchAgents/
└── com.family.vpn.watchdog.plist  # Watchdog launchd config

/tmp/
├── family-vpn-client.log    # VPN client log
├── family-vpn-watchdog.log  # Watchdog log
└── family-vpn-menubar.log   # Menu bar log
```

## Security Model

### VPN Tunnel

- OpenVPN protocol over TLS
- Certificate-based authentication
- All traffic encrypted end-to-end

### WebSocket

- WSS (WebSocket Secure) over TLS
- Same certificates as VPN
- Event data is application-level, not sensitive

### SSH Access

- Key-based authentication between family machines
- Used for remote debugging and management
- StrictHostKeyChecking disabled for convenience

## Performance Considerations

### Event Bus

- Events are lightweight JSON messages
- Snapshots consolidate state to reduce bandwidth
- Sequence numbers enable replay and ordering

### Watchdog

- 5-second check interval balances responsiveness and CPU usage
- Process checks use pgrep (low overhead)
- Logs are minimal to avoid disk I/O

### Updates

- Git pull is incremental (only changed files)
- Go builds are fast (< 10 seconds typically)
- Restarts are coordinated to minimize downtime

## Related Documentation

- [PROJECT.md](../PROJECT.md) - Project map and navigation
- [CLAUDE.md](../CLAUDE.md) - Development philosophy
- [Protocol README](../protocol/README.md) - Event Bus specification
- [Client README](../client/README.md) - VPN client details
- [Server README](../server/README.md) - VPN server details
