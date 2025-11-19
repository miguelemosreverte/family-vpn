# Deployment Guide

## First-Time Server Setup (Manual - ONE TIME ONLY)

SSH to the server and run these commands:

```bash
# SSH to server
ssh root@95.217.238.72

# Navigate to repo (or clone if not exists)
cd /root/family-vpn || git clone https://github.com/miguelemosreverte/family-vpn.git /root/family-vpn
cd /root/family-vpn

# Pull latest code
git pull origin main

# Make scripts executable
chmod +x server-update.sh

# Build server
cd server
go build -o /root/family-vpn/vpn-server main.go

# Stop old server if running
pkill vpn-server || true

# Start new server with update endpoint
cd /root/family-vpn
nohup ./vpn-server -port 8888 -webhook-port 9000 > /var/log/vpn-server.log 2>&1 &

# Save PID
echo $! > /var/run/vpn-server.pid

# Verify server is running
sleep 2
tail -20 /var/log/vpn-server.log
```

You should see:
```
Starting HTTP server on port 9000
  - POST /webhook - GitHub webhook endpoint
  - POST /update/init - Trigger server and client updates
VPN server listening on :8888 (encryption: false)
```

## All Future Deployments (Automated)

After the initial setup, just run:

```bash
./deploy.sh
```

This will:
1. Ping the server's `/update/init` endpoint
2. Server pulls latest code, rebuilds, and restarts
3. Server broadcasts UPDATE_AVAILABLE to all connected clients
4. Clients automatically pull, rebuild, and restart

## How It Works

### Deployment Flow
```
Developer                Server                  Clients
    |                       |                        |
    |-- POST /update/init ->|                        |
    |<------ 200 OK ---------|                        |
    |                       |-- UPDATE_AVAILABLE -->|
    |                       |                        |-- git pull + rebuild
    |                       |-- git pull             |
    |                       |-- go build             |
    |                       |-- restart              |
    |                       |                        |-- restart
```

### Update Endpoint (`/update/init`)

When you POST to `http://95.217.238.72:9000/update/init`:

1. **Immediately** broadcasts `UPDATE_AVAILABLE` to all connected VPN clients
2. **Responds** with success (so deployment script completes)
3. **Background process** executes `server-update.sh`:
   - `git pull origin main`
   - `go build -o vpn-server server/main.go`
   - Kills old server process
   - Starts new server with nohup

### Client Auto-Update

Clients connected to VPN:
1. Receive `CTRL:UPDATE_AVAILABLE` packet through encrypted VPN tunnel
2. Write signal file: `~/.family-vpn-update-signal`
3. Menu bar app detects signal (checks every 5 seconds)
4. Runs `performUpdate()`:
   - `git pull origin main`
   - `./build-menubar.sh`
   - Restarts menu bar app
5. User sees new version in About menu

## Testing

After running `./deploy.sh`, check:

1. **Server logs**: `ssh root@95.217.238.72 'tail -f /var/log/vpn-server.log'`
2. **Client logs**: Check menu bar app console output
3. **Version**: Open About menu - should show latest commit

## Troubleshooting

**Server not responding to /update/init:**
- Check server is running: `ssh root@95.217.238.72 'ps aux | grep vpn-server'`
- Check port 9000 is listening: `ssh root@95.217.238.72 'netstat -tlnp | grep 9000'`
- Check firewall allows port 9000

**Clients not updating:**
- Ensure VPN is connected (clients must be connected to receive broadcast)
- Check client logs for `[CONTROL] Received: UPDATE_AVAILABLE`
- Manually trigger: `echo "update" > ~/.family-vpn-update-signal`

**Server update script fails:**
- Check logs: `ssh root@95.217.238.72 'tail -50 /var/log/vpn-server.log'`
- Verify script is executable: `ssh root@95.217.238.72 'ls -la /root/family-vpn/server-update.sh'`
- Test script manually: `ssh root@95.217.238.72 '/root/family-vpn/server-update.sh'`
