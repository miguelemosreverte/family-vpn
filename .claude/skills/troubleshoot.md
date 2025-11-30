# Skill: Troubleshoot

Diagnose and fix issues with the Family VPN system.

## When to Use

Use this skill when:
- Something isn't working
- User reports an error or problem
- Client is disconnected or unresponsive
- Updates aren't propagating
- Dashboard shows issues

## Diagnostic Flow

```
1. Identify the symptom
2. Check relevant logs
3. Verify component status
4. Apply fix
5. Verify resolution
```

## Common Issues & Solutions

### 1. Client Not Connected

**Symptoms**: Client missing from dashboard, can't SSH

**Diagnosis**:
```bash
# Check if machine is reachable
ping -c 1 {ip} 2>/dev/null && echo "Reachable" || echo "Unreachable"

# If reachable, SSH and check
ssh {user}@{host} "pgrep -lf vpn-client"
```

**Solutions**:
- Machine sleeping: Wake it up
- Client crashed: Restart via watchdog or manually
- Network issue: Check local network

### 2. VPN Tunnel Not Establishing

**Symptoms**: Client runs but no tunnel

**Diagnosis**:
```bash
ssh {user}@{host} "ifconfig tun0 2>/dev/null || echo 'No tunnel'"
ssh {user}@{host} "tail -50 /tmp/family-vpn-client.log | grep -i error"
```

**Solutions**:
- Permission denied: Run with sudo
- Server unreachable: Check server status
- TUN device issue: Reinstall TUN driver

### 3. Update Not Working

**Symptoms**: Client stuck on old version

**Diagnosis**:
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && git status && git log -1 --format='%h'"
```

**Solutions**:
- Git conflicts: `git stash && git pull`
- No auto-update.sh: Ensure file exists and is executable
- WebSocket disconnected: Check connection

### 4. WebSocket Disconnected

**Symptoms**: No real-time updates, dashboard stale

**Diagnosis**:
```bash
ssh {user}@{host} "tail -50 /tmp/family-vpn-client.log | grep -i websocket"
```

**Solutions**:
- Server not running: Check server status
- Certificate issue: Verify SSL
- Firewall: Check port 443

### 5. Watchdog Not Running

**Symptoms**: Processes don't restart after crash

**Diagnosis**:
```bash
ssh {user}@{host} "pgrep -lf watchdog && launchctl list | grep watchdog"
```

**Solutions**:
```bash
# Restart watchdog
ssh {user}@{host} "launchctl load ~/Library/LaunchAgents/com.family.vpn.watchdog.plist"
```

### 6. Desktop App Not Starting

**Symptoms**: No dashboard, app doesn't open

**Diagnosis**:
```bash
ssh {user}@{host} "open -a 'Family VPN' 2>&1"
```

**Solutions**:
- App corrupted: Reinstall
- Login items missing: Add to login items
- Watchdog not managing: Check watchdog config

## Log File Locations

| Component | Log File |
|-----------|----------|
| VPN Client | `/tmp/family-vpn-client.log` |
| Watchdog | `/tmp/family-vpn-watchdog.log` |
| Menu Bar | `/tmp/family-vpn-menubar.log` |
| Desktop App | Electron DevTools Console |

## Recovery Commands

### Full Reset (Local)
```bash
cd ~/Desktop/family-vpn
./uninstall.sh
git pull origin main
./install.sh
```

### Full Reset (Remote)
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && ./uninstall.sh && git pull origin main && ./install.sh"
```

### Force Update
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && git fetch && git reset --hard origin/main && ./install.sh"
```

## Escalation

If issue persists:
1. Check server logs at 95.217.238.72
2. Review recent commits for breaking changes
3. Consider rollback to previous version
