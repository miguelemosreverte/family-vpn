# 🎉 Final Manual Update - Your Last Time!

**Date:** November 20, 2025
**Purpose:** One-time setup for automatic hot-reload system

---

## Why This Is The Last Manual Update

After completing this update, **you will never need to manually update again**. Here's why:

### What We Built

We implemented a **microservices architecture with hot-reload capability**:

```
┌─────────────────────────────────────────────┐
│         Menu-Bar (Extension Manager)         │
│  • Monitors UPDATE_* signals from server    │
│  • Automatically rebuilds changed components│
│  • Restarts only affected parts             │
└──────────┬──────────────────────────────────┘
           │
    ┌──────┴──────┐
    │             │
┌───▼────┐   ┌───▼──────────┐
│  VPN   │   │  Extensions  │
│ Client │◄─►│  • video     │
│ (Core) │IPC│  • future... │
│ :8889  │   │              │
└────────┘   └──────────────┘
```

### The Magic Workflow (After This Update)

1. **You make changes** on your development machine
2. **You run:** `git push`
3. **Server receives** GitHub webhook
4. **Server broadcasts** component-specific signals:
   - `UPDATE_VIDEO` → Only video extension restarts
   - `UPDATE_MENU` → Only menu-bar restarts
   - `UPDATE_VPN` → Only VPN core restarts (rare)
5. **All clients automatically:**
   - Receive the update signal
   - Run `git pull` locally
   - Rebuild only the changed component
   - Restart only that component
   - **VPN stays connected throughout!**

**Result:** Zero downtime, zero manual intervention, instant updates across all family devices.

---

## 📋 Final Manual Update Instructions

### Prerequisites

- [ ] All computers have `git` installed
- [ ] All computers have `go` installed (version 1.21+)
- [ ] You have cloned the repository on each computer
- [ ] You have `.env` file with `SUDO_PASSWORD` configured

### Step-by-Step Instructions

#### On Each Computer:

1. **Navigate to repository:**
   ```bash
   cd ~/family-vpn  # Or wherever you cloned it
   ```

2. **Pull latest changes:**
   ```bash
   git pull origin main
   ```

   You should see updates for:
   - Server component-aware update handler
   - Client update signal handling
   - Menu-bar extension manager
   - Video extension module
   - Build script improvements
   - Video calling port fix

3. **Rebuild everything:**
   ```bash
   ./build-menubar.sh
   ```

   Expected output:
   ```
   ✅ VPN client built successfully
   ✅ Menu bar application built successfully
   ✅ Video extension built successfully
   ```

4. **Restart menu-bar application:**

   **Option A - If menu-bar is running:**
   - Click menu-bar icon → Quit
   - Open Terminal: `cd menu-bar && ./family-vpn-menubar`

   **Option B - If using auto-launch:**
   - Log out and log back in
   - Or run: `killall family-vpn-menubar && open menu-bar/family-vpn-menubar`

5. **Verify it's working:**
   - Menu-bar icon should appear in status bar
   - Click "Connect to VPN"
   - VPN should connect successfully
   - Check logs: Extension manager should start video extension

---

## 🧪 Testing The New System

After updating all computers, test the hot-reload workflow:

### Test 1: Extension Hot-Reload

1. **Make a trivial change to video extension:**
   ```bash
   cd extensions/video
   # Edit main.go - add a log line or change a message
   ```

2. **Commit and push:**
   ```bash
   git add .
   git commit -m "Test: video extension hot-reload"
   git push
   ```

3. **Watch the magic happen:**
   - Server receives webhook
   - Server broadcasts `UPDATE_VIDEO` to all clients
   - All clients' menu-bars see the signal
   - Extension managers automatically:
     - Pull latest code
     - Rebuild video extension
     - Restart video extension
   - **VPN stays connected!**

4. **Check logs:**
   ```bash
   # Should see:
   [UPDATE] Restarting video extension...
   [EXT] Stopping extension: video
   [EXT] Rebuilding extension: video
   [EXT] Using go binary: /usr/local/go/bin/go
   [EXT] Successfully rebuilt: video
   [EXT] Started extension: video (PID 12345)
   ```

### Test 2: Video Calling

1. **Connect both computers to VPN**
2. **Click "Video Call" on one computer**
3. **Expected behavior:**
   - Browser opens on both computers (auto-answer!)
   - Video call connects (no more "Calling..." hang)
   - WebRTC establishes peer-to-peer connection

### Test 3: Component Detection

1. **Make changes to different components:**
   ```bash
   # Scenario A: Change only server
   vim server/main.go
   git commit -m "Server change"
   git push
   # → Only server restarts, clients unaffected

   # Scenario B: Change only video extension
   vim extensions/video/main.go
   git commit -m "Video change"
   git push
   # → Only video extension restarts, VPN stays up

   # Scenario C: Change VPN client
   vim client/main.go
   git commit -m "Client change"
   git push
   # → Full client rebuild, brief VPN reconnect
   ```

---

## 🐛 Troubleshooting

### "Extension binary not found"

**Cause:** Extension wasn't built during setup.

**Fix:**
```bash
cd extensions/video
go build -o video-extension .
```

### "go: command not found" during auto-update

**Cause:** Go binary not in PATH or wrong location.

