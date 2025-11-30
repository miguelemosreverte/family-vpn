# Skill: Deploy

Deploy changes to Family VPN clients across the network.

## When to Use

Use this skill when:
- User wants to push updates to all clients
- User asks to "deploy", "update all", "rollout", or "push changes"
- After making code changes that need to go live

## Environment

- **Server**: 95.217.238.72:443
- **Update API**: `POST https://95.217.238.72:443/update/init?component={component}`
- **Components**: `all`, `client`, `server`, `desktop`, `ui`

## Steps

### 1. Check Current Status

First, check if there are uncommitted changes:
```bash
cd /Users/miguel_lemos/Desktop/family-vpn
git status
```

### 2. Commit and Push (if needed)

If there are changes:
```bash
git add -A
git commit -m "Deploy: <description of changes>"
git push origin main
```

### 3. Trigger Update

Call the server API to broadcast update to all clients:
```bash
curl -s -k -X POST "https://95.217.238.72:443/update/init?component=all"
```

### 4. Verify Deployment

Check client versions via EventBus CLI:
```bash
./bin/eventbus-cli versions
```

Or check the dashboard Versions tab.

## Rollback

If something goes wrong:
```bash
git revert HEAD
git push origin main
curl -s -k -X POST "https://95.217.238.72:443/update/init?component=all"
```

## Expected Output

Successful deployment shows:
- All clients receive `updates.available` event
- Clients run auto-update.sh
- Clients re-report their version
- Dashboard shows all clients on same commit

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Client not updating | Check `/tmp/family-vpn-client.log` |
| Update stuck | SSH to client, run `./client/auto-update.sh` manually |
| Version mismatch | Check if client has git access |
