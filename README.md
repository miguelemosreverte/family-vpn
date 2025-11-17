# Family VPN

Secure, encrypted VPN built from scratch with AES-256-GCM encryption.

## Features

- ✅ **AES-256-GCM encryption** - Military-grade encryption for all traffic
- ✅ **Fast downloads** - 7-24 Mbps through encrypted tunnel
- ✅ **Low latency** - ~100ms overhead, great for browsing and YouTube
- ✅ **DNS leak prevention** - All DNS queries through Cloudflare 1.1.1.1
- ✅ **TCP MSS clamping** - Reliable downloads without fragmentation
- ✅ **Diagnostic tools** - Built-in performance monitoring

---

## 🚀 Quick Start (New Computer)

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

### 3. Retrieve Secrets from GitHub

The repository secrets are stored securely in GitHub. Retrieve them to your local `.env` file:

```bash
# Create .env file from GitHub secrets
gh secret list  # Verify secrets exist

# Method 1: Manually create .env (copy from .env.example and fill in)
cp .env.example .env
nano .env  # Edit with your values

# Method 2: Retrieve from GitHub Actions (if configured)
# Secrets are: VPN_SERVER_HOST, VPN_SSH_KEY, SUDO_PASSWORD
```

**To view/manage secrets:**
```bash
# List all secrets
gh secret list

# View when a secret was last updated
gh secret list

# Set/update a secret
gh secret set VPN_SERVER_HOST -b "YOUR_SERVER_IP"
gh secret set VPN_SSH_KEY < ~/.ssh/your_ssh_key
gh secret set SUDO_PASSWORD -b "your_password"
```

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

1. **GitHub Repository Secrets** - Encrypted, accessible via `gh secret list`
2. **Local `.env` file** - Gitignored, only on your computer
3. **Never in git commits** - .gitignore prevents accidental commits

### Retrieving Secrets on New Computer

```bash
# Option 1: View in GitHub web UI
# Go to: https://github.com/miguelemosreverte/family-vpn/settings/secrets/actions

# Option 2: Use GitHub CLI
gh secret list  # Lists all secrets (but not their values)

# Option 3: Manually recreate .env
cp .env.example .env
# Fill in values (you need to know them or retrieve from secure storage)
```

**Note:** GitHub secrets can only be *set*, not *retrieved* via CLI. You'll need to either:
- Remember the values
- Store them in a password manager (1Password, LastPass, etc.)
- Document them in a secure location

### Adding a New Secret

```bash
# Via GitHub CLI
gh secret set SECRET_NAME -b "secret_value"

# Via file (for SSH keys)
gh secret set VPN_SSH_KEY < ~/.ssh/id_ed25519_hetzner

# Via GitHub web UI
# Go to: Settings → Secrets and variables → Actions → New repository secret
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

## 🙋 Support

If you encounter issues:

1. Check **Troubleshooting** section above
2. Run `./test-doctor.sh` to diagnose
3. Check server logs: `ssh root@95.217.238.72 "tail -100 /var/log/vpn-server.log"`
4. Review recent commits: `git log --oneline -10`
