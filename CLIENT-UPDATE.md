# Client Update Guide

Quick guide to update the VPN client on any computer.

## Update Steps

1. **Pull latest code:**
   ```bash
   cd family-vpn
   git pull origin main
   ```

2. **Rebuild client:**
   ```bash
   cd client
   go build -o vpn-client main.go
   ```

3. **Rebuild menu-bar app:**
   ```bash
   cd ../menu-bar
   go build -o family-vpn-manager main.go
   ```

4. **Restart menu-bar app:**
   - Quit the current menu-bar app (click icon → Quit)
   - Start the new one: `./family-vpn-manager`

## What Changed (TLS Update)

- Server now runs on port **443** with **TLS encryption**
- VPN traffic looks like HTTPS web browsing (stealth mode)
- Client automatically uses TLS when connecting
- No configuration changes needed - just rebuild and restart

## Verify It's Working

After connecting, check the menu-bar app shows:
- Status: Connected
- Server: 95.217.238.72:443
- Your VPN IP address

The connection should work normally with reasonable speed.
