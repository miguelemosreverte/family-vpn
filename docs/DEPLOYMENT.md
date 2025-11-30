# Deployment Guide

This document describes how to deploy and update the Family VPN system.

## Overview

Family VPN uses a **continuous deployment** model:
1. Push changes to `main` branch
2. Server broadcasts update notification
3. Clients automatically pull, rebuild, and restart
4. Dashboard shows rollout progress

## Initial Installation

### On a New Client Machine

```bash
# 1. Clone the repository
git clone https://github.com/yourusername/family-vpn.git ~/Desktop/family-vpn

# 2. Create .env file with configuration
cat > ~/Desktop/family-vpn/.env << 'EOF'
VPN_SERVER=95.217.238.72
VPN_PORT=443
SUDO_PASSWORD=your_password_here
EOF

# 3. Run install script
cd ~/Desktop/family-vpn
./install.sh
```

### What Install Does

1. Builds all Go binaries (client, watchdog, menu bar)
2. Installs binaries to `/usr/local/bin/`
3. Packages Electron app to `/Applications/`
4. Creates LaunchAgent for watchdog
5. Starts all services

## Updating

### Automatic Updates (Preferred)

1. Push changes to `main` branch:
   ```bash
   git add -A
   git commit -m "Your changes"
   git push origin main
   ```

2. Trigger update broadcast:
   ```bash
   curl -k -X POST "https://95.217.238.72:443/update/init?component=all"
   ```

   Or use the "Update All Clients" button in the dashboard.

3. Clients will automatically:
   - Receive update notification via WebSocket
   - Run `./client/auto-update.sh`
   - Pull latest changes
   - Rebuild binaries
   - Restart services

### Manual Update (Single Client)

SSH to the client and run:

```bash
cd ~/Desktop/family-vpn
./client/auto-update.sh
```

Or for a full reinstall:

```bash
cd ~/Desktop/family-vpn
./uninstall.sh
git pull origin main
./install.sh
```

## Update Components

### Component-Specific Updates

You can update specific components instead of everything:

```bash
# Update only clients
curl -k -X POST "https://95.217.238.72:443/update/init?component=client"

# Update only desktop app
curl -k -X POST "https://95.217.238.72:443/update/init?component=desktop"

# Update only UI (hot-reload, no restart)
curl -k -X POST "https://95.217.238.72:443/update/init?component=ui"
```

### Update Layers

| Layer | Component | Update Method |
|-------|-----------|---------------|
| 4 | UI (HTML/CSS/JS) | Hot-reload, no restart |
| 3 | Extensions | IPC message, plugin reload |
| 2 | Desktop Shell | Electron renderer reload |
| 1 | VPN Core | Process restart |
| 0 | Protocol | Full reinstall |

## Rollback

### Quick Rollback

```bash
# Revert the last commit
git revert HEAD
git push origin main

# Trigger update to all clients
curl -k -X POST "https://95.217.238.72:443/update/init?component=all"
```

### Rollback to Specific Version

```bash
# Find the commit you want
git log --oneline

# Reset to that commit
git reset --hard abc1234
git push origin main --force

# Trigger update
curl -k -X POST "https://95.217.238.72:443/update/init?component=all"
```

## Monitoring Deployment

### Check Client Versions

```bash
# Using eventbus-cli
./bin/eventbus-cli versions

# Using API
curl -k https://95.217.238.72:443/api/versions
```

### Dashboard

Open the Family VPN dashboard and go to the **Versions** tab to see:
- Current version of each client
- Which clients are updating
- Which clients are behind

### Expected Output

All clients should show the same git commit hash:

```
Client Versions:
  MacBook-Air:    abc1234 ✓
  Mac-mini:       abc1234 ✓
  Anastasia:      abc1234 ✓
```

## Troubleshooting

### Client Not Updating

1. Check if client is connected:
   ```bash
   curl -k https://95.217.238.72:443/api/peers
   ```

2. SSH to client and check logs:
   ```bash
   ssh user@client "tail -50 /tmp/family-vpn-client.log"
   ```

3. Manually trigger update:
   ```bash
   ssh user@client "cd ~/Desktop/family-vpn && ./client/auto-update.sh"
   ```

### Git Conflicts

If a client has local changes that prevent pulling:

```bash
ssh user@client "cd ~/Desktop/family-vpn && git stash && git pull origin main"
```

Or force reset:

```bash
ssh user@client "cd ~/Desktop/family-vpn && git fetch && git reset --hard origin/main"
```

### Build Failures

Check Go version:
```bash
ssh user@client "go version"
```

Check for missing dependencies:
```bash
ssh user@client "cd ~/Desktop/family-vpn/client && go mod tidy && go build -v"
```

## Server Deployment

The VPN server (95.217.238.72) is updated separately:

```bash
# SSH to server
ssh root@95.217.238.72

# Update server
cd /opt/family-vpn
git pull origin main
cd server
go build -o vpn-server .
systemctl restart vpn-server
```

## Best Practices

1. **Test locally first**: Build and run on your machine before pushing
2. **Small commits**: Make small, incremental changes
3. **Monitor rollout**: Watch the dashboard during updates
4. **Keep backups**: Note the commit hash before major changes
5. **Staged rollout**: For risky changes, update one client first

## CI/CD Integration

For automated deployments, you can use GitHub Actions or similar:

```yaml
# .github/workflows/deploy.yml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Trigger update
        run: |
          curl -k -X POST "https://95.217.238.72:443/update/init?component=all"
```

## Related Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) - System design
- [Client README](../client/README.md) - Auto-update details
- [PROJECT.md](../PROJECT.md) - Project overview
