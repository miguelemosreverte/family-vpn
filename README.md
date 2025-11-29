# Family VPN

Secure, encrypted VPN built from scratch with AES-256-GCM encryption.

## ⚠️ DEPLOYING CHANGES? → [READ THIS FIRST: DEPLOY-README.md](DEPLOY-README.md)

**ALWAYS use `./deploy.sh` after code changes - NEVER manually restart VPN components!**

---

## Features

- 🖥️ **macOS Menu Bar App** - One-click VPN toggle with auto-connect on login
- ✅ **AES-256-GCM encryption** - Military-grade encryption for all traffic
- ✅ **Fast downloads** - 7-24 Mbps through encrypted tunnel
- ✅ **Low latency** - ~100ms overhead, great for browsing and YouTube
- ✅ **DNS leak prevention** - All DNS queries through Cloudflare 1.1.1.1
- ✅ **TCP MSS clamping** - Reliable downloads without fragmentation
- ✅ **Diagnostic tools** - Built-in performance monitoring

---

## 🚀 Quick Start (New Computer)

### Automated Installation (Recommended)

The fastest way to get started is with the automated installation script:

```bash
# 1. Clone the repository
git clone git@github.com:miguelemosreverte/family-vpn.git
cd family-vpn

# 2. Install dependencies (macOS)
brew install go node imagemagick gh

# 3. Retrieve secrets from GitHub Gist
gh auth login  # Authenticate with GitHub
gh gist clone b523442d7bec467dbba22a21feab027e
cp b523442d7bec467dbba22a21feab027e/.env .
rm -rf b523442d7bec467dbba22a21feab027e

# 4. Run automated installation (builds and installs everything)
./install.sh
```

This single script will:
- ✅ Build the VPN client (Go)
- ✅ Build the menu bar app (Go)
- ✅ Build the desktop app (Electron)
- ✅ Generate app icon
- ✅ Install both apps to `/Applications/`
- ✅ Set up crash recovery scripts

**After installation:**
- Open "Family VPN" from Applications folder (appears in dock)
- Menu bar icon appears automatically at top-right
- Click menu bar icon to connect/disconnect VPN

---

### Manual Installation (Advanced)

If you prefer to build components separately or the automated script doesn't work:

<details>
<summary>Click to expand manual installation steps</summary>

### 1. Clone the Repository

```bash
git clone git@github.com:miguelemosreverte/family-vpn.git
cd family-vpn
```

### 2. Install Prerequisites

```bash
# macOS
brew install go gh

# Authenticate with GitHub (needed to retrieve secrets)
gh auth login
# Follow prompts: choose HTTPS, login with browser, paste the code shown
```

### 3. Retrieve Secrets from GitHub Gist

The `.env` file with all secrets is stored in a **private GitHub Gist**. This is much easier than GitHub repository secrets (which only work for Actions).

```bash
# Clone the secret gist containing .env
gh gist clone b523442d7bec467dbba22a21feab027e

# Copy .env to your VPN directory
cp b523442d7bec467dbba22a21feab027e/.env .

# Verify it worked
cat .env  # Should show VPN_SERVER_HOST, VPN_SSH_KEY, etc.

# Delete the gist clone (keep only .env)
rm -rf b523442d7bec467dbba22a21feab027e
```

**Alternative: Manually create .env**
```bash
cp .env.example .env
nano .env  # Fill in: VPN_SERVER_HOST, VPN_SSH_KEY path, SUDO_PASSWORD
```

**Gist URL (for reference):**
- https://gist.github.com/miguelemosreverte/b523442d7bec467dbba22a21feab027e
- This is a **secret gist** - only visible when logged into GitHub

### 4. Build the VPN Client

```bash
cd client
go build -o vpn-client main.go
cd ..
```

### 5. Connect to VPN

```bash
# Source the .env file (if using environment variables)
export VPN_SERVER_HOST=95.217.238.72  # Or load from .env

# Connect (with 60s timeout for testing)
sudo ./client/vpn-client -server ${VPN_SERVER_HOST}:8888 -encrypt

# Connect (no timeout, for real use)
sudo ./client/vpn-client -server ${VPN_SERVER_HOST}:8888 -encrypt --no-timeout
```

You should see:
```
✓ Connected to VPN server
✓ DNS configured: 1.1.1.1 (Cloudflare), 8.8.8.8 (Google) through VPN
✓ All traffic now routed through VPN
```

</details>

---

## 🖥️ Menu Bar Application (macOS - Recommended)

The easiest way to use Family VPN on macOS is with the **menu bar application** - a beautiful, native macOS app with one-click VPN toggle.

### Features

