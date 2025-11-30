# VPN Server

The VPN server handles encrypted tunnels, WebSocket connections, and update broadcasts.

## Stability: MATURING

Core routing is stable. API endpoints and update broadcasting are maturing.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    VPN Server                       │
├─────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │  VPN Tunnel │  │  WebSocket  │  │  HTTP API   │ │
│  │   Router    │  │    Hub      │  │  Endpoints  │ │
│  └─────────────┘  └─────────────┘  └─────────────┘ │
│         │                │                │        │
│         └────────────────┼────────────────┘        │
│                          ▼                          │
│                   Event Bus                         │
│              (protocol/events.go)                   │
└─────────────────────────────────────────────────────┘
```

## Deployment

- **Host**: 95.217.238.72 (Hetzner Cloud)
- **Port**: 443 (HTTPS/WSS)
- **Protocol**: TLS-encrypted

## API Endpoints

### Update API

```bash
# Trigger update broadcast to all clients
POST /update/init?component={component}

# Components: all, client, server, desktop, ui
curl -k -X POST "https://95.217.238.72:443/update/init?component=all"
```

### Status API

```bash
# Get connected peers
GET /api/peers
curl -k https://95.217.238.72:443/api/peers

# Get client versions
GET /api/versions
curl -k https://95.217.238.72:443/api/versions
```

## WebSocket Events

### Published Events

| Event | Trigger |
|-------|---------|
| `updates.available` | POST /update/init |
| `system.snapshot` | Every 5 minutes |
| `peers.joined` | Client connects |
| `peers.left` | Client disconnects |

### Subscribed Events

| Event | Action |
|-------|--------|
| `versions.client` | Updates version registry |
| `health.ping` | Updates health metrics |

## Build

```bash
cd server
go build -o vpn-server .
```

## Configuration

Environment variables:

| Variable | Description |
|----------|-------------|
| `VPN_PORT` | VPN tunnel port |
| `WS_PORT` | WebSocket port |
| `TLS_CERT` | Path to TLS certificate |
| `TLS_KEY` | Path to TLS private key |

## Monitoring

```bash
# Check server process
ssh root@95.217.238.72 "pgrep -lf vpn-server"

# View server logs
ssh root@95.217.238.72 "tail -50 /var/log/vpn-server.log"

# Check connected clients
curl -k https://95.217.238.72:443/api/peers
```

## Related Documentation

- [Protocol README](../protocol/README.md) - Event Bus specification
- [DEPLOYMENT.md](../docs/DEPLOYMENT.md) - Deployment guide
- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design
