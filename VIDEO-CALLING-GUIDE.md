# 📹 Video Calling Feature - Quick Start Guide

## What's New

You can now make **spontaneous video calls** between family members over the VPN!

- Click on any connected peer → "📹 Video Call"
- Browser opens with live video feed
- See each other instantly
- No dialogs, no friction - just click and connect!

## Update Instructions

### On Each Computer:

1. **Pull latest code:**
   ```bash
   cd family-vpn
   git pull origin main
   ```

2. **Rebuild menu-bar app:**
   ```bash
   cd menu-bar
   go build -o family-vpn-manager main.go
   ```

3. **Restart menu-bar app:**
   ```bash
   # Kill old version
   pkill -f "family-vpn-manager"

   # Start new version
   ./family-vpn-manager
   ```

4. **Verify it's running:**
   - Look for the VPN icon in your menu bar
   - Check that it shows "Connected" status

## How to Use Video Calling

### Starting a Video Call:

1. **Make sure VPN is connected**
   - Menu bar should show "● Connected"

2. **Click on a peer:**
   - You'll see a list of connected family members
   - Example: "🖥️ MacBook-Air (10.8.0.2)"

3. **Choose an action:**
   - **📹 Video Call** - Start instant video call
   - **🖥️ Screen Sharing** - Remote desktop access (same as before)

4. **Browser opens automatically:**
   - Your camera activates
   - You see yourself in the corner (small preview)
   - Main screen shows "Connecting..." then your peer's video

### Video Call Controls:

- **📹 Video button** - Toggle camera on/off
- **🎤 Mic button** - Toggle microphone on/off
- **Close browser window** - End the call

### First Time Setup:

The browser will ask for permissions:
- ✅ Allow camera access
- ✅ Allow microphone access

These permissions are saved for future calls.

## Current Limitations (MVP)

**Both people need to click "Video Call":**
- Right now, each person needs to initiate the call on their side
- Next version will auto-open the peer's video window (true spontaneous calls!)

**Browser-based UI:**
- Uses your default browser for video interface
- This ensures perfect camera/mic support on macOS
- Lightweight - no Electron or complex dependencies

## Troubleshooting

### "Camera not working"
- Check System Preferences → Privacy → Camera
- Make sure browser has camera permissions
- Try reloading the browser page

### "No peers showing up"
- Make sure both computers are connected to VPN
- Wait a few seconds for peer discovery
- Check menu bar shows "● Connected"

### "Browser doesn't open"
- Check logs: `tail -f /tmp/vpn-menu.log`
- Try clicking "Video Call" again
- Restart menu-bar app if needed

### "Build fails with video-call module error"
- The video-call package is local (not published)
- Make sure you're in the menu-bar directory
- The go.mod has a `replace` directive pointing to ../video-call

## Technical Details

**Architecture:**
- WebRTC for browser-to-browser video/audio
- WebSocket for signaling over encrypted VPN
- Local HTTP server (random port) serves video UI
- All traffic encrypted via VPN tunnel

**Privacy:**
- Video streams directly peer-to-peer over VPN
- No external servers involved
- No data leaves your private network

## What's Next

Planned improvements:
- **Spontaneous auto-open:** When you call, peer's video window opens automatically
- **Group calls:** Video chat with multiple family members
- **Watch together:** Share movies/videos with family
- **Screen sharing in video:** Share your screen during a call

---

**Enjoy video calling with your family! 🎉**

For issues or questions, check the logs or restart the menu-bar app.
