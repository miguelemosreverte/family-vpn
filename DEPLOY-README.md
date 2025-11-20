# ⚠️ DEPLOYMENT GUIDE - READ THIS FIRST ⚠️

## CRITICAL: Never Manually Restart VPN Components

The VPN core is **stable and working**. From this point forward:

❌ **NEVER** manually restart the VPN client
❌ **NEVER** manually restart extensions
❌ **NEVER** manually SSH to machines and rebuild

✅ **ALWAYS** use the automated deployment system

---

## The Correct Deployment Process

### After Making Code Changes:

```bash
# 1. Commit your changes
git add .
git commit -m "Your changes"

# 2. Push to GitHub
git push

# 3. Deploy using the automated system
./deploy.sh
```

That's it! The system will:
- Auto-detect which component(s) changed
- Broadcast component-specific update signals
- Hot-reload ONLY the affected components
- Keep the VPN core running throughout

---

## How Component Detection Works

The `deploy.sh` script analyzes `git diff` to detect changes:

| Changed Files | Component Signal | What Restarts |
|--------------|------------------|---------------|
| `server/*` | `UPDATE_VPN` | Server only |
| `client/*`, `ipc/*` | `UPDATE_VPN` | VPN client only |
| `extensions/video/*` | `UPDATE_VIDEO` | Video extension only |
| `extensions/ssh/*` | `UPDATE_SSH` | SSH extension only |
| `menu-bar/*` | `UPDATE_MENU` | Menu-bar app only |
| Multiple files | Multiple signals | Only affected components |

---

## Manual Component Deployment (if needed)

If you need to deploy a specific component without auto-detection:

```bash
./deploy.sh video      # Deploy only video extension
./deploy.sh ssh        # Deploy only SSH extension
./deploy.sh menu       # Deploy only menu-bar app
./deploy.sh vpn        # Deploy only VPN core
./deploy.sh all        # Deploy everything (rarely needed)
```

---

## What Happens When You Deploy

### Server Side:
1. Receives POST to `/update/init?component=<name>`
2. Broadcasts `UPDATE_<COMPONENT>` to all connected VPN clients
3. If component is `vpn` or `all`, server rebuilds and restarts itself

### Client Side:
1. VPN client receives control message via encrypted tunnel
2. VPN client writes signal file: `~/.family-vpn-update-signal`
3. Menu-bar app detects signal (polls every 2 seconds)
4. Extension manager handles the update:
   - Stops the extension
   - Runs `git pull origin main`
   - Rebuilds the extension binary
   - Starts the new extension
5. **VPN core keeps running throughout!**

---

## Verification

After deployment, check the menu-bar logs for:

```
🔔 Update signal received from VPN server: UPDATE_<COMPONENT>
[UPDATE] Restarting extension: <name>
[EXT] Stopping extension: <name>
[EXT] Rebuilding extension: <name>
[EXT] Successfully rebuilt: <name>
[EXT] Started extension: <name> (PID XXXXX)
[EXT] Successfully restarted extension: <name>
```

---

## Example: Successful SSH Extension Update

```
2025/11/20 03:15:50 [CONTROL] Received: UPDATE_SSH
2025/11/20 03:15:51 🔔 Update signal received from VPN server: UPDATE_SSH
2025/11/20 03:15:51 [UPDATE] Restarting extension: ssh
2025/11/20 03:15:51 [EXT] Stopping extension: ssh
2025/11/20 03:15:52 [EXT] Rebuilding extension: ssh
2025/11/20 03:15:55 [EXT] Successfully rebuilt: ssh
2025/11/20 03:15:55 [EXT] Started extension: ssh (PID 43305)
2025/11/20 03:15:55 [EXT] Successfully restarted extension: ssh
```

Notice: **VPN stayed running, only SSH extension restarted!**

---

## Troubleshooting

**Server not responding to /update/init:**
```bash
# Test the endpoint
curl -X POST "http://95.217.238.72:9000/update/init?component=test"

# Expected response:
# Update initiated for component: test
```

**Client not receiving update signal:**
- Ensure VPN is connected (clients must be on VPN to receive broadcasts)
- Check VPN client logs for: `[CONTROL] Received: UPDATE_<COMPONENT>`
- Manually test: `echo "UPDATE_SSH" > ~/.family-vpn-update-signal`

**Extension fails to rebuild:**
- Check extension manager logs for build errors
- Ensure `go` is in PATH
- Verify extension directory exists

---

## Why This System Exists

**Before (Manual Approach):**
- Developer manually rebuilds binaries
- Developer SSHs to each machine
- Developer manually restarts VPN clients
- **VPN connection drops during restart**
- Extensions restart unnecessarily
- Error-prone and time-consuming

**After (Automated Component-Aware Deployment):**
- Developer runs `./deploy.sh`
- System auto-detects what changed
- Only affected components restart
- **VPN stays up, no connection drop**
- All machines update simultaneously
- Reliable and fast

---

## Summary

✅ Use `./deploy.sh` after every code change
✅ Trust the automated system
✅ VPN core stability is maintained
✅ Hot-reload keeps everything running smoothly

❌ Never manually restart VPN components
❌ Never SSH to rebuild manually
❌ Never bypass the deployment system