- 🔒/🔓 **One-click toggle** - Click menu bar icon to connect/disconnect instantly
- 🚀 **Auto-start on login** - App launches automatically when you log in
- ⚡ **Auto-connect** - VPN connects automatically 2 seconds after app launch
- 📊 **Live monitoring** - Real-time connection status, IP address, duration
- 🛡️ **Safe fallback** - If VPN fails to connect, routing is restored automatically
- 💕 **Made with love** - Sweet personal message in "About"

### Setup

1. **Build everything** (VPN client + menu bar app):
```bash
./build-menubar.sh
```

2. **Run the menu bar app**:
```bash
cd menu-bar
./family-vpn-menubar
```

You'll see a 🔓 icon in your menu bar. Click it to:
- Connect/disconnect VPN with one click
- See your current IP address and server location
- Monitor connection duration
- View connection stats

3. **Install auto-launch** (start on login - RECOMMENDED):
```bash
cd menu-bar
./install-autolaunch.sh
```

This makes the menu bar app:
- ✅ Launch automatically when you log in
- ✅ Auto-connect to VPN 2 seconds after launch
- ✅ Always available in your menu bar

### Usage

**First time:**
- When you click "Connect to VPN", macOS will prompt for your password
- This is needed to create network interfaces and modify routing
- Enter your password once, and the VPN will connect

**After auto-launch is installed:**
- App starts automatically when you log in
- VPN connects automatically after 2 seconds
- If connection fails, routing is safely restored (internet still works)
- Click the 🔒 icon to disconnect or see connection details

**To uninstall auto-launch:**
```bash
cd menu-bar
./uninstall-autolaunch.sh
```

---

## 📋 Common Commands

### Deploy Server (Updates VPN server)

```bash
# Builds, deploys, and restarts VPN server
./deploy-server.sh

# What it does:
# 1. Commits current code to git
# 2. Pushes to GitHub
# 3. SSHs to server, pulls latest code
# 4. Builds server binary
# 5. Restarts VPN server
```

### Run VPN Doctor (Test Performance)

```bash
# Comprehensive test suite
./test-doctor.sh

# Tests:
# - Connectivity (with/without VPN)
# - Latency measurements
# - Throughput (upload speed)
# - DNS leak protection
# - Encryption verification
```

### Test HTTP Downloads

```bash
# Test real-world download performance
./test-doctor-http.sh

# Downloads test files:
# - 100KB (small)
# - 1MB (medium)  
# - 10MB (large)
# Compares baseline vs VPN performance
```

### Start VPN Client

```bash
# Quick test (60 second timeout)
sudo ./client/vpn-client -server 95.217.238.72:8888 -encrypt

# Production use (no timeout)
sudo ./client/vpn-client -server 95.217.238.72:8888 -encrypt --no-timeout

# Convenient wrapper script
./browse-with-vpn.sh
```

### Stop VPN Client

```bash
# Press Ctrl+C in the terminal running vpn-client
# OR
sudo pkill vpn-client
```

---

## 🔧 Development Workflow

### Making Changes to Server

```bash
# 1. Edit server code
nano server/main.go

# 2. Test locally (if you have a test server)
cd server && go build -o vpn-server main.go

# 3. Commit changes
git add server/main.go
git commit -m "Description of changes"

# 4. Deploy to production server
./deploy-server.sh

# 5. Test with VPN Doctor
./test-doctor.sh
```

### Making Changes to Client

```bash
# 1. Edit client code
nano client/main.go

# 2. Rebuild
cd client && go build -o vpn-client main.go

# 3. Test connection
sudo ./vpn-client -server 95.217.238.72:8888 -encrypt

# 4. Commit when working
git add client/main.go
git commit -m "Description of changes"
git push
```

---

## 🔐 Security & Secrets Management

### What Secrets Are Stored

- **VPN_SERVER_HOST** - IP address of your VPN server (e.g., 95.217.238.72)
- **VPN_SSH_KEY** - SSH private key for deploying to server
- **SUDO_PASSWORD** - Your local sudo password (for running VPN client)
- **VPN_ENCRYPTION_KEY** - 32-byte AES-256 key (currently hardcoded in code)

### Where Secrets Are Stored

1. **Private GitHub Gist** - `.env` file stored securely, retrievable on any computer
2. **Local `.env` file** - Gitignored, only on your computer
3. **Never in git commits** - .gitignore prevents accidental commits

### Retrieving Secrets on New Computer

**Best Method: Clone the private gist**

```bash
# Clone the gist containing .env
gh gist clone b523442d7bec467dbba22a21feab027e

# Copy to VPN directory
cp b523442d7bec467dbba22a21feab027e/.env .

# Clean up
rm -rf b523442d7bec467dbba22a21feab027e
```

