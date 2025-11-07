# Development Guide

## Regression Protection

This project has **automatic regression protection** via git pre-commit hooks.

### How It Works

Before any commit is allowed:
1. **Client builds successfully** - Code must compile
2. **Plain VPN mode passes** - Core functionality must work
3. **Test suite runs in ~5 seconds** - Fast feedback

### Pre-Commit Hook

Located at: `.git/hooks/pre-commit`

**What it checks:**
- ✅ Client compiles without errors
- ✅ Plain VPN connection works (no encryption)
- ✅ IP changes to server IP
- ⚠️  Encrypted mode can fail (work in progress)

**Commit is BLOCKED if:**
- Build fails
- Plain VPN mode breaks
- IP doesn't change through VPN

**Commit is ALLOWED if:**
- Plain VPN works (even if encryption fails)

### Why This Matters

**Before:** Could accidentally break working features
**After:** Git automatically prevents regressions

### Running Tests Manually

```bash
# Run full test suite
./test-suite.sh

# Quick check plain mode only
echo "osopanda" | sudo -S ./client/vpn-client -server 95.217.238.72:8888 &
sleep 5
curl ifconfig.me  # Should show 95.217.238.72
sudo pkill -f vpn-client
```

### Development Workflow

1. Make changes to code
2. Run `./test-suite.sh` to verify
3. Commit - pre-commit hook runs automatically
4. If tests pass → commit succeeds
5. If tests fail → commit blocked, fix issues first

### Bypassing Hook (NOT RECOMMENDED)

Only in emergencies:
```bash
git commit --no-verify -m "Emergency fix"
```

**Never bypass the hook for regular development!**

### Current Status

- ✅ **Plain VPN**: Fully working
  - Connection: Works
  - Traffic routing: Works
  - IP masking: Works

- ⚠️  **Encrypted VPN**: In progress
  - Connection: Establishes
  - Encryption negotiation: Works
  - Packet handling: Needs debugging (auth failures)

## Making Changes Safely

### Rule #1: Never Break Plain Mode

Plain VPN is the core functionality. Any changes must keep it working.

### Rule #2: Test Before Committing

```bash
./test-suite.sh
```

If it fails, don't commit.

### Rule #3: Fast Feedback Loop

The test suite runs in ~5 seconds. Use it frequently:
- After every change
- Before every commit
- When debugging

### Working on Encryption

Encryption is a **separate feature** from plain VPN. You can:
- Break encryption temporarily while debugging
- Commit code with broken encryption
- **NEVER** break plain VPN in the process

### Test-Driven Development

For encryption fixes:

1. Write a test that shows the bug
2. Run test - it fails
3. Fix the code
4. Run test - it passes
5. Commit (plain mode still works)

### Example Session

```bash
# Make changes to encryption code
vim client/main.go

# Test it
./test-suite.sh
# Output: Plain PASS, Encrypted FAIL

# More changes
vim client/main.go

# Test again
./test-suite.sh
# Output: Plain PASS, Encrypted PASS

# Commit
git add client/main.go
git commit -m "Fix encryption packet handling"
# Pre-commit hook runs, both pass, commit succeeds
```

## Debugging Encryption

Current issue: "cipher: message authentication failed"

**Root cause:** macOS TUN vs Linux TUN packet format differences

**Approach:**
1. Keep plain mode working
2. Add encryption debug logging
3. Compare packet formats
4. Fix platform differences
5. Test with suite

**Never:**
- Break plain mode while debugging
- Commit without testing
- Bypass pre-commit hook
