# Family VPN - Quick Start Guide

## 🎉 What's New - TLS Stealth Mode + Remote Access

Your Family VPN now has:
- **TLS Encryption on Port 443** - Traffic looks like HTTPS, undetectable by Netflix/DPI
- **One-Click Remote Access** - Click any family member to open Screen Sharing
- **Peer Discovery** - See all connected family members in real-time
- **Auto-Updates** - Clients automatically update when you push code

---

## 🚀 Setup on a New Mac

### 1. Clone the Repository
```bash
git clone https://github.com/miguelemosreverte/family-vpn.git
cd family-vpn
```

### 2. Install Go (if needed)
```bash
# Check if you have Go:
go version

# If not installed:
brew install go
# OR download from: https://golang.org/dl/
```

### 3. Build the Menu Bar App
```bash
cd menu-bar
go build -o family-vpn-manager main.go
```

### 4. Run It!
```bash
# Normal mode (auto-connects on startup):
./family-vpn-manager

# Development mode (no auto-connect):
./family-vpn-manager -dev
```

---

## 🖥️ How to Use

### Menu Bar Features

When you run the app, you'll see a VPN icon in your menu bar. Click it to see:

1. **Connection Status** - Connected/Disconnected
2. **Your VPN IP** - e.g., 10.8.0.3
3. **Connection Duration** - How long you've been connected
4. **Data Transfer** - Upload/Download stats
5. **👨‍👩‍👧‍👦 Connected Family** - All family members on the VPN
6. **Buttons** - Connect, Disconnect, About, Quit

### One-Click Remote Access

1. Connect to the VPN (app auto-connects on startup)
2. Wait a few seconds for peer discovery
3. In the menu bar, you'll see: **👨‍👩‍👧‍👦 Connected Family**
4. Click any family member's name (e.g., "🖥️ MacBook-Pro (10.8.0.2)")
5. **Screen Sharing opens automatically!**
6. Enter the remote Mac's password
7. You're in! 🎉

---

## 🔒 Enable Screen Sharing (One-Time Setup)

For remote access to work, each Mac needs Screen Sharing enabled:

1. Open **System Preferences** → **Sharing**
2. Check ☑️ **Screen Sharing**
3. Set who can access:
   - **All users** - Anyone on the VPN can connect
   - **Only these users** - Specific family members only

**Important:** Make sure you know the Mac's login password - you'll need it to connect!

---

## 🌍 How It Works

### The Technology Stack

1. **VPN Server** (running on 95.217.238.72)
   - Listens on port 443 with TLS encryption
   - Looks like HTTPS traffic (undetectable!)
   - Assigns VPN IPs: 10.8.0.1 (server), 10.8.0.2, 10.8.0.3, etc.
   - Tracks all connected peers

2. **Menu Bar Client** (your Mac)
   - Connects via TLS to port 443
   - Receives VPN IP assignment
   - Routes all traffic through VPN
   - Gets peer list updates every 3 seconds
   - Shows family members in menu

3. **Remote Access** (macOS Screen Sharing)
   - Uses VPN IPs to connect directly
   - No need for port forwarding!
   - Works through the encrypted VPN tunnel

### Traffic Flow

```
Your Mac → TLS/443 → VPN Server → Internet
             ↓
        (looks like HTTPS)
             ↓
    Netflix/DPI sees: "Normal web browsing"
```

---

## 🧪 Testing Peer Discovery

### On Mac #1:
1. Run `./family-vpn-manager`
2. Wait for connection
3. Check menu bar - you should see your own device listed

### On Mac #2:
1. Run `./family-vpn-manager`
2. Wait for connection
3. Check menu bar - you should see **both devices** listed!

### Click to Connect:
- On Mac #1: Click Mac #2's name → Screen Sharing opens
- On Mac #2: Click Mac #1's name → Screen Sharing opens

**Success!** If you can see each other and click to connect, peer discovery is working! 🎉