**Gist URL:** https://gist.github.com/miguelemosreverte/b523442d7bec467dbba22a21feab027e

**Alternative:** Manually recreate .env from template
```bash
cp .env.example .env
nano .env  # Fill in values from memory or password manager
```

### Updating the Gist (when secrets change)

```bash
# Edit your local .env file
nano .env

# Update the gist
gh gist edit b523442d7bec467dbba22a21feab027e .env

# Or delete and recreate
gh gist delete b523442d7bec467dbba22a21feab027e
gh gist create .env -d "Family VPN secrets - private .env file"
```

---

## 📊 Performance Metrics

### Expected Performance

| Metric | Without VPN | With VPN (Encrypted) |
|--------|-------------|---------------------|
| **Latency** | 92ms | 98ms (+6ms) |
| **Upload** | 19 Mbps | 6-7 Mbps (32%) |
| **Download** | 6-30 Mbps | 7-24 Mbps (70-80%) |
| **YouTube** | ✅ Instant | ✅ Fast (~2s start) |

### Monitoring While Connected

The VPN client shows real-time stats every 5 seconds:

```
[EGRESS] 102 pkt/s, 0.50 Mbps, 1.3 pkt/flush
[TIMING] TUN:9782µs Encrypt:8µs Mutex:0µs NetWrite:0µs Flush:0µs
[INGRESS] 143 pkt/s, 1.01 Mbps
[TIMING] NetRead:6896µs Decrypt:7µs TUNWrite:5µs
```

- **EGRESS** - Upload (client → server)
- **INGRESS** - Download (server → client)
- **TIMING** - Performance breakdown in microseconds

---

## 🐛 Troubleshooting

### VPN Won't Connect

```bash
# Check server is running
ssh root@95.217.238.72 "pgrep vpn-server"

# View server logs
ssh root@95.217.238.72 "tail -50 /var/log/vpn-server.log"

# Restart server
./deploy-server.sh
```

### DNS Not Working

```bash
# Check DNS configuration
scutil --dns | grep nameserver
# Should show: nameserver[0] : 1.1.1.1

# Test DNS resolution
dig youtube.com
```

### Slow Downloads

```bash
# Run performance test
./test-doctor-http.sh

# Check for packet drops
ssh root@95.217.238.72 "ip -s link show tun0"
# Look for "TX dropped" - should be 0 or very low
```

### "Permission Denied" Errors

```bash
# VPN client requires sudo (creates TUN interface, modifies routes)
sudo ./client/vpn-client -server 95.217.238.72:8888 -encrypt

# Deploy script requires SSH access
chmod 600 ~/.ssh/id_ed25519_hetzner
ssh-add ~/.ssh/id_ed25519_hetzner
```

### 🚨 Emergency: VPN Client Crashed / Internet Broken

**Symptoms:** VPN client was killed/crashed, and now internet doesn't work at all.

**What happened:** The VPN changed your routing and DNS, but didn't clean up when it died. Your computer is trying to route traffic through a dead VPN tunnel.

**Quick Fix:**

```bash
# Run the emergency cleanup script
./cleanup-vpn.sh
```

