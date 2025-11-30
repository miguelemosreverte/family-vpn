# /update-all

Trigger an update across all connected VPN clients.

## Usage

```
/update-all
```

## What This Command Does

1. Checks for uncommitted local changes
2. If changes exist, commits and pushes them
3. Calls the server update API to broadcast to all clients
4. Monitors client version reports to confirm update success

## Execution Steps

```bash
# 1. Check git status
cd /Users/miguel_lemos/Desktop/family-vpn
git status --short

# 2. If there are changes, commit and push
git add -A
git commit -m "Deploy: Auto-commit from /update-all"
git push origin main

# 3. Trigger update broadcast
curl -s -k -X POST "https://95.217.238.72:443/update/init?component=all"

# 4. Wait for clients to update (5-10 seconds)
sleep 5

# 5. Check versions
./bin/eventbus-cli versions
```

## Expected Output

```
Triggering update to all clients...
Server response: {"status":"ok","message":"Update broadcast sent"}

Waiting for clients to update...

Client Versions:
  MacBook-Air:    abc1234 ✓
  Mac-mini:       abc1234 ✓
  Anastasia-Mac:  abc1234 ✓

All clients updated successfully!
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Client not updating | SSH to client, check `/tmp/family-vpn-client.log` |
| Server not responding | Check if server is running at 95.217.238.72 |
| Git push failed | Check credentials, network connectivity |
