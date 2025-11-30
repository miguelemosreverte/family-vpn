# Family VPN - Project Map

> A resilient, self-updating VPN system for family networks with real-time monitoring.

## Quick Navigation

| Document | Purpose |
|----------|---------|
| [CLAUDE.md](./CLAUDE.md) | Architecture philosophy & development guidelines |
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | System design & component interaction |
| [docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md) | Deployment & continuous delivery |
| [protocol/README.md](./protocol/README.md) | Layer 0 EventBus protocol |
| [client/README.md](./client/README.md) | VPN client documentation |
| [server/README.md](./server/README.md) | VPN server documentation |
| [desktop-app/README.md](./desktop-app/README.md) | Electron dashboard |
| [watchdog/README.md](./watchdog/README.md) | Process supervisor |

---

## Project Structure

```
family-vpn/
├── .claude/                    # Claude Code automation
│   ├── skills/                 # Reusable skills for Claude
│   │   ├── deploy.md          # Deployment skill
│   │   └── test.md            # Testing skill
│   └── commands/              # Slash commands
│       ├── update-all.md      # /update-all command
│       └── status.md          # /status command
│
├── protocol/                   # Layer 0: Shared protocols (STABLE)
│   ├── events.go              # EventBus Go implementation
│   ├── events.js              # EventBus JS implementation
│   ├── update.go              # Update protocol
│   └── README.md              # Protocol documentation
│
├── client/                     # VPN Client (Go)
│   ├── main.go                # Client entry point
│   ├── auto-update.sh         # Self-update script
│   └── README.md              # Client documentation
│
├── server/                     # VPN Server (Go)
│   ├── main.go                # Server entry point
│   └── README.md              # Server documentation
│
├── desktop-app/               # Electron Dashboard
│   ├── main.js                # Electron main process
│   ├── preload.js             # IPC bridge
│   ├── renderer/              # UI components
│   │   ├── index.html         # Dashboard HTML
│   │   ├── app.js             # Dashboard logic
│   │   └── styles.css         # Styling
│   └── README.md              # Dashboard documentation
│
├── menu-bar/                  # Menu Bar App (Go + systray)
│   ├── main.go                # Menu bar entry point
│   └── README.md              # Menu bar documentation
│
├── watchdog/                  # Process Supervisor
│   ├── main.go                # Watchdog entry point
│   └── README.md              # Watchdog documentation
│
├── docs/                      # Documentation
│   ├── ARCHITECTURE.md        # System architecture
│   └── DEPLOYMENT.md          # Deployment guide
│
├── install.sh                 # Installation script
├── uninstall.sh               # Uninstallation script
├── CLAUDE.md                  # AI assistant guidelines
└── PROJECT.md                 # This file
```

---

## Artifact Stability Levels

Components are classified by stability to guide development:

| Level | Status | Description |
|-------|--------|-------------|
| **STABLE** | Can build on | Protocol layer, tested & reliable |
| **MATURING** | Use with care | Core functionality works, edge cases being refined |
| **DEVELOPING** | In progress | Active development, API may change |
| **EXPERIMENTAL** | Do not depend | Prototype, may be removed |

### Current Status

| Component | Stability | Notes |
|-----------|-----------|-------|
| `protocol/events.go` | STABLE | EventBus core - do not modify without tests |
| `protocol/events.js` | STABLE | JS client - mirrors Go implementation |
| `client/main.go` | MATURING | VPN tunnel stable, WebSocket updates maturing |
| `server/main.go` | MATURING | Core routing stable |
| `desktop-app/` | DEVELOPING | Dashboard functional, features being added |
| `watchdog/` | MATURING | Process supervision works |
| `menu-bar/` | DEVELOPING | Basic functionality |

---

## Key Concepts

### 1. Satellite Programming Model
Clients are deployed remotely and must be **autonomous**:
- Self-healing after failures
- Self-updating without manual intervention
- Always maintain WebSocket connection for remote management

### 2. Layer 0 EventBus
All real-time communication uses namespaced events:
```
system.*     - Connect/disconnect/snapshot
health.*     - Ping, latency, uptime
versions.*   - Client/server versions
updates.*    - Update notifications
peers.*      - Peer discovery
```

### 3. Update Protocol
```
Developer pushes → Server broadcasts → Clients update → Dashboard reflects
```

---

## Development Workflow

### Making Changes

1. **Check stability level** of component you're modifying
2. **Read component README** for specific guidelines
3. **Run tests** before committing
4. **Use slash commands** for common operations:
   - `/update-all` - Trigger update on all clients
   - `/status` - Check system status

### Continuous Deployment

Changes to `main` branch trigger automatic updates:
1. Server detects git push
2. Broadcasts `updates.available` via EventBus
3. Clients pull, rebuild, restart
4. Dashboard shows new versions

---

## Getting Started

### First Time Setup
```bash
./install.sh
```

### Development
```bash
# Build all components
cd client && go build -o vpn-client .
cd ../server && go build -o vpn-server .
cd ../watchdog && go build -o family-vpn-watchdog .
```

### Testing
```bash
# Run protocol tests
cd protocol && go test -v ./...
```

---

## Support

- **Issues**: Check `/tmp/family-vpn-*.log` files
- **Dashboard**: Open Family VPN app → Health tab
- **Manual update**: `./client/auto-update.sh`

---

*Last updated: 2025-11-30*