---

## 🛠️ Development Mode

Use `-dev` flag when developing to prevent auto-connect:

```bash
./family-vpn-manager -dev
```

This prevents VPN from interfering with your internet while coding.

---

## 🔐 Security Features

- **TLS 1.3 Encryption** - All traffic encrypted twice (VPN + TLS wrapper)
- **Port 443** - Standard HTTPS port, impossible to block without breaking web browsing
- **Self-signed certificates** - No CA needed, your family's private network
- **AES-256-GCM** - Military-grade encryption for VPN tunnel
- **DNS Leak Protection** - Uses Cloudflare (1.1.1.1) and Google (8.8.8.8) DNS

---

## 🌟 Why This Works Against Detection

### Netflix/Streaming Services
- **What they see:** HTTPS connection to 95.217.238.72 on port 443
- **What they think:** "Normal web server communication"
- **Result:** ✅ No VPN detection

### Government DPI (Russia, China)
- **What they see:** TLS 1.3 encrypted traffic on port 443
- **What they can't see:** The VPN tunnel inside the TLS
- **Result:** ✅ Undetectable, can't be blocked without breaking all HTTPS

### ISP Throttling
- **What they see:** Regular HTTPS traffic
- **What they can't do:** Identify it as VPN traffic
- **Result:** ✅ No throttling

---

## 📊 Monitoring & Stats

The menu bar shows real-time stats:
- **Connection Duration** - How long connected
- **Data Transfer** - Upload/Download in MB/GB
- **Packet Rates** - Throughput in Mbps
- **Connected Peers** - Live family member list

Check the logs for detailed diagnostics:
```bash
tail -f /tmp/vpn-*.log
```

---

## 🐛 Troubleshooting

### VPN Won't Connect
1. Check server is running: `curl -I http://95.217.238.72:9000/health`
2. Check TLS: `openssl s_client -connect 95.217.238.72:443`
3. Check logs: `tail -f /tmp/vpn-*.log`

### Can't See Other Family Members
1. Wait 5-10 seconds after connecting (peer list updates every 3 seconds)
2. Check you're both connected to VPN (green icon in menu bar)
3. Check peer list file: `cat ~/.family-vpn-peers.json`

### Screen Sharing Doesn't Open
1. Make sure Screen Sharing is enabled on the target Mac
2. Check the VPN IP is correct (10.8.0.x)
3. Try manually: `open vnc://10.8.0.3` (replace with target IP)

### Internet Stops Working
1. Disconnect VPN (click "Disconnect" in menu)
2. Routing will be restored automatically
3. Reconnect to try again

---

## 🚀 Advanced Usage

### Check Your Public IP
```bash
# Before VPN:
curl https://api.ipify.org
# Your home IP

# After VPN:
curl https://api.ipify.org
# 95.217.238.72 (VPN server IP)
```

### Manual VPN IP Check
```bash
ifconfig | grep -A 3 "10.8.0"
```

### Server Logs (SSH required)
```bash
ssh root@95.217.238.72
tail -f /var/log/vpn-server.log
```

---

## 📝 Configuration

The app uses `.env` file in the repo root:
- `VPN_SERVER_HOST` - Server IP (default: 95.217.238.72)
- `VPN_SERVER_PORT` - Server port (default: 443)
- `SUDO_PASSWORD` - Your Mac's sudo password (for TUN setup)

---

## 🎯 Next Steps

1. **Clone on your other Mac** - Follow this guide
2. **Test peer discovery** - Both Macs should see each other
3. **Test remote access** - Click each other's names
4. **Enable auto-start** - Add to Login Items in System Preferences

Enjoy your secure, undetectable Family VPN! 🎉

---

**Built with:** Go, TLS 1.3, WireGuard-inspired design, macOS native APIs
**Status:** Production-ready with TLS stealth mode
**Server:** Running 24/7 on 95.217.238.72:443
