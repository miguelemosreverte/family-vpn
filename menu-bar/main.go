package main

import (
	"bufio"
	"flag"
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

	// Development mode - disables auto-connect
	devMode bool
)

// getEnv reads an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// loadEnvFile loads environment variables from .env file in parent directory
func loadEnvFile() error {
	// Get executable path and look for .env in parent directory
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	// Go up to parent directory (family-vpn root)
	parentDir := filepath.Dir(filepath.Dir(exePath))
	envPath := filepath.Join(parentDir, ".env")

	// Try to read the .env file
	file, err := os.Open(envPath)
	if err != nil {
		// If .env doesn't exist in parent, that's okay - will use defaults
		log.Printf("No .env file found at %s, using defaults", envPath)
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			os.Setenv(key, value)
		}
	}

	log.Printf("Loaded configuration from %s", envPath)
	return scanner.Err()
}

// checkForUpdates checks if there are new commits on GitHub
func checkForUpdates() (bool, error) {
	// Get executable path to find repo directory
	exePath, err := os.Executable()
	if err != nil {
		return false, err
	}
	repoDir := filepath.Dir(filepath.Dir(exePath))

	// Fetch latest from remote
	fetchCmd := exec.Command("git", "-C", repoDir, "fetch", "origin", "main")
	if err := fetchCmd.Run(); err != nil {
		return false, fmt.Errorf("git fetch failed: %v", err)
	}

	// Compare local and remote HEAD
	localCmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	localOutput, err := localCmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get local HEAD: %v", err)
	}

	remoteCmd := exec.Command("git", "-C", repoDir, "rev-parse", "origin/main")
	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get remote HEAD: %v", err)
	}

	localHash := strings.TrimSpace(string(localOutput))
	remoteHash := strings.TrimSpace(string(remoteOutput))

	return localHash != remoteHash, nil
}

// performUpdate pulls changes, rebuilds, and restarts the app
func performUpdate() error {
	log.Println("🔄 Update available! Starting auto-update...")

	// Get paths
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(filepath.Dir(exePath))

	// Pull latest changes
	log.Println("Pulling latest changes from GitHub...")
	pullCmd := exec.Command("git", "-C", repoDir, "pull", "origin", "main")
	if output, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %v\n%s", err, output)
	}

	// Rebuild
	log.Println("Rebuilding menu bar app...")
	buildScript := filepath.Join(repoDir, "build-menubar.sh")
	buildCmd := exec.Command(buildScript)
	buildCmd.Dir = repoDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build failed: %v\n%s", err, output)
	}

	log.Println("✅ Update complete! Restarting app...")

	// Restart the app
	restartCmd := exec.Command(exePath)
	if err := restartCmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %v", err)
	}

	// Exit current process
	os.Exit(0)
	return nil
}

// watchUpdateSignal watches for update signal file from VPN client
func watchUpdateSignal() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Failed to get home dir: %v", err)
		return
	}
	signalFile := filepath.Join(homeDir, ".family-vpn-update-signal")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		// Check if signal file exists
		if _, err := os.Stat(signalFile); err == nil {
			log.Println("🔔 Update signal received from VPN server!")

			// Remove signal file
			os.Remove(signalFile)

			// Trigger update
			log.Println("Starting immediate update...")
			if err := performUpdate(); err != nil {
				log.Printf("Update failed: %v", err)
			}
		}
	}
}

// autoUpdater runs in background and checks for updates every hour
func autoUpdater() {
	// Start signal file watcher (for real-time updates from VPN server)
	go watchUpdateSignal()

	// Wait 5 minutes before first check (let app start up first)
	time.Sleep(5 * time.Minute)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		log.Println("Checking for updates (polling)...")

		hasUpdate, err := checkForUpdates()
		if err != nil {
			log.Printf("Update check failed: %v", err)
		} else if hasUpdate {
			log.Println("Update found! Starting update process...")
			if err := performUpdate(); err != nil {
				log.Printf("Update failed: %v", err)
			}
		} else {
			log.Println("No updates available")
		}

		<-ticker.C
	}
}

