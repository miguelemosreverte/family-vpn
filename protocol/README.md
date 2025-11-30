# Protocol Layer (Layer 0)

The protocol layer defines the foundational event-driven communication system used by all Family VPN components.

## Stability: STABLE

This is the foundation. Changes require comprehensive testing.

## Overview

The Event Bus enables real-time, namespaced communication between all components:
- VPN clients report versions and health
- Server broadcasts updates
- Dashboard receives and displays state
- All communication is event-driven

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Event Bus                        │
├─────────────────────────────────────────────────────┤
│                                                     │
│   Publishers              Subscribers               │
│   ─────────              ───────────                │
│   VPN Client    ──────►  Dashboard                  │
│   VPN Server    ──────►  All Clients                │
│   Dashboard     ──────►  Server                     │
│                                                     │
│                    WebSocket                        │
│                   (Transport)                       │
│                                                     │
└─────────────────────────────────────────────────────┘
```

## Files

| File | Description |
|------|-------------|
| `events.go` | Go implementation (server, clients) |
| `events.js` | JavaScript implementation (Electron) |
| `update.go` | Update protocol for hot-reload |
| `update.js` | Update protocol for JavaScript |

## Event Format

```json
{
  "ns": "versions.client",
  "ts": 1234567890123,
  "seq": 42,
  "data": {
    "hostname": "MacBook-Air.local",
    "version": "abc123"
  }
}
```

| Field | Description |
|-------|-------------|
| `ns` | Namespace (e.g., `versions.client`) |
| `ts` | Unix timestamp in milliseconds |
| `seq` | Sequence number for ordering |
| `data` | Event-specific payload |

## Namespaces

| Namespace | Description |
|-----------|-------------|
| `system.connect` | Client connected |
| `system.disconnect` | Client disconnected |
| `system.snapshot` | Periodic state snapshot |
| `health.ping` | Ping result |
| `health.latency` | Latency measurement |
| `versions.client` | Client version update |
| `versions.server` | Server version update |
| `updates.available` | Update notification |
| `peers.joined` | Peer joined network |
| `peers.left` | Peer left network |

## Subscription Patterns

```javascript
// Subscribe to all version events
bus.subscribe('versions.*', handler);

// Subscribe to specific event
bus.subscribe('health.ping', handler);

// Subscribe to everything
bus.subscribe('*', handler);
```

## Snapshots

Every 5 minutes, the server broadcasts a consolidated state snapshot:

```json
{
  "id": "20231129T123456Z",
  "ts": 1234567890123,
  "state": {
    "versions": {
      "client1": "abc123",
      "client2": "def456"
    },
    "health": {
      "uptime": 99.9,
      "lastPing": 1234567890
    }
  },
  "last_seq": 42
}
```

New clients receive the latest snapshot on connect, then incremental events.

## Go Usage

```go
import "github.com/family-vpn/protocol"

// Create event bus
eb := protocol.NewEventBus(5 * time.Minute)
defer eb.Stop()

// Register state collector
eb.RegisterStateCollector("versions", func() interface{} {
    return getClientVersions()
})

// Subscribe to events
eb.Subscribe("versions.*", func(e protocol.Event) {
    log.Printf("Version: %s", e.Namespace)
})

// Publish event
eb.Publish(protocol.NSVersionsClient, map[string]string{
    "hostname": hostname,
    "version":  gitCommit,
})
```

## JavaScript Usage

```javascript
const { createEventBus, NS, SUB } = require('./protocol/events.js');

const bus = createEventBus({
  url: 'wss://95.217.238.72:443/ws',
  subscriptions: SUB.VERSIONS,
  onSnapshot: (snapshot) => {
    renderVersions(snapshot.get('versions'));
  }
});

// Subscribe to events
bus.subscribe('versions.*', (event) => {
  updateVersionDisplay(event);
});

// Connect
bus.connect();
```

## Update Protocol

The update protocol defines how hot-reload works:

```
Layer 4: UI Components (HTML/CSS/JS) - instant reload
Layer 3: Extensions (Plugins) - IPC reload
Layer 2: Desktop Shell (Electron) - renderer reload
Layer 1: VPN Core (Go binary) - process restart
Layer 0: This protocol - always on
```

## Testing

```bash
cd protocol
go test -v ./...
```

## Related Documentation

- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design
- [Client README](../client/README.md) - VPN client
- [Server README](../server/README.md) - VPN server
