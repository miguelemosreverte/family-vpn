#!/bin/bash
#
# Fix Port 443 for VPN Server
# This script checks why port 443 isn't working and fixes it
#

set -e

echo "========================================"
echo "Port 443 Diagnostic and Fix"
echo "========================================"

# Check if anything is listening on port 443
echo "1. Checking if port 443 is already in use..."
if netstat -tlnp | grep ':443 ' > /dev/null 2>&1; then
    echo "⚠️  Port 443 is already in use:"
    netstat -tlnp | grep ':443 '
    echo ""
    read -p "Kill the process using port 443? (y/N) " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        PID=$(netstat -tlnp | grep ':443 ' | awk '{print $7}' | cut -d'/' -f1)
        kill $PID
        echo "✅ Killed process $PID"
    fi
else
    echo "✅ Port 443 is available"
fi

# Check firewall rules
echo ""
echo "2. Checking firewall (iptables)..."
if iptables -L -n | grep -q 'Chain INPUT'; then
    echo "Current INPUT rules:"
    iptables -L INPUT -n --line-numbers | head -20
    echo ""

    # Check if port 443 is allowed
    if iptables -L INPUT -n | grep -q 'dpt:443'; then
        echo "✅ Port 443 is allowed in firewall"
    else
        echo "⚠️  Port 443 not explicitly allowed"
        read -p "Add firewall rule to allow port 443? (y/N) " -n 1 -r
        echo ""
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            iptables -I INPUT -p tcp --dport 443 -j ACCEPT
            echo "✅ Added firewall rule for port 443"
        fi
    fi
else
    echo "✅ No firewall rules found (iptables not filtering)"
fi

# Option: Use iptables to forward 443 to 8888
echo ""
echo "3. Port forwarding option..."
echo "Instead of binding directly to 443, we can forward 443→8888"
echo "This way:"
echo "  - Clients connect to :443 (looks like HTTPS)"
echo "  - Server listens on :8888 (no privilege issues)"
echo "  - iptables forwards traffic automatically"
echo ""
read -p "Set up port forwarding 443→8888? (y/N) " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Enable IP forwarding
    echo 1 > /proc/sys/net/ipv4/ip_forward

    # Add PREROUTING rule to redirect 443→8888
    iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8888

    # Add INPUT rule to allow port 8888
    iptables -I INPUT -p tcp --dport 8888 -j ACCEPT

    # Make it persistent (on Debian/Ubuntu)
    if command -v netfilter-persistent > /dev/null; then
        netfilter-persistent save
        echo "✅ Made iptables rules persistent"
    fi

    echo "✅ Port forwarding configured: 443→8888"
    echo ""
    echo "Note: Server will still listen on port 8888"
    echo "      But external clients connect to port 443"
fi

echo ""
echo "========================================"
echo "Diagnostic complete!"
echo "========================================"
