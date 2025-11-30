# /ssh [client]

SSH to a specific VPN client for debugging or management.

## Usage

```
/ssh mac-mini
/ssh anastasia
/ssh [hostname or IP]
```

## Known Clients

| Alias | Hostname | Username |
|-------|----------|----------|
| `mac-mini` | miguel-lemoss-Mac-mini.local | miguel_lemos |
| `anastasia` | 192.168.0.14 | anastasiia |
| `macbook` | (this machine) | miguel_lemos |

## What This Command Does

1. Resolves the client alias to hostname/IP
2. Establishes SSH connection
3. Optionally runs a command

## Common Operations After Connection

```bash
# Check VPN status
pgrep -lf 'family-vpn' && ifconfig tun0 2>/dev/null | head -5

# View logs
tail -30 /tmp/family-vpn-client.log

# Restart VPN client
pkill -f 'vpn-client' && sleep 2 && /usr/local/bin/vpn-client --no-timeout &

# Run update
cd ~/Desktop/family-vpn && ./client/auto-update.sh

# Full reinstall
cd ~/Desktop/family-vpn && ./uninstall.sh && ./install.sh
```

## SSH Options Used

```bash
ssh -o ConnectTimeout=10 -o StrictHostKeyChecking=no {username}@{hostname}
```

- `ConnectTimeout=10`: Fail fast if client is unreachable
- `StrictHostKeyChecking=no`: Don't prompt for host key verification

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Connection refused | Client may be asleep or offline |
| Permission denied | Check username, may need password |
| Timeout | Check network, try VPN IP instead |
