# Family VPN - Architecture & Development Philosophy

## Project Philosophy

### Core Principles

1. **Resilience Through Layers of Defense**
   - Multiple fallback mechanisms at every critical junction
   - Graceful degradation when components fail
   - Self-healing systems that recover automatically

2. **Satellite Programming Model**
   - Assume clients are remote and inaccessible after deployment
   - All systems must be autonomous and self-sufficient
   - Manual intervention should be minimal to none post-deployment
   - Dogfooding: We test by using the system as if it's already in production

3. **Zero-Breakage Deployment**
   - Changes must not break existing functionality
   - All clients must remain connected during updates
   - WebSocket-based update propagation ensures coordination
   - Git commit tags define versions for lockstep advancement

## System Architecture

### Clear Boundaries of Responsibility

#### 1. WebSocket Connection (NON-NEGOTIABLE)
- **Responsibility**: Maintain persistent connection between client and server
- **Scope**: ALL clients at ALL times, regardless of VPN status
- **Priority**: HIGHEST - This is the lifeline for remote management
- **Fallback**: Automatic reconnection with exponential backoff
- **Monitoring**: Connection health tracked and visualized in dashboard

#### 2. VPN Client
- **Responsibility**: Encrypted tunnel for data traffic
- **Scope**: Network routing through VPN server
- **Dependency**: Can operate independently of WebSocket
- **Fallback**: Managed by Watchdog (see below)

#### 3. Watchdog Service
- **Responsibility**:
  - Monitor VPN health and connectivity
  - Manage internet access fallback
  - Ensure system remains functional
- **Behavior**:
  - If VPN is offline → Enable normal Wi-Fi internet access
  - If VPN server is unreachable → Route through local gateway
  - Monitor all components (VPN client, menu bar, desktop app)
  - Auto-restart crashed components
  - Auto-update when new versions are available
- **Philosophy**: Resilience over perfection
  - Better to have normal internet than no internet
  - Better to reconnect than to stay disconnected
  - Better to update gracefully than to require manual intervention

#### 4. Desktop Application (Electron)
- **Responsibility**: User interface and visualization
- **Dashboards**:
  - Home (placeholder for future features)
  - Health Monitoring (client/server status, ping history, uptime charts)
  - Storage/Volumes (existing functionality)
  - Version Control (Git commit tracking, update status)
- **Navigation**: Left sidebar for dashboard switching
- **Updates**: Receives state changes via WebSocket

#### 5. Menu Bar App
- **Responsibility**: Quick access and status indicator
- **Features**:
  - VPN connection status
  - Peer list with SSH access
  - Quick actions (connect/disconnect, settings)
- **Integration**: Communicates with VPN client via IPC

## Dashboard Requirements

### Health Monitoring Dashboard

**Purpose**: Real-time visibility into system health

**Metrics to Display**:
1. **Client Health**
   - WebSocket connection status (connected/disconnected)
   - VPN tunnel status (active/inactive)
   - Last seen timestamp
   - Current IP address (VPN and local)

2. **Server Health**
   - Server reachability (ping success/failure)
   - Response time (latency)
   - Active connections count
   - Server version

3. **Ping History & Charts**
   - Last 100 pings (table view)
   - Last hour (line chart, 1-minute intervals)
   - Last 24 hours (line chart, 5-minute intervals)
   - Uptime percentage calculations

4. **Visual Indicators**
   - Green: Healthy (< 100ms latency)
   - Yellow: Degraded (100-500ms latency)
   - Red: Unhealthy (> 500ms or timeout)

### Version Control Dashboard

**Purpose**: Track deployment status across all clients

**Metrics to Display**:
1. **Current Versions**
   - Server: Git commit hash + tag
   - Each Client: Git commit hash + tag
   - Visual diff if versions mismatch

2. **Update Status**
   - Clients currently updating (in progress)
   - Clients pending update (queued)
   - Clients up-to-date (success)
   - Failed updates (error state)

3. **Commit Timeline Chart**
   - X-axis: Time
   - Y-axis: Clients
   - Visual representation of version advancement
   - Goal: All clients moving in lockstep

4. **Deployment Health**
   - Success rate of recent deployments
   - Average time to deploy across all clients
   - Manual intervention count (goal: zero)

## WebSocket Protocol

### Message Types

```javascript
// Client → Server
{
  type: "health_ping",
  timestamp: 1234567890,
  vpn_status: "connected",
  version: "abc123def"
}

// Server → Client
{
  type: "update_available",
  version: "def456ghi",
  changelog: "Bug fixes and improvements"
}

// Bidirectional
{
  type: "metrics_report",
  data: {
    latency: 45,
    packet_loss: 0.01,
    uptime: 99.99
  }
}
```

### Connection Guarantee
- Reconnect on disconnect (exponential backoff)
- Heartbeat every 30 seconds
- Timeout detection at 90 seconds
- State synchronization on reconnect

## Deployment Strategy

### Update Propagation

