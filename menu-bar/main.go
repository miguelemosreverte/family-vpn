package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlantern/systray"
	"github.com/sqweek/dialog"
)

type VPNState struct {
	Connected     bool
	Server        string
	IP            string
	ConnectedAt   time.Time
	BytesSent     int64
	BytesReceived int64
	Process       *exec.Cmd
}

var (
	vpnState = &VPNState{Connected: false}
	mStatus  *systray.MenuItem
	mToggle  *systray.MenuItem
	mServer  *systray.MenuItem
	mIP      *systray.MenuItem
	mDuration *systray.MenuItem
	mData    *systray.MenuItem

	// VPN Configuration - read from environment or use defaults
	vpnServerHost = getEnv("VPN_SERVER_HOST", "95.217.238.72") // Default server from family-vpn
	vpnServerPort = getEnv("VPN_SERVER_PORT", "8888")
)

// getEnv reads an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	updateMenuBarIcon()
	systray.SetTooltip("Family VPN - Click to open")

	// Header
	mTitle := systray.AddMenuItem("Family VPN Manager", "VPN Manager Application")
	mTitle.Disable()
	systray.AddSeparator()

	// Status section
	mStatus = systray.AddMenuItem("● Disconnected", "Current connection status")
	mStatus.Disable()
	systray.AddSeparator()

	// Toggle button
	mToggle = systray.AddMenuItem("Connect to VPN", "Connect or disconnect VPN")

	systray.AddSeparator()

	// Connection details
	mServer = systray.AddMenuItem("Server: Not connected", "Current server location")
	mServer.Disable()
	mIP = systray.AddMenuItem("IP Address: ---", "Your IP address")
	mIP.Disable()
	mDuration = systray.AddMenuItem("Connected: ---", "Connection duration")
	mDuration.Disable()
	mData = systray.AddMenuItem("Data: --- / ---", "Data sent/received")
	mData.Disable()

	systray.AddSeparator()

	// About and Quit
	mAbout := systray.AddMenuItem("About", "About this VPN")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit VPN Manager")

	// Start data updater
	go dataUpdater()

	// Auto-connect on startup (REQUIRED - not optional)
	go func() {
		// Wait for system to initialize (network, etc.)
		time.Sleep(2 * time.Second)

		// Try to connect automatically
		if !vpnState.Connected {
			log.Println("Auto-connecting to VPN on startup...")
			toggleVPN()
		}
	}()

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				toggleVPN()
			case <-mAbout.ClickedCh:
				showAbout()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func toggleVPN() {
	if vpnState.Connected {
		// Disconnect VPN
		disconnectVPN()
	} else {
		// Connect VPN
		connectVPN()
	}
}

func connectVPN() {
	// Update UI to show connecting state
	mStatus.SetTitle("● Connecting...")
	mToggle.SetTitle("Cancel")
	updateConnectionDetails()

	// Get the directory where the app is running
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Failed to get executable path: %v", err)
		dialog.Message("Failed to start VPN: %v", err).Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}
	appDir := filepath.Dir(exePath)

	// Look for vpn-client in the parent directory's client folder
	vpnClientPath := filepath.Join(filepath.Dir(appDir), "client", "vpn-client")

	// If not found, try same directory
	if _, err := os.Stat(vpnClientPath); os.IsNotExist(err) {
		vpnClientPath = filepath.Join(appDir, "vpn-client")
	}

	// Check if vpn-client exists
	if _, err := os.Stat(vpnClientPath); os.IsNotExist(err) {
		dialog.Message("VPN client not found at:\n%s\n\nPlease build the VPN client first.", vpnClientPath).Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}

	// Get sudo password using macOS graphical prompt
	passwordScript := `osascript -e 'Tell application "System Events" to display dialog "Family VPN needs administrator access to create network interfaces.\n\nPlease enter your password:" with title "Family VPN" default answer "" with hidden answer' -e 'text returned of result'`
	passwordCmd := exec.Command("sh", "-c", passwordScript)
	passwordOutput, err := passwordCmd.Output()
	if err != nil {
		log.Printf("User cancelled or failed to provide password: %v", err)
		// Don't show error dialog on auto-connect - just log it
		log.Println("Failed to get sudo password - VPN will not connect automatically")
		handleConnectionFailure()
		return
	}
	password := strings.TrimSpace(string(passwordOutput))

	// Spawn VPN client with sudo using the password
	serverAddr := fmt.Sprintf("%s:%s", vpnServerHost, vpnServerPort)
	cmd := exec.Command("sudo", "-S", vpnClientPath, "-server", serverAddr, "-encrypt", "--no-timeout")

	// Pass password to sudo via stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("Failed to get stdin pipe: %v", err)
		dialog.Message("Failed to start VPN: %v", err).Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}

	// Write password to sudo
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, password+"\n")
	}()

	// Get stdout/stderr for monitoring
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to get stdout: %v", err)
		dialog.Message("Failed to start VPN: %v", err).Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Failed to get stderr: %v", err)
		dialog.Message("Failed to start VPN: %v", err).Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}

	// Start the VPN client
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start VPN client: %v", err)
		dialog.Message("Failed to start VPN: %v\n\nMake sure you have sudo permissions.", err).Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}

	vpnState.Process = cmd
	vpnState.Connected = true
	vpnState.ConnectedAt = time.Now()
	vpnState.BytesSent = 0
	vpnState.BytesReceived = 0

	// Monitor VPN client output in background
	go monitorVPNOutput(stdout, stderr)

	// Wait a moment for VPN to establish, then get real IP
	go func() {
		time.Sleep(3 * time.Second)
		if vpnState.Connected {
			ip := getPublicIP()
			if ip != "" {
				vpnState.IP = ip
			} else {
				vpnState.IP = "Unknown"
			}
			vpnState.Server = "Helsinki, Finland" // From family-vpn README
			updateConnectionDetails()
			dialog.Message("VPN Connected!\n\nServer: %s\nIP: %s", vpnState.Server, vpnState.IP).Title("Family VPN").Info()
		}
	}()

	// Update UI
	mStatus.SetTitle("● Connected")
	mToggle.SetTitle("Disconnect from VPN")
	updateConnectionDetails()
	updateMenuBarIcon()

	// Monitor process in background
	go func() {
		err := cmd.Wait()
		if vpnState.Connected {
			// Process died unexpectedly
			log.Printf("VPN client exited unexpectedly: %v", err)
			handleConnectionFailure()
			dialog.Message("VPN connection lost").Title("Family VPN").Error()
		}
	}()
}

