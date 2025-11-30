# /restart [component] [client]

Restart Family VPN components locally or remotely.

## Usage

```
/restart                 # Restart all local components
/restart client          # Restart VPN client only
/restart watchdog        # Restart watchdog only
/restart all mac-mini    # Restart all on Mac mini
```

## Components

| Component | Process Name | Restart Method |
|-----------|--------------|----------------|
| VPN Client | `vpn-client` | Kill + watchdog restarts |
| Watchdog | `family-vpn-watchdog` | Launchctl reload |
| Menu Bar | `family-vpn-menubar` | Kill + watchdog restarts |
| Desktop App | `Family VPN` | Kill + relaunch |

## Execution Steps

### Restart All (Local)

```bash
# Stop all
pkill -f 'vpn-client'
pkill -f 'family-vpn-menubar'
pkill -f 'Family VPN'

# Reload watchdog (it will restart the others)
launchctl unload ~/Library/LaunchAgents/com.family.vpn.watchdog.plist
launchctl load ~/Library/LaunchAgents/com.family.vpn.watchdog.plist
```

### Restart VPN Client Only

```bash
pkill -f 'vpn-client'
# Watchdog will restart it automatically
sleep 3
pgrep -lf 'vpn-client' && echo "VPN client restarted" || echo "Failed to restart"
```

### Restart Watchdog Only

```bash
launchctl unload ~/Library/LaunchAgents/com.family.vpn.watchdog.plist
sleep 1
launchctl load ~/Library/LaunchAgents/com.family.vpn.watchdog.plist
pgrep -lf 'family-vpn-watchdog' && echo "Watchdog restarted" || echo "Failed to restart"
```

### Remote Restart

```bash
# Restart all on Mac mini
ssh miguel_lemos@miguel-lemoss-Mac-mini.local "
  pkill -f 'vpn-client'
  pkill -f 'family-vpn-menubar'
  pkill -f 'Family VPN'
  launchctl unload ~/Library/LaunchAgents/com.family.vpn.watchdog.plist
  launchctl load ~/Library/LaunchAgents/com.family.vpn.watchdog.plist
  sleep 3
  pgrep -lf 'family-vpn'
"
```

## Full Reset

For a complete reset with reinstallation:

```bash
cd ~/Desktop/family-vpn
./uninstall.sh
./install.sh
```
