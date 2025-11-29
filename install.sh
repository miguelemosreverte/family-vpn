#!/bin/bash
# Family VPN Installation Script
# Supports fresh installation and updates on existing installations
# Fully automated - reads sudo password from .env file

set -e

echo "🚀 Family VPN Installation Script"
echo "=================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Detect OS
OS_TYPE="$(uname -s)"
if [[ "$OS_TYPE" != "Darwin" ]]; then
    echo -e "${RED}❌ This script currently only supports macOS${NC}"
    exit 1
fi

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo "📍 Working directory: $SCRIPT_DIR"
echo ""

# Load environment variables from .env file
if [ -f "$SCRIPT_DIR/.env" ]; then
    # Only export valid KEY=VALUE lines, skip comments and empty lines
    while IFS='=' read -r key value; do
        # Skip empty lines, comments, and lines without =
        if [[ -n "$key" && ! "$key" =~ ^# && -n "$value" ]]; then
            # Remove leading/trailing whitespace from key
            key=$(echo "$key" | xargs)
            # Only export if key is a valid variable name
            if [[ "$key" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
                export "$key=$value"
            fi
        fi
    done < "$SCRIPT_DIR/.env"
    echo -e "${GREEN}✓ Loaded configuration from .env${NC}"
else
    echo -e "${YELLOW}⚠️  No .env file found - sudo operations may require manual password entry${NC}"
fi

# Helper function to run sudo commands with password from .env
run_sudo() {
    if [ -n "$SUDO_PASSWORD" ]; then
        echo "$SUDO_PASSWORD" | sudo -S "$@" 2>/dev/null
    else
        sudo "$@"
    fi
}

echo ""

# Check dependencies
echo "🔍 Checking dependencies..."

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go from https://golang.org/dl/${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go $(go version | awk '{print $3}')${NC}"

# Check Node.js
if ! command -v node &> /dev/null; then
    echo -e "${RED}❌ Node.js is not installed. Please install Node.js from https://nodejs.org/${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Node.js $(node --version)${NC}"

# Check npm
if ! command -v npm &> /dev/null; then
    echo -e "${RED}❌ npm is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ npm $(npm --version)${NC}"

# Check ImageMagick (for icon generation) - only needed if icon doesn't exist
if [ ! -f "desktop-app/assets/icon.icns" ]; then
    if ! command -v magick &> /dev/null; then
        echo -e "${YELLOW}⚠️  ImageMagick not found. Installing via Homebrew...${NC}"
        if command -v brew &> /dev/null; then
            brew install imagemagick
        else
            echo -e "${RED}❌ Homebrew not found. Please install ImageMagick manually${NC}"
            exit 1
        fi
    fi
    echo -e "${GREEN}✓ ImageMagick $(magick --version | head -1 | awk '{print $3}')${NC}"
else
    echo -e "${GREEN}✓ Icon already exists, skipping ImageMagick check${NC}"
fi

echo ""
echo "🔌 Building VPN Client..."
echo "-------------------------"

# Build VPN client FIRST (required by menu bar)
cd client
if [ ! -f "vpn-client" ]; then
    echo "Building VPN client..."
    GOTOOLCHAIN=local go build -o vpn-client . 2>/dev/null || go build -o vpn-client .
    if [ $? -ne 0 ]; then
        echo -e "${RED}❌ Failed to build VPN client${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ VPN client built successfully${NC}"
else
    echo -e "${GREEN}✓ VPN client already exists${NC}"
fi
cd ..

echo ""
echo "🔨 Building Menu Bar App..."
echo "----------------------------"

# Build menu bar app
cd menu-bar
if [ ! -f "family-vpn-menubar" ]; then
    echo "Building menu bar app..."
    go mod download 2>/dev/null || true
    GOTOOLCHAIN=local go build -o family-vpn-menubar . 2>/dev/null || go build -o family-vpn-menubar .
    if [ $? -ne 0 ]; then
        echo -e "${RED}❌ Failed to build menu bar app${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Menu bar app built successfully${NC}"
else
    echo -e "${GREEN}✓ Menu bar app already exists${NC}"
fi
cd ..

echo ""
echo "🎨 Generating App Icon..."
echo "-------------------------"

# Generate icon only if it doesn't exist
cd desktop-app/assets

if [ ! -f "icon.icns" ]; then
    # Create solid orange base with shield
    if [ ! -f "family-vpn-icon.png" ]; then
        magick -size 1024x1024 canvas:"#FF8C42" -alpha off \
            -fill white -stroke none \
            -draw "path 'M 512,272 L 700,352 L 700,572 Q 700,692 512,832 Q 324,692 324,572 L 324,352 Z'" \
            -fill "#FF8C42" \
            -draw "roundrectangle 462,512 562,662 15,15" \
            -fill white \
            -draw "circle 512,572 512,542" \
            family-vpn-icon.png
        echo -e "${GREEN}✓ Icon PNG generated${NC}"
    else
        echo -e "${YELLOW}✓ PNG icon already exists${NC}"
    fi

    # Generate iconset
    rm -rf icon.iconset
    mkdir -p icon.iconset

    for size in 16 32 64 128 256 512; do
        magick family-vpn-icon.png -resize ${size}x${size} icon.iconset/icon_${size}x${size}.png
        magick family-vpn-icon.png -resize $((size*2))x$((size*2)) icon.iconset/icon_${size}x${size}@2x.png
    done

    # Build .icns
    iconutil -c icns icon.iconset -o icon.icns
    echo -e "${GREEN}✓ Icon.icns generated${NC}"
else
    echo -e "${GREEN}✓ Icon.icns already exists, skipping generation${NC}"
fi

cd ../..

echo ""
echo "🖥️  Building Desktop App..."
echo "---------------------------"

# Build desktop app
cd desktop-app

# Install dependencies
npm install
if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Failed to install desktop app dependencies${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Dependencies installed${NC}"

# Build with electron-builder
npm run build
if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Failed to build desktop app${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Desktop app built successfully${NC}"

cd ..

echo ""
echo "📦 Installing Applications..."
echo "-----------------------------"

# Install desktop app
DESKTOP_APP_PATH="/Applications/Family VPN.app"
if [ -d "$DESKTOP_APP_PATH" ]; then
    echo -e "${YELLOW}⚠️  Removing existing desktop app...${NC}"
    run_sudo rm -rf "$DESKTOP_APP_PATH"
fi

run_sudo cp -R "desktop-app/dist/mac-arm64/Family VPN.app" "/Applications/"
echo -e "${GREEN}✓ Desktop app installed to /Applications/${NC}"

# Install menu bar app
MENU_BAR_PATH="/usr/local/bin/family-vpn-menubar"
if [ -f "$MENU_BAR_PATH" ]; then
    echo -e "${YELLOW}⚠️  Removing existing menu bar app...${NC}"
    run_sudo rm -f "$MENU_BAR_PATH"
fi

run_sudo cp "menu-bar/family-vpn-menubar" "/usr/local/bin/"
run_sudo chmod +x "/usr/local/bin/family-vpn-menubar"
echo -e "${GREEN}✓ Menu bar app installed to /usr/local/bin/${NC}"

# Install VPN client
VPN_CLIENT_PATH="/usr/local/bin/family-vpn-client"
if [ -f "$VPN_CLIENT_PATH" ]; then
    echo -e "${YELLOW}⚠️  Removing existing VPN client...${NC}"
    run_sudo rm -f "$VPN_CLIENT_PATH"
fi

run_sudo cp "client/vpn-client" "/usr/local/bin/family-vpn-client"
run_sudo chmod +x "/usr/local/bin/family-vpn-client"
echo -e "${GREEN}✓ VPN client installed to /usr/local/bin/${NC}"

# Install routing fix script
ROUTING_SCRIPT="/usr/local/bin/family-vpn-fix-routing"
run_sudo cp "client/fix-routing.sh" "$ROUTING_SCRIPT"
run_sudo chmod +x "$ROUTING_SCRIPT"
echo -e "${GREEN}✓ Routing fix script installed${NC}"

echo ""
echo "🔐 Setting up configuration..."
echo "------------------------------"

# Check if .env exists in project
if [ ! -f ".env" ]; then
    echo -e "${YELLOW}⚠️  .env file not found. Creating from .env.example...${NC}"
    if [ -f ".env.example" ]; then
        cp .env.example .env
        echo -e "${YELLOW}⚠️  Please edit .env file with your VPN server details${NC}"
    else
        echo -e "${RED}❌ .env.example not found${NC}"
    fi
else
    echo -e "${GREEN}✓ .env file exists${NC}"
fi

# Install .env to /usr/local/ for menu bar app to find
run_sudo cp ".env" "/usr/local/.env"
run_sudo chmod 644 "/usr/local/.env"
echo -e "${GREEN}✓ Configuration installed to /usr/local/.env${NC}"

echo ""
echo "🚀 Configuring auto-start and Dock..."
echo "--------------------------------------"

# Add to Login Items (start on login)
osascript -e 'tell application "System Events" to make login item at end with properties {path:"/Applications/Family VPN.app", hidden:false}' 2>/dev/null && \
    echo -e "${GREEN}✓ Added to Login Items (starts on login)${NC}" || \
    echo -e "${YELLOW}⚠️  Could not add to Login Items - add manually in System Preferences${NC}"

# Add to Dock using dockutil or defaults
if command -v dockutil &> /dev/null; then
    dockutil --add "/Applications/Family VPN.app" --no-restart 2>/dev/null
    killall Dock 2>/dev/null
    echo -e "${GREEN}✓ Added to Dock${NC}"
else
    # Use defaults to add to Dock
    defaults write com.apple.dock persistent-apps -array-add "<dict><key>tile-data</key><dict><key>file-data</key><dict><key>_CFURLString</key><string>/Applications/Family VPN.app</string><key>_CFURLStringType</key><integer>0</integer></dict></dict></dict>" 2>/dev/null
    killall Dock 2>/dev/null
    echo -e "${GREEN}✓ Added to Dock${NC}"
fi

echo ""
echo "✅ Installation Complete!"
echo "========================="
echo ""
echo "To run Family VPN:"
echo "  1. Open 'Family VPN' from Applications folder"
echo "  2. The menu bar icon will appear automatically"
echo "  3. Or run from terminal: cd $(pwd)/menu-bar && ./family-vpn-menubar"
echo ""
echo "To update existing installation:"
echo "  - Run this script again: ./install.sh"
echo "  - Or use the hidden update button in the app (Cmd+Shift+U)"
echo ""
echo "Logs:"
echo "  - Menu bar: /tmp/menubar.log"
echo "  - Desktop app: Check Developer Tools (Cmd+Opt+I)"
echo ""