**Manual Recovery (if script doesn't work):**

```bash
# 1. Kill zombie VPN process
sudo pkill -9 vpn-client

# 2. Check routing
netstat -rn | grep default
# If shows "10.8.0.1" → routing is broken!

# 3. Delete broken VPN route
sudo route -n delete default

# 4. Restore original gateway (replace with YOUR router IP)
sudo route -n add -net default 192.168.100.1

# 5. Reset DNS to automatic
sudo networksetup -setdnsservers Wi-Fi Empty

# 6. Test internet
ping 8.8.8.8
curl ifconfig.me  # Should show your real IP
```

**If still broken:**

```bash
# Restart Wi-Fi
sudo networksetup -setairportpower en0 off
sleep 2
sudo networksetup -setairportpower en0 on

# Or worst case: restart computer
```

---

## 🏗️ Architecture

```
┌─────────────────┐                  ┌──────────────────┐
│  Your Computer  │                  │   VPN Server     │
│   (10.8.0.2)    │                  │   (10.8.0.1)     │
│                 │                  │   Helsinki       │
│  ┌───────────┐  │   Encrypted      │  ┌────────────┐  │
│  │ VPN Client├──┼──────Tunnel──────┼──┤ VPN Server │  │
│  └─────┬─────┘  │  AES-256-GCM     │  └──────┬─────┘  │
│        │        │                  │         │        │
│   ┌────▼─────┐  │                  │   ┌─────▼─────┐  │
│   │   TUN    │  │                  │   │    TUN    │  │
│   │Interface │  │                  │   │ Interface │  │
│   └──────────┘  │                  │   └─────┬─────┘  │
└─────────────────┘                  │         │        │
                                     │   ┌─────▼─────┐  │
                                     │   │ iptables  │  │
                                     │   │ NAT/MSS   │  │
                                     │   └─────┬─────┘  │
                                     │         │        │
                                     └─────────┼────────┘
                                               │
                                          Internet
```

### Key Components

- **TUN Interface** - Virtual network interface for routing traffic
- **AES-256-GCM** - Encryption with authentication
- **TCP MSS Clamping** - Prevents fragmentation (MSS=1360)
- **NAT/Masquerading** - Translates VPN IPs to server's public IP
- **DNS Override** - Forces all DNS through Cloudflare 1.1.1.1

---

## 📚 Additional Resources

### Helper Scripts

- `browse-with-vpn.sh` - Start VPN client conveniently
- `start-encrypted-server.sh` - Start server with encryption
- `deploy-server.sh` - Deploy latest code to server
- `test-doctor.sh` - Comprehensive VPN testing
- `test-doctor-http.sh` - HTTP download performance test
- `cleanup-vpn.sh` - 🚨 Emergency cleanup if VPN crashes

### Important Files

- `client/main.go` - VPN client source code
- `server/main.go` - VPN server source code
- `.env` - Local secrets (gitignored)
- `.env.example` - Template for secrets

### Logs

- Client: stdout (terminal where you run vpn-client)
- Server: `/var/log/vpn-server.log` on VPN server

---

## 🎯 Common Use Cases

### Daily Browsing

```bash
# Start VPN
sudo ./client/vpn-client -server 95.217.238.72:8888 -encrypt --no-timeout

# Browse normally
# DNS queries → Cloudflare 1.1.1.1 (through VPN)
# All traffic encrypted with AES-256-GCM
# Your IP appears as Helsinki server IP

# Stop VPN
Ctrl+C
```

### Testing After Changes

```bash
# 1. Deploy changes
./deploy-server.sh

# 2. Run full test suite
./test-doctor.sh

# 3. Test HTTP downloads
./test-doctor-http.sh

# 4. Manual browsing test
sudo ./client/vpn-client -server 95.217.238.72:8888 -encrypt
# Open YouTube, test speed test sites
```

### Checking VPN is Working

```bash
# While connected:
# 1. Check IP address
curl ifconfig.me
# Should show: 95.217.238.72 (Helsinki)

# 2. Check DNS
scutil --dns | grep nameserver
# Should show: 1.1.1.1

# 3. Check routing
netstat -rn | grep default
# Should show: default -> 10.8.0.1 (VPN)
```

---

## 📝 Notes

- VPN uses port **8888** for server connection
- Client automatically configures DNS to prevent leaks
- All routes restored when VPN disconnects (Ctrl+C)
- Server runs on Ubuntu 22.04 LTS
- Client tested on macOS (should work on Linux too)

---

## 🖥️ Desktop Dashboard & Auto-Update System

### Desktop App Features

The Family VPN now includes an **Electron desktop dashboard** with:

- 📊 **Real-time Monitoring** - View all connected VPN clients
- 💾 **Disk Usage** - Monitor disk space across all peers
- 🖥️ **Multi-Volume Support** - Track multiple physical drives per client
- 🔗 **SSH Integration** - Direct SSH access to VPN peers
- 🎨 **Dark/Light Mode** - WSJ-inspired newspaper theme
- 🔄 **Auto-Update** - Hot-reload UI from Git **without restart**
- ⚙️ **Feature Flags** - Enable/disable features remotely

### Installation & Setup

```bash
# One-command installation
./install.sh
```

This script:
1. Checks dependencies (Go, Node.js, ImageMagick)
2. Builds menu bar app (Go)
3. Builds desktop app (Electron)
4. Generates app icon
5. Installs both to Applications folder
6. Sets up crash recovery scripts

### Running the Apps

```bash
# Open desktop app (auto-launches menu bar too)
open "/Applications/Family VPN.app"

# Or run menu bar app only
/usr/local/bin/family-vpn-menubar
```

**Integration:** Menu bar and desktop apps are **linked** - closing one closes the other.

### Feature Flags System

Feature flags are controlled via `desktop-app/feature-flags.json`:

```json
{
  "version": "1.0.0",
  "features": {
    "darkMode": {
      "enabled": true,
      "description": "Enable dark/light mode toggle"
    },
    "autoUpdate": {
      "enabled": true,
      "description": "Auto-update UI from Git without restart",
      "checkIntervalMinutes": 5
    },
    "ssh_integration": {
      "enabled": true,
      "description": "SSH access to VPN peers"
    }
  },
  "ui": {
    "refreshInterval": 3000,
    "animationsEnabled": true
  }
}
```

**How it works:**

1. Edit `feature-flags.json` on **any machine**
2. Commit and push to Git
3. **All other machines auto-pull** within 5 minutes
4. **UI updates automatically** without restart

**Example:** Disable dark mode across all clients:

```bash
# Edit feature-flags.json
{
  "features": {
    "darkMode": { "enabled": false }
  }
}

# Commit and push
git add desktop-app/feature-flags.json
git commit -m "Disable dark mode"
git push origin main

# All clients will pull and apply within 5 minutes!
```

### Auto-Update System

The desktop app has **two update modes**:

#### 1. Hot-Reload (No Restart Needed)

For UI changes (HTML, CSS, JS, feature flags):

- Checks Git every **5 minutes** (configurable)
- Detects changes to `desktop-app/renderer/` or `feature-flags.json`
- **Automatically pulls** from Git
- **Reloads CSS** without full page reload
- Shows success notification

**Just push to Git, and all clients update automatically!**

```bash
# Edit UI files
nano desktop-app/renderer/styles.css

# Commit and push
git add desktop-app/renderer/styles.css
git commit -m "Update UI colors"
git push origin main

# Within 5 minutes:
# ✅ All clients pull changes
# ✅ CSS reloads automatically
# ✅ No restart needed!
```

#### 2. Full Reinstall (Cmd+Shift+U)

For code changes (Go, Electron main process, dependencies):

Press **`Cmd+Shift+U`** in the desktop app to:

1. Pull latest from Git
2. Rebuild menu bar app (Go)
3. Rebuild desktop app (Electron)
4. Reinstall both apps
5. Auto-restart

**This is a hidden feature** - useful for updating existing installations remotely.

### Crash Recovery

**CRITICAL FIX:** If VPN client crashes or is force-killed, the system no longer loses internet access.

**How it works:**

1. Menu bar app detects VPN client death
2. Automatically runs `fix-routing.sh`
3. Restores internet via multiple fallback methods:
   - Check existing routes for gateway
   - Query DHCP for router IP
   - Ping common router IPs (192.168.0.1, etc.)
   - Restart Wi-Fi as last resort

**No manual intervention needed!**

### Updating Existing Installations

#### Method 1: Auto-Update (Recommended)

**For UI changes** (HTML, CSS, JS, feature flags):

```bash
git add desktop-app/renderer/
git commit -m "Update feature"
git push origin main
```

✅ All clients auto-pull within 5 minutes
✅ No restart needed

**For code changes** (Go, Electron main):

Users press `Cmd+Shift+U` to trigger reinstall.

#### Method 2: Manual Reinstall

On each machine:

```bash
cd family-vpn
git pull origin main
./install.sh
```

### Development

Run in dev mode with DevTools:

```bash
# Desktop app (with DevTools)
cd desktop-app
npm start -- --dev

# Menu bar (10 second timeout)
cd menu-bar
go run main.go
```

### Architecture

```
family-vpn/
├── client/              # VPN client (Go)
│   ├── main.go
│   ├── fix-routing.sh   # Crash recovery
│   └── vpn-watchdog.sh
├── server/              # VPN server (Go)
├── menu-bar/            # Menu bar app (Go + systray)
│   └── main.go
├── desktop-app/         # Electron dashboard
│   ├── main.js          # Main process
│   ├── preload.js       # IPC bridge
│   ├── renderer/        # UI (hot-reloadable)
│   │   ├── index.html
│   │   ├── app.js
│   │   └── styles.css
│   ├── feature-flags.json
│   └── assets/
│       └── icon.icns
├── extensions/          # Plugin system
│   ├── video/
│   └── ssh/
└── install.sh           # One-command install
```

### Logs

- **Menu bar**: `/tmp/menubar.log`, `/tmp/menubar-debug.log`
- **Desktop app**: Open DevTools (`Cmd+Opt+I`)
- **VPN client**: Logged via menu bar

---

## 🙋 Support

If you encounter issues:

1. Check **Troubleshooting** section above
2. Run `./test-doctor.sh` to diagnose
3. Check server logs: `ssh root@95.217.238.72 "tail -100 /var/log/vpn-server.log"`
4. Review recent commits: `git log --oneline -10`
5. For desktop app issues: Open DevTools (`Cmd+Opt+I`) and check console
