# Skill: Event Bus CLI

Manage and observe the VPN event bus system using the `eventbus-cli` tool.

## When to Use

Use this skill when:
- User wants to check client versions across all machines
- User wants to trigger updates via WebSocket
- User needs to view event logs (Splunk-like queries)
- User wants system health/statistics
- User asks about connected peers or deployment status
- User wants to see latency/throughput benchmarks
- User wants historical performance data (time series)

## Prerequisites

The daemon must be running:
```bash
# Check if daemon is running
pgrep -lf eventbus-daemon

# Start daemon if not running
./bin/eventbus-daemon &
```

## Commands

### 1. System Status & Health

```bash
# Quick daemon status
./bin/eventbus-cli status

# Full system health
./bin/eventbus-cli health
```

### 2. Client Versions

```bash
# See all client versions
./bin/eventbus-cli versions
```

### 3. Connected Peers

```bash
# List all connected VPN peers
./bin/eventbus-cli peers
```

### 4. Event Logs (Splunk-like)

```bash
# View all logs
./bin/eventbus-cli logs

# Filter by time range
./bin/eventbus-cli logs --last 2h
./bin/eventbus-cli logs --last 30m
./bin/eventbus-cli logs --last 1d

# Filter by hostname
./bin/eventbus-cli logs MacBook-Air.local

# Filter by namespace
./bin/eventbus-cli logs --ns versions
./bin/eventbus-cli logs --ns health

# Combine filters
./bin/eventbus-cli logs --last 2h --ns versions
```

### 5. Client Statistics

```bash
# View all client stats
./bin/eventbus-cli stats

# Stats for specific client
./bin/eventbus-cli stats MacBook-Air.local
```

### 6. Trigger Updates (WebSocket-based)

```bash
# Update all clients
./bin/eventbus-cli update all

# Update specific client
./bin/eventbus-cli update 10.8.0.4

# Update multiple clients
./bin/eventbus-cli update 10.8.0.4 10.8.0.5

# Rollback to specific version
./bin/eventbus-cli rollback 10.8.0.4 --to abc123
```

### 7. Watch Events Live

```bash
# Watch all events in real-time
./bin/eventbus-cli watch

# Subscribe to specific patterns
./bin/eventbus-cli subscribe "versions.*"
./bin/eventbus-cli subscribe "health.*"
./bin/eventbus-cli subscribe "*"
```

### 8. Latency & Throughput Benchmarks

Benchmarks run automatically every hour when idle (no traffic for 30s).

```bash
# View benchmark summary
./bin/eventbus-cli benchmark

# Manually trigger a benchmark
./bin/eventbus-cli benchmark run

# Check scheduler status
./bin/eventbus-cli benchmark status

# Get latest benchmark results
./bin/eventbus-cli benchmark latest
```

Example output:
```json
{
  "latency": {"avg": 281.03, "min": 281.03, "max": 281.03},
  "throughput": {"avg": 172.35, "min": 103.45, "max": 241.26}
}
```

### 9. Time Series Data (Historical)

Time series store 7 days of historical data with smart bucketing.
- Latency: 5-minute buckets (2016 points max)
- Throughput: 15-minute buckets (672 points max)

```bash
# View time series summary
./bin/eventbus-cli timeseries

# Get latency time series (for charts)
./bin/eventbus-cli timeseries latency

# Get throughput time series (for charts)
./bin/eventbus-cli timeseries throughput
```

Example output:
```json
{
  "series": "latency",
  "count": 2,
  "points": [
    {"ts": "2025-11-30T16:35:00-03:00", "avg": 288.01, "min": 288.01, "max": 288.01},
    {"ts": "2025-11-30T16:40:00-03:00", "avg": 281.03, "min": 281.03, "max": 281.03}
  ]
}
```

## Event Namespaces

| Namespace | Description |
|-----------|-------------|
| `versions.*` | Version reports from clients |
| `health.*` | Ping, latency, uptime events |
| `updates.*` | Update notifications |
| `peers.*` | Peer join/leave events |
| `system.*` | Connect, disconnect, snapshot |
| `benchmark.*` | Latency/throughput measurements |

## WebSocket Update Flow

```
Developer pushes to Git
        │
        ▼
Server detects change (monitorGitUpdates)
        │
        ▼
Server broadcasts updates.available via WebSocket
        │
        ▼
Clients receive event and run auto-update.sh
        │
        ▼
Clients re-report version to server
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| "Daemon not running" | Start with `./bin/eventbus-daemon &` |
| No events in logs | Events accumulate over time, wait or generate activity |
| WebSocket disconnected | Check VPN connection, daemon will auto-reconnect |
| Update not triggering | Check client logs at `/tmp/family-vpn-client.log` |
| Benchmark not running | Benchmarks only run when idle (no traffic for 30s). Force with `benchmark run` |
| No time series data | Benchmarks populate time series. Run `benchmark run` to generate initial data |
| High latency reported | Normal range is 50-300ms. Check VPN connection if consistently above 500ms |