func handleConnectionFailure() {
	// Safety: Ensure we're in disconnected state if connection fails
	// This prevents internet from being broken
	vpnState.Connected = false
	vpnState.Process = nil
	mStatus.SetTitle("● Disconnected")
	mToggle.SetTitle("Connect to VPN")
	updateConnectionDetails()
	updateMenuBarIcon()
}

func disconnectVPN() {
	if vpnState.Process != nil {
		// Kill the VPN client process
		if err := vpnState.Process.Process.Kill(); err != nil {
			log.Printf("Failed to kill VPN client: %v", err)
		}
		vpnState.Process = nil
	}

	vpnState.Connected = false
	mStatus.SetTitle("● Disconnected")
	mToggle.SetTitle("Connect to VPN")
	updateConnectionDetails()
	updateMenuBarIcon()

	dialog.Message("VPN Disconnected").Title("Family VPN").Info()
}

func monitorVPNOutput(stdout, stderr io.ReadCloser) {
	// Monitor stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[VPN] %s", line)
			// Could parse stats from output here if needed
		}
	}()

	// Monitor stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[VPN ERROR] %s", line)
		}
	}()
}

func getPublicIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ifconfig.me")
	if err != nil {
		log.Printf("Failed to get public IP: %v", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read IP response: %v", err)
		return ""
	}

	return strings.TrimSpace(string(body))
}

func dataUpdater() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Just update the connection details (time counter, etc.)
		// Real VPN stats could be parsed from VPN client output if needed
		updateConnectionDetails()
	}
}

func updateConnectionDetails() {
	if !vpnState.Connected {
		mServer.SetTitle("Server: Not connected")
		mIP.SetTitle("IP Address: ---")
		mDuration.SetTitle("Connected: ---")
		mData.SetTitle("Data: --- / ---")
	} else {
		duration := time.Since(vpnState.ConnectedAt).Round(time.Second)
		mServer.SetTitle(fmt.Sprintf("Server: %s", vpnState.Server))
		mIP.SetTitle(fmt.Sprintf("IP Address: %s", vpnState.IP))
		mDuration.SetTitle(fmt.Sprintf("Connected: %s", duration))
		mData.SetTitle(fmt.Sprintf("Data: ↑ %s / ↓ %s",
			formatBytes(vpnState.BytesSent),
			formatBytes(vpnState.BytesReceived)))
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func updateMenuBarIcon() {
	if vpnState.Connected {
		systray.SetIcon(getConnectedIcon())
		systray.SetTitle("🔒")
		systray.SetTooltip("VPN: Connected - Click for details")
	} else {
		systray.SetIcon(getDisconnectedIcon())
		systray.SetTitle("🔓")
		systray.SetTooltip("VPN: Disconnected - Click to connect")
	}
}

func showAbout() {
	about := `Family VPN Manager

Secure, encrypted VPN built from scratch
with AES-256-GCM encryption.

Server: Helsinki, Finland
Encryption: AES-256-GCM

Made with love for the family. ❤️

💕 2025`

	dialog.Message(about).Title("About Family VPN").Info()
}

func getConnectedIcon() []byte {
	// Green shield icon (base64 decoded PNG)
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}

func getDisconnectedIcon() []byte {
	// Gray shield icon
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}

func onExit() {
	// Disconnect VPN if connected
	if vpnState.Connected && vpnState.Process != nil {
		log.Println("Disconnecting VPN before exit...")
		if err := vpnState.Process.Process.Kill(); err != nil {
			log.Printf("Failed to kill VPN client: %v", err)
		}
	}
	fmt.Println("Family VPN Manager exited")
}