func main() {
	// Parse command-line flags
	flag.BoolVar(&devMode, "dev", false, "Development mode - disable auto-connect on startup")
	flag.Parse()

	if devMode {
		log.Println("🔧 Development mode enabled - auto-connect disabled")
	}

	// Load .env file before starting
	if err := loadEnvFile(); err != nil {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}

	// Start auto-updater in background
	go autoUpdater()

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

	// Auto-connect on startup (unless in dev mode)
	if !devMode {
		go func() {
			// Wait for system to initialize (network, etc.)
			time.Sleep(2 * time.Second)

			// Try to connect automatically
			if !vpnState.Connected {
				log.Println("Auto-connecting to VPN on startup...")
				toggleVPN()
			}
		}()
	} else {
		log.Println("⏸️  Auto-connect skipped (dev mode)")
	}

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

	// Get sudo password from environment (.env file)
	password := os.Getenv("SUDO_PASSWORD")
	if password == "" {
		log.Printf("SUDO_PASSWORD not found in .env file")
		dialog.Message("SUDO_PASSWORD not found in .env file.\n\nPlease add your password to the .env file.").Title("Family VPN").Error()
		handleConnectionFailure()
		return
	}

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
	log.Println("Disconnecting VPN...")

	// Get sudo password from environment
	password := os.Getenv("SUDO_PASSWORD")
	if password == "" {
		log.Printf("SUDO_PASSWORD not found - cannot disconnect")
		return
	}

	// Kill VPN client process with sudo (required since it runs as root)
	log.Println("Killing vpn-client processes with sudo...")
	killCmd := exec.Command("sudo", "-S", "pkill", "-9", "-f", "vpn-client")
	killStdin, _ := killCmd.StdinPipe()
	go func() {
		defer killStdin.Close()
		io.WriteString(killStdin, password+"\n")
	}()
	killCmd.Run()

	// Wait for process to die
	time.Sleep(500 * time.Millisecond)

	// Manually restore routing and DNS (can't rely on VPN client cleanup)
	log.Println("Restoring network configuration...")

	// Restore default route
	restoreRoute := exec.Command("sudo", "-S", "route", "-n", "delete", "default")
	routeStdin, _ := restoreRoute.StdinPipe()
	go func() {
		defer routeStdin.Close()
		io.WriteString(routeStdin, password+"\n")
	}()
	restoreRoute.Run()

	// Add back default route through original gateway
	addRoute := exec.Command("sudo", "-S", "route", "-n", "add", "-net", "default", "192.168.100.1")
	addStdin, _ := addRoute.StdinPipe()
	go func() {
		defer addStdin.Close()
		io.WriteString(addStdin, password+"\n")
	}()
	addRoute.Run()

	// Restore DNS to automatic
	restoreDNS := exec.Command("networksetup", "-setdnsservers", "Wi-Fi", "Empty")
	restoreDNS.Run()

	vpnState.Process = nil
	vpnState.Connected = false
	mStatus.SetTitle("● Disconnected")
	mToggle.SetTitle("Connect to VPN")
	updateConnectionDetails()
	updateMenuBarIcon()

	log.Println("VPN disconnected and network restored successfully")
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

	// Try api.ipify.org first (reliable, always returns plain text IP)
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		log.Printf("Failed to get public IP from ipify: %v", err)
		// Fallback to icanhazip.com
		resp, err = client.Get("https://icanhazip.com")
		if err != nil {
			log.Printf("Failed to get public IP from icanhazip: %v", err)
			return "Unknown"
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read IP response: %v", err)
		return "Unknown"
	}

	ip := strings.TrimSpace(string(body))

	// Validate that we got an IP address, not HTML
	// IP addresses should be less than 50 characters
	if len(ip) > 50 || strings.Contains(ip, "<") || strings.Contains(ip, ">") {
		log.Printf("Received invalid IP response (possibly HTML): %s", ip[:min(len(ip), 100)])
		return "Unknown"
	}

	return ip
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// getVersionInfo gets current git commit info
func getVersionInfo() string {
	exePath, err := os.Executable()
	if err != nil {
		return "Version: Unknown"
	}
	repoDir := filepath.Dir(filepath.Dir(exePath))

	// Get commit hash
	hashCmd := exec.Command("git", "-C", repoDir, "rev-parse", "--short", "HEAD")
	hashOutput, err := hashCmd.Output()
	if err != nil {
		return "Version: Unknown"
	}
	hash := strings.TrimSpace(string(hashOutput))

	// Get commit date
	dateCmd := exec.Command("git", "-C", repoDir, "log", "-1", "--format=%cd", "--date=format:%Y-%m-%d %H:%M")
	dateOutput, _ := dateCmd.Output()
	date := strings.TrimSpace(string(dateOutput))

	// Get commit message
	msgCmd := exec.Command("git", "-C", repoDir, "log", "-1", "--format=%s")
	msgOutput, _ := msgCmd.Output()
	message := strings.TrimSpace(string(msgOutput))

	return fmt.Sprintf("Version: %s\nDate: %s\n\n%s", hash, date, message)
}

func showAbout() {
	version := getVersionInfo()

	about := fmt.Sprintf(`Family VPN Manager

Secure, encrypted VPN built from scratch
with AES-256-GCM encryption.

Server: Helsinki, Finland
Encryption: AES-256-GCM

%s

Made with love for the family. ❤️

💕 2025`, version)

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
