# Family VPN

Secure, encrypted VPN built from scratch with AES-256-GCM encryption.

## Features

- ✅ AES-256-GCM encryption
- ✅ Fast downloads (7-24 Mbps)
- ✅ Low latency (~100ms)
- ✅ DNS leak prevention (Cloudflare 1.1.1.1)
- ✅ TCP MSS clamping for reliable downloads
- ✅ Diagnostic instrumentation

## Setup

### 1. Configure Secrets

**Option A: Local Development (.env file)**

```bash
cp .env.example .env
# Edit .env with your values
```

**Option B: GitHub Repository Secrets (for CI/CD)**

Set these secrets in your GitHub repository:
- `VPN_SERVER_HOST` - Your VPN server IP
- `VPN_SSH_KEY` - Your SSH private key (base64 encoded)
- `SUDO_PASSWORD` - Your local sudo password
- `VPN_ENCRYPTION_KEY` - 32-byte hex key for AES-256

To set secrets via GitHub CLI:
```bash
gh secret set VPN_SERVER_HOST -b "95.217.238.72"
gh secret set VPN_SSH_KEY < ~/.ssh/id_ed25519_hetzner
gh secret set SUDO_PASSWORD -b "your_password"
```

### 2. Build

```bash
# Build client
cd client && go build -o vpn-client main.go

# Build server
cd server && go build -o vpn-server main.go
```

### 3. Deploy Server

```bash
./deploy-server.sh
```

### 4. Connect Client

```bash
# With timeout (60 seconds, for testing)
sudo ./client/vpn-client -server YOUR_SERVER:8888 -encrypt

# Without timeout (for real use)
sudo ./client/vpn-client -server YOUR_SERVER:8888 -encrypt --no-timeout
```

## Testing

```bash
# Comprehensive test suite
./test-doctor.sh

# HTTP download testing
./test-doctor-http.sh
```

## Security

- Never commit `.env` file
- Use strong encryption keys (32 bytes, generated with `openssl rand -hex 32`)
- Store secrets in GitHub repository secrets for CI/CD
- DNS queries route through VPN to prevent leaks

## Architecture

```
Client (10.8.0.2) <--[Encrypted Tunnel]--> Server (10.8.0.1) <--> Internet
```

- **Client**: macOS/Linux TUN interface, AES-256-GCM encryption
- **Server**: Linux TUN interface, NAT, iptables MSS clamping
- **Encryption**: AES-256-GCM with random nonces
- **DNS**: Cloudflare 1.1.1.1 + Google 8.8.8.8 through VPN
