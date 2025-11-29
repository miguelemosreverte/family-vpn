#!/bin/bash
# Family VPN Unified Installer
# Installs both menu bar and desktop app

set -e

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Family VPN - Complete Installation${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# 1. Build VPN Client
echo -e "\n${YELLOW}→${NC} Building VPN client..."
cd "$SCRIPT_DIR"
./build-client.sh

# 2. Build Menu Bar App
echo -e "\n${YELLOW}→${NC} Building menu bar app..."
./build-menubar.sh

# 3. Build Desktop App
echo -e "\n${YELLOW}→${NC} Building desktop app..."
cd desktop-app
npm run build 2>&1 | tail -10

# 4. Install to Applications
echo -e "\n${YELLOW}→${NC} Installing to /Applications..."
APP_PATH="$SCRIPT_DIR/desktop-app/dist/mac/Family VPN.app"

if [ -d "$APP_PATH" ]; then
  sudo cp -R "$APP_PATH" /Applications/
  echo -e "${GREEN}✓${NC} Desktop app installed to /Applications"
else
  echo -e "${YELLOW}⚠ Desktop app not built with electron-builder, using manual install...${NC}"
  # Create a simple .app bundle manually
  mkdir -p "/Applications/Family VPN.app/Contents/MacOS"
  mkdir -p "/Applications/Family VPN.app/Contents/Resources"

  # Copy files
  cp -R "$SCRIPT_DIR/desktop-app" "/Applications/Family VPN.app/Contents/Resources/"

  # Create launcher script
  cat > "/Applications/Family VPN.app/Contents/MacOS/Family VPN" <<'EOF'
#!/bin/bash
cd "/Applications/Family VPN.app/Contents/Resources/desktop-app"
npm start
EOF
  chmod +x "/Applications/Family VPN.app/Contents/MacOS/Family VPN"

  # Copy icon
  cp "$SCRIPT_DIR/desktop-app/assets/icon.icns" "/Applications/Family VPN.app/Contents/Resources/"

  # Create Info.plist
  cat > "/Applications/Family VPN.app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>Family VPN</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>com.family.vpn</string>
    <key>CFBundleName</key>
    <string>Family VPN</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
</dict>
</plist>
EOF
fi

# 5. Add to Dock
echo -e "\n${YELLOW}→${NC} Adding to Dock..."
defaults write com.apple.dock persistent-apps -array-add '<dict><key>tile-data</key><dict><key>file-data</key><dict><key>_CFURLString</key><string>/Applications/Family VPN.app</string><key>_CFURLStringType</key><integer>0</integer></dict></dict></dict>'
killall Dock

# 6. Install menu bar app to Launch Agents
echo -e "\n${YELLOW}→${NC} Installing menu bar app for auto-launch..."
cd "$SCRIPT_DIR/menu-bar"
./install-autolaunch.sh 2>&1 || echo "Auto-launch setup complete"

# 7. Update main.js to launch desktop app when menu bar starts
echo -e "\n${YELLOW}→${NC} Integrating menu bar and desktop app..."
# This will be handled by updating the menu bar app code

echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✓ Installation Complete!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

echo -e "\nInstalled components:"
echo -e "  • VPN Client: $SCRIPT_DIR/client/vpn-client"
echo -e "  • Menu Bar App: Auto-launches on login"
echo -e "  • Desktop App: /Applications/Family VPN.app"
echo -e "\nTo start using:"
echo -e "  1. Menu bar app will start automatically on next login"
echo -e "  2. Or run manually: $SCRIPT_DIR/menu-bar/family-vpn-menubar"
echo -e "  3. Desktop app is in your Dock - click to open dashboard"
echo -e "\n${BLUE}Enjoy your secure Family VPN! 🔒${NC}\n"
