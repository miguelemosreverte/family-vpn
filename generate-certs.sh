#!/bin/bash
#
# Generate TLS certificates for VPN
# Creates self-signed certs that make VPN traffic look like HTTPS
#

set -e

CERT_DIR="certs"
DOMAIN="${1:-vpn.family.local}"

echo "========================================"
echo "Generating TLS Certificates"
echo "========================================"
echo "Domain: $DOMAIN"
echo ""

# Create certs directory
mkdir -p "$CERT_DIR"

# Generate private key
echo "📝 Generating private key..."
openssl genrsa -out "$CERT_DIR/server.key" 2048

# Generate certificate signing request
echo "📝 Generating certificate..."
openssl req -new -x509 -sha256 -key "$CERT_DIR/server.key" \
    -out "$CERT_DIR/server.crt" -days 3650 \
    -subj "/C=FI/ST=Uusimaa/L=Helsinki/O=Family VPN/CN=$DOMAIN"

echo ""
echo "✅ Certificates generated in $CERT_DIR/"
echo "   - server.key (private key)"
echo "   - server.crt (certificate)"
echo ""
echo "These make your VPN traffic look like HTTPS!"
echo "========================================"
