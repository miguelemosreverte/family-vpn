#!/bin/bash
#
# Component-Aware Deployment Script
# Usage:
#   ./deploy.sh           - Deploy all components
#   ./deploy.sh video     - Deploy only video extension
#   ./deploy.sh vpn       - Deploy only VPN core
#   ./deploy.sh menu      - Deploy only menu-bar
#

set -e

VPN_SERVER="95.217.238.72"
UPDATE_PORT="9000"
COMPONENT="${1:-all}"

echo "========================================"
echo "Family VPN - Component Deployment"
echo "========================================"
echo ""
echo "Component: $COMPONENT"
echo "Server: $VPN_SERVER"
echo ""

# Auto-detect what changed in latest commit
if [ "$COMPONENT" = "all" ]; then
    echo "🔍 Detecting changed components..."
    CHANGED_FILES=$(git diff-tree --no-commit-id --name-only -r HEAD)

    COMPONENTS=()
    if echo "$CHANGED_FILES" | grep -q "^server/\|^deploy"; then
        COMPONENTS+=("vpn")
    fi
    if echo "$CHANGED_FILES" | grep -q "^client/\|^ipc/"; then
        COMPONENTS+=("vpn")
    fi
    if echo "$CHANGED_FILES" | grep -q "^extensions/video/\|^video-call/"; then
        COMPONENTS+=("video")
    fi
    if echo "$CHANGED_FILES" | grep -q "^menu-bar/"; then
        COMPONENTS+=("menu")
    fi

    # Check for new extensions
    NEW_EXTENSIONS=$(echo "$CHANGED_FILES" | grep "^extensions/" | cut -d'/' -f2 | sort -u | grep -v "framework" || true)
    for ext in $NEW_EXTENSIONS; do
        if [ "$ext" != "video" ]; then
            COMPONENTS+=("$ext")
        fi
    done

    if [ ${#COMPONENTS[@]} -eq 0 ]; then
        echo "ℹ️  No component changes detected"
        COMPONENTS=("all")
    fi

    echo "Components to deploy: ${COMPONENTS[*]}"
else
    COMPONENTS=("$COMPONENT")
fi

echo ""
read -p "Continue? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Deployment cancelled."
    exit 0
fi

echo ""

# Deploy each component
for COMP in "${COMPONENTS[@]}"; do
    echo "🚀 Deploying component: $COMP"

    UPDATE_ENDPOINT="http://$VPN_SERVER:$UPDATE_PORT/update/init?component=$COMP"
    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$UPDATE_ENDPOINT" 2>&1)
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" = "200" ]; then
        echo "  ✅ $COMP deployed successfully"
    else
        echo "  ❌ $COMP deployment failed (HTTP $HTTP_CODE)"
        echo "  Response: $BODY"
    fi
done

echo ""
echo "========================================"
echo "Deployment complete! 🎉"
echo ""
echo "What happens next:"
echo "  • Server broadcasts UPDATE_<COMPONENT> to clients"
echo "  • Only affected components restart"
echo "  • VPN stays running during extension updates"
echo "========================================"
