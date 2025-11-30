# Skill: SSH to Client

Connect to and manage remote VPN clients via SSH.

## When to Use

Use this skill when:
- User wants to SSH to a specific client
- User needs to debug a remote client
- User wants to run commands on remote machines
- User asks to "check on", "connect to", or "fix" a remote client

## Known Clients

| Hostname | Local IP | VPN IP | Username |
|----------|----------|--------|----------|
| Miguel's MacBook Air | 192.168.0.X | 10.8.0.2 | miguel_lemos |
| Miguel's Mac mini | miguel-lemoss-Mac-mini.local | 10.8.0.3 | miguel_lemos |
| Anastasia's MacBook | 192.168.0.14 | 10.8.0.10 | anastasiia |

## SSH Connection

### Basic Connection
```bash
ssh -o ConnectTimeout=10 -o StrictHostKeyChecking=no {username}@{hostname}
```

### With Command
```bash
ssh -o ConnectTimeout=10 -o StrictHostKeyChecking=no {username}@{hostname} "{command}"
```

## Common Remote Operations

### 1. Check VPN Status
```bash
ssh {user}@{host} "pgrep -lf 'family-vpn' && ifconfig tun0 2>/dev/null | head -5"
```

### 2. Check Logs
```bash
ssh {user}@{host} "tail -30 /tmp/family-vpn-client.log"
```

### 3. Restart VPN Client
```bash
ssh {user}@{host} "pkill -f 'vpn-client' && sleep 2 && /usr/local/bin/vpn-client --no-timeout &"
```

### 4. Run Auto-Update
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && ./client/auto-update.sh"
```

### 5. Check Git Version
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && git log -1 --format='%h %s'"
```

### 6. Pull Latest Changes
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && git pull origin main"
```

### 7. Reinstall
```bash
ssh {user}@{host} "cd ~/Desktop/family-vpn && ./uninstall.sh && ./install.sh"
```

## Troubleshooting SSH

| Issue | Solution |
|-------|----------|
| Connection refused | Client may be asleep, check if awake |
| Permission denied | Check username, may need password |
| Host key changed | Remove from known_hosts or use -o StrictHostKeyChecking=no |
| Timeout | Check network, client may be offline |

## Sudo Operations

Some operations require sudo. Get password from .env:
```bash
ssh {user}@{host} "
PASS=\$(grep SUDO_PASSWORD ~/Desktop/family-vpn/.env | cut -d= -f2)
echo \"\$PASS\" | sudo -S {command}
"
```

## Batch Operations

Run on all clients:
```bash
for host in "miguel-lemoss-Mac-mini.local" "192.168.0.14"; do
  echo "=== $host ==="
  ssh -o ConnectTimeout=5 miguel_lemos@$host "{command}" 2>/dev/null || echo "Failed"
done
```
