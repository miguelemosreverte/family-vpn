# VPN From Scratch

A simple VPN implementation in Go with toggleable encryption and routing.

## Features

- **Toggle VPN On/Off**: Connect and disconnect easily
- **Full Traffic Routing**: Route all your traffic through the VPN server
- **Toggleable Encryption**: Enable/disable AES-256-GCM encryption
- **Simple Deployment**: Easy CI/CD to your server

## Architecture

```
Client (MacOS/Linux)          Server (Ubuntu)
    |                              |
    |--- TUN Interface (10.8.0.2) |--- TUN Interface (10.8.0.1)
    |                              |
    |--- TCP Connection ---------->|
    |    (Port 8888)                |
    |                              |
    |<-- Encrypted/Plain Packets ->|
    |                              |
    |--- Routes all traffic ------>|--- Forwards to Internet
```

## Requirements

### Server
- Ubuntu server (tested on Ubuntu 22.04)
- Root access
- Go 1.21+ (auto-installed by deploy script)

### Client
- MacOS or Linux
- Go 1.21+
- sudo access (for TUN interface and routing)

## Quick Start

### 1. Deploy Server

```bash
# Make deploy script executable
chmod +x deploy-server.sh

# Deploy to server (installs Go, clones repo, builds, starts server)
./deploy-server.sh
```

The server will start with encryption **enabled** by default on port 8888.

### 2. Build and Run Client

```bash
# Build client
chmod +x build-client.sh
./build-client.sh

# Connect to VPN (requires sudo for TUN interface)
sudo ./client/vpn-client -server 95.217.238.72:8888
```

Press **Ctrl+C** to disconnect and restore normal routing.

## Usage

### Server Options

Run on server manually:
```bash
# With encryption (default)
./server/vpn-server -port 8888 -encrypt

# Without encryption
./server/vpn-server -port 8888
```

### Client Options

```bash
# Connect to VPN
sudo ./client/vpn-client -server <SERVER_IP>:8888
```

The client automatically:
- Detects server encryption setting
- Creates TUN interface (10.8.0.2/24)
- Routes all traffic through VPN
- Restores original routing on disconnect

## How It Works

### Server Side

1. Creates TUN device (`tun0`) with IP `10.8.0.1/24`
2. Enables IP forwarding
3. Listens for TCP connections on port 8888
4. Encrypts/decrypts packets (if encryption enabled)
5. Forwards packets between TUN interface and client

### Client Side

1. Connects to VPN server via TCP
2. Creates TUN device (`tun0`) with IP `10.8.0.2/24`
3. Saves current default gateway
4. Adds route to VPN server through original gateway
5. Routes all other traffic through VPN
6. Encrypts/decrypts packets (matching server setting)
7. On disconnect: restores original routing and cleans up TUN device

### Encryption

- **Algorithm**: AES-256-GCM
- **Key**: Shared 32-byte key (hardcoded for prototype)
- **Toggle**: Server controls encryption via `-encrypt` flag
- **Handshake**: Server sends encryption status on connect

## Testing

### Test Without Encryption

1. Deploy server without encryption:
```bash
# On server
./server/vpn-server -port 8888
```

2. Connect client:
```bash
sudo ./client/vpn-client -server 95.217.238.72:8888
```

3. Verify your IP changed:
```bash
curl ifconfig.me
```

### Test With Encryption

1. Deploy server with encryption:
```bash
# On server
./server/vpn-server -port 8888 -encrypt
```

2. Connect client (auto-detects encryption):
```bash
sudo ./client/vpn-client -server 95.217.238.72:8888
```

3. Verify connection in logs shows "Encryption: true"

## Deployment

The deployment script (`deploy-server.sh`):
- SSHs into your server
- Installs Go if needed
- Clones/updates the repository
- Builds the server
- Starts the VPN server with encryption enabled

To redeploy after changes:
```bash
git add .
git commit -m "Update VPN implementation"
git push origin main
./deploy-server.sh
```

## Troubleshooting

### Client can't create TUN device
```bash
# Ensure you're running with sudo
sudo ./client/vpn-client -server ...
```

### Connection refused
```bash
# Check server is running
ssh root@95.217.238.72 "ps aux | grep vpn-server"

# Check server logs
ssh root@95.217.238.72 "tail -f /var/log/vpn-server.log"
```

### Routing not working
```bash
# Check TUN interface
ip addr show tun0

# Check routes
ip route show
```

## Security Notes

This is a **prototype** for learning. For production:
- Use proper key exchange (e.g., Diffie-Hellman)
- Add authentication
- Use certificates/TLS
- Implement proper key rotation
- Add logging and monitoring
- Harden server (firewall, fail2ban, etc.)

## License

MIT