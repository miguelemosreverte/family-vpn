#!/bin/bash
cd "$(dirname "$0")/client"

echo "osopanda" | sudo -S ./family-vpn-client \
  --server 95.217.238.72:443 \
  --encrypt \
  --no-timeout \
  > /tmp/vpn-client.log 2>&1 &

echo "VPN client started"
