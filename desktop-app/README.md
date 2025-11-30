# Desktop App (Electron Dashboard)

The desktop app is an Electron-based dashboard for monitoring and managing the Family VPN system.

## Stability: DEVELOPING

Dashboard is functional. New features and tabs are being added actively.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Electron App                      │
├─────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────┐│
│  │               Main Process                      ││
│  │  (main.js - Node.js, IPC, Window Management)   ││
│  └─────────────────────────────────────────────────┘│
│                        │                            │
│                   IPC Bridge                        │
│                  (preload.js)                       │
│                        │                            │
│  ┌─────────────────────────────────────────────────┐│
│  │             Renderer Process                    ││
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐          ││
│  │  │  Home   │ │ Health  │ │Versions │ ...      ││
│  │  │   Tab   │ │   Tab   │ │   Tab   │          ││
│  │  └─────────┘ └─────────┘ └─────────┘          ││
│  │                                                 ││
│  │              WebSocket Client                   ││
│  │           (protocol/events.js)                  ││
│  └─────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────┘
```

## Files

| File | Description |
|------|-------------|
| `main.js` | Electron main process |
| `preload.js` | IPC bridge for security |
| `renderer/index.html` | Dashboard HTML |
| `renderer/app.js` | Dashboard logic |
| `renderer/styles.css` | Dashboard styling |
| `package.json` | Dependencies and scripts |

## Dashboard Tabs

| Tab | Description |
|-----|-------------|
| Home | Overview and quick actions |
| Health | Ping history, latency charts, uptime |
| Versions | Client versions, update status |
| Volumes | Storage management (legacy) |

## Features

### Real-time Updates

The dashboard connects via WebSocket and receives live updates:

```javascript
// Subscribes to all version events
bus.subscribe('versions.*', updateVersionDisplay);

// Subscribes to health events
bus.subscribe('health.*', updateHealthDisplay);
```

### Update All Button

Triggers server API to broadcast updates to all clients:

```javascript
// POST https://95.217.238.72:443/update/init?component=all
triggerUpdateAll();
```

## Build & Run

```bash
cd desktop-app

# Install dependencies
npm install

# Run in development
npm start

# Package for distribution
npm run package
```

## Install Location

- App: `/Applications/Family VPN.app`
- Data: `~/Library/Application Support/Family VPN/`

## Development

### Hot Reload

During development, changes to renderer files can be reloaded:

```bash
# In the app: Cmd+R to reload renderer
# Or use electron-reload for auto-refresh
```

### DevTools

Open Electron DevTools for debugging:
- `Cmd+Option+I` in the app
- Or via menu: View → Toggle DevTools

### IPC Communication

```javascript
// From renderer to main
window.api.send('channel-name', data);

// From main to renderer
mainWindow.webContents.send('channel-name', data);
```

## Troubleshooting

### App Not Starting

```bash
# Check if app exists
ls -la "/Applications/Family VPN.app"

# Try launching from terminal
"/Applications/Family VPN.app/Contents/MacOS/Family VPN"

# Check for crashes
tail -50 ~/Library/Logs/DiagnosticReports/Family\ VPN*
```

### WebSocket Not Connecting

```bash
# Check server reachability
curl -k https://95.217.238.72:443/api/peers

# Open DevTools (Cmd+Option+I) and check Console tab
```

### Blank Screen

```bash
# Clear app cache
rm -rf ~/Library/Application\ Support/Family\ VPN/

# Reinstall
cd ~/Desktop/family-vpn && ./install.sh
```

## Related Documentation

- [Protocol README](../protocol/README.md) - Event Bus specification
- [Watchdog README](../watchdog/README.md) - Process supervision
- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - System design