1. **Developer pushes to Git**
2. **Server detects new commit**
3. **Server sends WebSocket notification to all clients**
4. **Clients pull and rebuild autonomously**
5. **Clients report update status back to server**
6. **Dashboard visualizes rollout progress**

### Safety Mechanisms

1. **Staged Rollout**: Update 1 client first, validate, then propagate
2. **Rollback Capability**: Keep previous version binary for quick revert
3. **Health Checks**: Verify connectivity post-update before marking success
4. **Connection Preservation**: Update must not break WebSocket connection

## Development Guidelines

### Before Making Changes

1. Read this document thoroughly
2. Identify which component(s) you're modifying
3. Understand the responsibility boundaries
4. Plan for failure modes and fallbacks

### During Implementation

1. **Never break WebSocket connection** - This is the golden rule
2. Test locally before deploying remotely
3. Add logging for debugging (but not too verbose)
4. Update dashboards to reflect new metrics
5. Document new WebSocket message types

### After Implementation

1. Deploy to one test client first
2. Monitor health dashboard for anomalies
3. Verify all metrics are reporting correctly
4. Roll out to remaining clients
5. Update this document if architecture changed

## Code Organization

### Directory Structure

```
family-vpn/
├── client/              # VPN client (Go)
├── server/              # VPN server (Go)
├── menu-bar/            # Menu bar app (Go + systray)
├── desktop-app/         # Electron dashboard
│   ├── dashboards/
│   │   ├── home.js
│   │   ├── health.js
│   │   ├── volumes.js
│   │   └── versions.js
│   └── components/
│       ├── Sidebar.js
│       └── Chart.js
├── watchdog/            # Watchdog service (Go)
├── extensions/          # Extensions (SSH, video, etc.)
├── install.sh           # Automated installation
├── uninstall.sh         # Clean uninstall
└── CLAUDE.md            # This file
```

### Component Communication

```
┌─────────────┐         WebSocket          ┌─────────────┐
│   Desktop   │◄──────────────────────────►│   Server    │
│     App     │                             │             │
└─────────────┘                             └─────────────┘
      ▲                                            ▲
      │ IPC                                        │ VPN Tunnel
      ▼                                            ▼
┌─────────────┐         IPC/Signals        ┌─────────────┐
│  Menu Bar   │◄──────────────────────────►│ VPN Client  │
└─────────────┘                             └─────────────┘
      ▲                                            ▲
      │ Process Monitoring                         │ Process Monitoring
      └────────────────┬───────────────────────────┘
                       ▼
                ┌─────────────┐
                │  Watchdog   │
                └─────────────┘
```

## Monitoring & Observability

### What to Log

1. **Connection Events**: Connect, disconnect, reconnect attempts
2. **Health Checks**: Ping results, latency measurements
3. **Version Changes**: Update starts, successes, failures
4. **Errors**: With context and stack traces
5. **Performance**: Resource usage, memory, CPU

### What to Visualize

1. **Real-time**: Current status, active connections
2. **Historical**: Charts, trends, patterns
3. **Comparative**: Client-to-client, current-vs-previous
4. **Predictive**: Anomaly detection, degradation warnings

### Log Locations

- Menu bar: `/tmp/family-vpn-menubar.log`
- Desktop app: Electron DevTools Console
- VPN client: `/tmp/family-vpn-client.log`
- Watchdog: `/tmp/family-vpn-watchdog.log`

## Future Enhancements

### Planned Features

1. **Intelligent Fallback**: Multiple VPN servers with automatic failover
2. **Bandwidth Monitoring**: Track data usage per client
3. **Security Alerts**: Detect and report suspicious activity
4. **Mobile Support**: iOS/Android clients with same philosophy
5. **Cloud Dashboard**: Web-based monitoring for remote management

### Research Areas

1. **Mesh Networking**: Peer-to-peer fallback when server is unreachable
2. **AI-Driven Routing**: Optimize paths based on latency/bandwidth
3. **Zero-Trust Security**: Enhanced authentication and authorization
4. **Performance Optimization**: Reduce latency, increase throughput

## Success Metrics

### Operational Excellence

- **Uptime**: > 99.9% across all clients
- **Update Success Rate**: > 99% without manual intervention
- **Mean Time to Recovery**: < 5 minutes for any component failure
- **Manual Interventions**: < 1 per month across all clients

### Development Velocity

- **Deploy Frequency**: Multiple times per day without breakage
- **Lead Time**: < 30 minutes from commit to full rollout
- **Change Failure Rate**: < 5% of deployments require rollback
- **Recovery Time**: < 10 minutes when rollback is needed

## Conclusion

This project embodies the principle of **resilient autonomy**. Every decision should be evaluated against these questions:

1. What happens if this component fails?
2. Can it recover automatically?
3. Will clients remain connected during changes?
4. Can I deploy this remotely without manual access?
5. How will I know if something goes wrong?

When in doubt, prioritize **connection preservation** over feature completeness. A connected client with degraded features is infinitely more valuable than a disconnected client with perfect features.

---

**Last Updated**: 2025-11-29
**Version**: 1.0.0
**Maintainer**: Family VPN Team
