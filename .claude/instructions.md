# Family VPN - Claude Code Instructions

**READ THIS BEFORE MAKING ANY CHANGES TO THIS CODEBASE**

This document contains critical information about the architecture, deployment process, and workflows for the Family VPN project. Following these instructions is **MANDATORY** to maintain system stability.

---

## 🚨 CRITICAL RULES - NEVER BREAK THESE

### 1. NEVER Manually Restart VPN Components

❌ **ABSOLUTELY FORBIDDEN:**
- `killall vpn-client && ./vpn-client` (DON'T DO THIS)
- `sudo pkill vpn-client` (DON'T DO THIS)
- SSHing to machines and manually rebuilding (DON'T DO THIS)
- Restarting the menu-bar app to "apply changes" (DON'T DO THIS)

✅ **ALWAYS USE:**
```bash
./deploy.sh
```

**Why this matters:**
- The VPN core is **stable and working**
- Manual restarts break the hot-reload system
- Manual restarts drop VPN connections for all users
- Component-aware deployment keeps everything running smoothly

### 2. ALWAYS Use Component-Aware Deployment

After making ANY code change:

```bash
# 1. Commit changes
git add .
git commit -m "Description"

# 2. Push to GitHub
git push

# 3. Deploy (REQUIRED - do not skip!)
./deploy.sh
```

The deployment script will:
- Auto-detect which components changed
- Send component-specific update signals
- Hot-reload ONLY affected components
- Keep VPN running throughout

---

## 📁 Project Architecture

### Core Components

```
family-vpn/
├── server/          → VPN server (runs on 95.217.238.72)
├── client/          → VPN client (runs on each MacBook)
├── menu-bar/        → macOS menu-bar app (manages client + extensions)
├── extensions/      → Hot-reloadable feature modules
│   ├── video/       → Video calling extension
│   ├── ssh/         → SSH terminal extension
│   └── framework/   → Extension base framework
├── ipc/             → Inter-process communication (client ↔ extensions)
└── deploy.sh        → Component-aware deployment script
```

### How Components Interact

```
┌─────────────────┐
│   Menu-Bar App  │ (Manages everything)
└────────┬────────┘
         │ starts
         ├──────────┐
         │          │
    ┌────▼────┐  ┌─▼──────────────┐
    │  VPN    │  │  Extensions    │
    │ Client  │  │ (video, ssh)   │
    │         │  │                │
    │ + IPC   │◄─┤ Poll IPC for   │
    │ Server  │  │ signals        │
    └────┬────┘  └────────────────┘
         │
         │ encrypted tunnel
         │
    ┌────▼────┐
    │  VPN    │ (Remote server)
    │ Server  │
    └─────────┘
```

**Key Points:**
1. **VPN Client** runs as root (needs TUN device access)
2. **IPC Server** runs inside VPN client (port 8889)
3. **Extensions** run as normal user, communicate via IPC
4. **Menu-bar** manages lifecycle of client + extensions

---

## 🔄 Deployment System

### Component Detection

The `deploy.sh` script analyzes `git diff` to detect what changed:

| Changed Files | Component | Update Signal | What Restarts |
|---------------|-----------|---------------|---------------|
| `server/*` | vpn | `UPDATE_VPN` | Server only |
| `client/*`, `ipc/*` | vpn | `UPDATE_VPN` | VPN client only |
| `extensions/video/*` | video | `UPDATE_VIDEO` | Video extension only |
| `extensions/ssh/*` | ssh | `UPDATE_SSH` | SSH extension only |
| `menu-bar/*` | menu | `UPDATE_MENU` | Menu-bar app only |
| `extensions/newext/*` | newext | `UPDATE_NEWEXT` | New extension only |

### Deployment Flow

```
Developer              Server (95.217.238.72)        Clients (MacBooks)
    │                           │                            │
    │  1. git push              │                            │
    │  2. ./deploy.sh           │                            │
    │                           │                            │
    ├─ POST /update/init?───────►                            │
    │  component=ssh            │                            │
    │                           │                            │
    │◄─ HTTP 200 ───────────────┤                            │
    │                           │                            │
    │                           ├─ Broadcast: ───────────────►│
    │                           │  UPDATE_SSH                │
    │                           │                            │
    │                           │                            ├─ Write signal file:
    │                           │                            │  ~/.family-vpn-update-signal
    │                           │                            │
    │                           │                            ├─ Menu-bar detects signal
    │                           │                            │
    │                           │                            ├─ Extension manager:
    │                           │                            │  • Stop SSH extension
    │                           │                            │  • git pull
    │                           │                            │  • go build
    │                           │                            │  • Start new SSH
    │                           │                            │
    │                           │                            ├─ VPN STAYS RUNNING ✅
```

### Manual Component Deployment

If you need to deploy a specific component:

```bash
./deploy.sh video    # Deploy only video extension
./deploy.sh ssh      # Deploy only SSH extension
./deploy.sh menu     # Deploy only menu-bar app
./deploy.sh vpn      # Deploy VPN core (server + client)
./deploy.sh all      # Deploy everything (rarely needed)
```

---

## 🧪 Testing Changes

### Before Asking User to Test

**CRITICAL:** Always verify features work BEFORE asking the user to test!

#### For Extension Changes:
1. Deploy using `./deploy.sh`
2. Check logs for successful restart:
   ```bash
   # Watch menu-bar logs
   tail -f /path/to/menu-bar.log | grep UPDATE
   ```
3. Verify extension restarted with new PID
4. Test functionality yourself via SSH or automation
5. **Only then** ask user to test

#### For Video Calling Changes:
1. Deploy using `./deploy.sh`
2. Run automated end-to-end test:
   ```bash
   ./test-video-calling.sh
   ```
3. Verify:
   - Signal sent from machine A
   - Signal received on machine B
   - Browser opens on both machines
   - Call connection established
4. **Only then** ask user to test

### Automated Testing Tools

```bash
# Test video calling end-to-end
./test-video-calling.sh

# Test SSH connectivity
./test-ssh-connectivity.sh

# Test deployment system
./test-deployment.sh

# Monitor real-time updates
tail -f ~/.family-vpn-update-signal
```

---

## 🐛 Common Scenarios

### Scenario: Video Calling Not Working

**DON'T:**
- Restart VPN client manually
- SSH to machines and rebuild

**DO:**
1. Check if it's a code issue or config issue
2. If code needs change, make the change
3. Commit and push
4. Deploy using `./deploy.sh`
5. Run automated test to verify
6. Ask user to test

### Scenario: New Extension Created

**DON'T:**
- Manually copy files to machines
- Manually register extension

**DO:**
1. Create extension in `extensions/<name>/`
2. Register in `menu-bar/main.go`:
   ```go
   extPath := filepath.Join(repoDir, "extensions", "name", "name-extension")
   extensionManager.RegisterExtension("name", extPath, []string{"--vpn-port", "8889"})
   ```
3. Commit and push
4. Deploy using `./deploy.sh`
5. The system will auto-build and start the extension

### Scenario: VPN Client Bug Fix

**DON'T:**
- Rebuild client manually on each machine
- Kill and restart VPN

**DO:**
1. Fix bug in `client/main.go`
2. Commit and push
3. Deploy using `./deploy.sh`
4. System will:
   - Broadcast `UPDATE_VPN` to all clients
   - Each client rebuilds and restarts
   - VPN reconnects automatically

---

## 📊 Monitoring and Logs

### Check Deployment Worked

After running `./deploy.sh`, verify on clients:

```bash
# Watch for update signal
tail -f /path/to/menu-bar.log | grep -E "UPDATE|Restart"

# Expected output:
# [CONTROL] Received: UPDATE_<COMPONENT>
# 🔔 Update signal received from VPN server: UPDATE_<COMPONENT>
# [UPDATE] Restarting extension: <name>
# [EXT] Stopping extension: <name>
# [EXT] Rebuilding extension: <name>
# [EXT] Successfully rebuilt: <name>
# [EXT] Started extension: <name> (PID XXXXX)
```

### Check Component Status

```bash
# VPN client status
curl -s http://localhost:8889/health

# List peers
curl -s http://localhost:8889/peers | python3 -m json.tool

# Extension processes
ps aux | grep -E "video-extension|ssh-extension" | grep -v grep
```

---

## 🔧 Development Workflow

### 1. Understanding the Change Scope

Before making changes, identify which component is affected:

| Change Type | Component | Deploy Command |
|-------------|-----------|----------------|
| Add video feature | video extension | `./deploy.sh video` |
| Fix VPN routing | vpn client | `./deploy.sh vpn` |
| Update menu UI | menu-bar | `./deploy.sh menu` |
| Add SSH feature | ssh extension | `./deploy.sh ssh` |
| Server performance | vpn server | `./deploy.sh vpn` |

### 2. Making Changes

```bash
# 1. Create feature branch (optional but recommended)
git checkout -b feature/my-change

# 2. Make your changes
vim extensions/video/main.go

# 3. Test locally if possible
# (for extensions, you can run them standalone for basic testing)

# 4. Commit with descriptive message
git add .
git commit -m "Add feature X to video extension

- Describe what changed
- Why it changed
- How to test it
"

# 5. Push
git push origin feature/my-change  # or main

# 6. Deploy
./deploy.sh
```

### 3. Verifying Deployment

```bash
# Wait 30 seconds for propagation
sleep 30

# Check logs
tail -50 /path/to/menu-bar.log | grep UPDATE

# Verify component restarted
ps aux | grep video-extension
```

---

## 🚀 Extension Development

### Creating a New Extension

1. **Create extension directory:**
   ```bash
   mkdir -p extensions/myext
   cd extensions/myext
   ```

2. **Create main.go using framework:**
   ```go
   package main

   import (
       "github.com/miguelemosreverte/family-vpn/extensions/framework"
       "github.com/miguelemosreverte/family-vpn/ipc"
   )

   type MyExtension struct {
       *framework.ExtensionBase
       vpnClient *ipc.VPNClient
   }

   func NewMyExtension(vpnPort int) *MyExtension {
       return &MyExtension{
           ExtensionBase: framework.NewExtensionBase("myext", "1.0.0"),
           vpnClient:     ipc.NewVPNClient(vpnPort),
       }
   }

   func (e *MyExtension) Start() error {
       // Your extension logic here
       return nil
   }

   func main() {
       vpnPort := flag.Int("vpn-port", 8889, "VPN client IPC port")
       flag.Parse()

       ext := NewMyExtension(*vpnPort)
       if err := ext.Start(); err != nil {
           log.Fatal(err)
       }

       select {} // Keep running
   }
   ```

3. **Register in menu-bar:**
   ```go
   // In menu-bar/main.go
   myextPath := filepath.Join(repoDir, "extensions", "myext", "myext-extension")
   extensionManager.RegisterExtension("myext", myextPath, []string{"--vpn-port", "8889"})
   ```

4. **Deploy:**
   ```bash
   git add .
   git commit -m "Add myext extension"
   git push
   ./deploy.sh
   ```

---

## 🔐 Security Notes

### SSH Access
- Passwordless SSH is configured between family VPN peers
- SSH keys are automatically set up via `/ssh/setup-ssh-key` endpoint
- `~/.ssh/config` auto-trusts VPN IPs (10.8.0.*)

### IPC Security
- IPC server (port 8889) is localhost-only
- Extensions must be on same machine to communicate
- No external access to IPC

### VPN Encryption
- AES-256-GCM encryption for all VPN traffic
- TLS on port 443 (looks like HTTPS)
- Server certificate validation

---

## 📚 Key Files Reference

### Deployment
- `deploy.sh` - Component-aware deployment script ⭐ USE THIS
- `DEPLOY-README.md` - Comprehensive deployment guide
- `.git/hooks/post-push` - Reminds to deploy after push

### VPN Core
- `client/main.go` - VPN client (runs as root, creates TUN device)
- `client/ipc_server.go` - IPC server for extension communication
- `server/main.go` - VPN server (95.217.238.72)

### Menu-Bar App
- `menu-bar/main.go` - Menu-bar app (manages VPN + extensions)
- `menu-bar/extension_manager.go` - Hot-reload extension manager

### Extensions
- `extensions/video/` - Video calling via WebRTC
- `extensions/ssh/` - SSH terminal access to peers
- `extensions/framework/` - Base framework for extensions

### Configuration
- `.env` - Server host, credentials (in private gist)
- `~/.ssh/config` - SSH auto-trust for VPN IPs

---

## 🎯 Success Checklist

Before asking user to test a feature:

- [ ] Code changes committed and pushed
- [ ] `./deploy.sh` executed successfully
- [ ] Logs show component restarted on all machines
- [ ] VPN stayed running (no restart unless UPDATE_VPN)
- [ ] Automated tests pass (if available)
- [ ] Feature tested via SSH or automation
- [ ] **ONLY THEN:** Ask user to test

---

## ❓ FAQ

**Q: Can I quickly test a change by rebuilding locally?**
A: NO. Always use `./deploy.sh`. Local rebuilds bypass the deployment system and won't update remote machines.

**Q: The deploy script says "No component changes detected" but I made changes.**
A: The script analyzes the latest commit. Make sure you committed your changes first.

**Q: Can I deploy to just one machine for testing?**
A: No. Deployments always go to all machines to maintain consistency. Use feature flags if you need gradual rollout.

**Q: What if the server is down?**
A: The deployment will fail with a connection error. Check server status:
```bash
curl -X POST http://95.217.238.72:9000/update/init?component=test
```

**Q: How do I roll back a deployment?**
A:
```bash
git revert HEAD
git push
./deploy.sh
```

---

## 🆘 Emergency Procedures

### If VPN is Completely Broken

Only in absolute emergency when VPN won't start:

1. **On affected machine:**
   ```bash
   cd ~/Desktop/family-vpn
   git pull origin main
   cd client
   go build -o vpn-client .
   cd ../menu-bar
   go build -o family-vpn-menubar .
   ```

2. **Restart menu-bar app**

3. **Then immediately:**
   - Document what went wrong
   - Fix the root cause
   - Deploy fix using `./deploy.sh`

### If Server is Down

SSH to server (requires credentials):
```bash
ssh root@95.217.238.72
cd /root/family-vpn
./server-update.sh
```

---

## 📝 Summary

**The Golden Rule:**
```bash
git push && ./deploy.sh
```

**Never bypass this. Ever.**

The component-aware deployment system is the foundation of this project's reliability. It ensures:
- ✅ Changes deploy to all machines simultaneously
- ✅ Only affected components restart
- ✅ VPN stays running unless absolutely necessary
- ✅ No manual intervention needed
- ✅ Consistent state across all machines

**Trust the system. Use the system. The system works.**