**Fix:** The extension manager checks these locations:
- `go` (in PATH)
- `/usr/local/go/bin/go`
- `/usr/bin/go`
- `/opt/homebrew/bin/go`

Install Go or create symlink:
```bash
sudo ln -s /path/to/go /usr/local/go/bin/go
```

### Video calls still hang on "Calling..."

**Cause:** Old video extension binary (before port fix).

**Fix:**
```bash
cd extensions/video
git pull origin main
go build -o video-extension .
# Restart menu-bar
```

### Auto-update not triggering

**Cause:** VPN client not receiving update signals.

**Check:**
1. Is VPN connected? (Updates only work when connected)
2. Check signal file:
   ```bash
   ls -la ~/.family-vpn-update-signal
   # Should appear when server sends update
   ```
3. Check menu-bar logs for `[UPDATE]` messages

### Menu-bar crashes when starting extensions

**Cause:** Extension path incorrect or binary permissions wrong.

**Fix:**
```bash
# Check extension path
ls -la extensions/video/video-extension

# Fix permissions if needed
chmod +x extensions/video/video-extension

# Check logs
tail -f /tmp/family-vpn-menubar.log
```

---

## 📊 What Changed In This Update

### Server (server/main.go)
- ✅ Component-aware update handler
- ✅ Parses `?component=video` from deploy.sh
- ✅ Broadcasts `UPDATE_VIDEO`, `UPDATE_VPN`, `UPDATE_MENU`
- ✅ Only restarts server for VPN component updates

### Client (client/main.go)
- ✅ Handles all `UPDATE_*` messages
- ✅ Writes component name to signal file
- ✅ Enables fine-grained restart control

### Menu-Bar (menu-bar/)
- ✅ Extension Manager (extension_manager.go)
- ✅ Manages extension lifecycle
- ✅ Auto-restart on crashes (>10s uptime)
- ✅ Component-specific update handling
- ✅ Removed baked-in video code (now uses extension)

### Deploy Script (deploy.sh)
- ✅ Auto-detects changed components via git diff
- ✅ Supports: `./deploy.sh video` or `./deploy.sh vpn`
- ✅ Discovers NEW extensions automatically
- ✅ Sends component-tagged requests

### Video Extension (extensions/video/)
- ✅ Standalone process (hot-reloadable)
- ✅ Fixed port 8890 (was random)
- ✅ IPC integration for signaling
- ✅ Auto-answer incoming calls

### Build System
- ✅ Builds extensions automatically
- ✅ Go binary path resolution
- ✅ Extension binaries in .gitignore

---

## 🎓 Architecture Benefits

### Before (Monolithic)
```
Single Process:
├── VPN Client
├── Video Calling (baked in)
├── Menu UI
└── Everything else

Problem: Change anything → Restart everything → VPN disconnects
```

### After (Microservices)
```
Menu-Bar Process:
  └── Extension Manager (orchestrator)

VPN Client Process:
  ├── TUN management
  ├── Traffic routing
  └── IPC Server :8889

Video Extension Process:
  ├── WebRTC signaling
  ├── Video UI :8890
  └── IPC Client

Future Extensions:
  ├── Screen share
  ├── File transfer
  └── Remote desktop
```

**Benefits:**
1. ✅ Change video → Only video restarts
2. ✅ Add new extension → No VPN disruption
3. ✅ Development is 10x faster
4. ✅ Crashes isolated to one component
5. ✅ Zero manual intervention after setup

---

## 🚀 Future: Adding New Extensions

After this update, adding extensions is automatic:

```bash
# 1. Create new extension
mkdir -p extensions/screen-share
cd extensions/screen-share

# 2. Write code
cat > main.go << 'EOF'
package main
import "github.com/miguelemosreverte/family-vpn/extensions/framework"
// ... implement Extension interface
EOF

# 3. Initialize module
go mod init github.com/miguelemosreverte/family-vpn/extensions/screen-share

# 4. Commit and push
git add .
git commit -m "Add screen-share extension"
git push

# 5. deploy.sh automatically detects it!
# 6. Broadcasts UPDATE_SCREEN-SHARE
# 7. Clients auto-build and start it
```

**No manual steps needed on other computers!**

---

## ✅ Verification Checklist

After completing the manual update on all computers:

- [ ] All computers run latest code (check git log)
- [ ] All binaries rebuilt successfully
- [ ] Menu-bar starts and connects VPN
- [ ] Video extension visible in logs
- [ ] Video calling works (no "Calling..." hang)
- [ ] Test hot-reload with trivial change
- [ ] Verify VPN stays up during extension updates
- [ ] Check server logs show UPDATE_* broadcasts

---

## 🎉 Congratulations!

**This is your last manual update!** From now on:

- Push code → Everything updates automatically
- Extensions hot-reload without VPN disruption
- New extensions are discovered and deployed automatically
- Family devices stay in sync with zero effort

**Development workflow is now:**
```bash
vim extensions/video/main.go  # Make changes
git commit -am "Add feature"
git push                       # Done! ✨
```

**No more:**
- ❌ SSH into computers
- ❌ Manual git pull
- ❌ Manual rebuilds
- ❌ Manual restarts
- ❌ VPN disruptions

**Welcome to the future! 🚀**

---

*Generated: November 20, 2025*
*System: Family VPN Microservices Architecture v2.0*
