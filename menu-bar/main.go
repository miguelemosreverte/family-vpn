package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
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

// PeerInfo represents a connected VPN peer
type PeerInfo struct {
	Hostname    string `json:"hostname"`
	VPNAddress  string `json:"vpn_address"`
	PublicIP    string `json:"public_ip"`
	ConnectedAt string `json:"connected_at"`
	OS          string `json:"os"`
}

var (
	vpnState = &VPNState{Connected: false}
	mStatus  *systray.MenuItem
	mToggle  *systray.MenuItem
	mServer  *systray.MenuItem
	mIP      *systray.MenuItem
	mDuration *systray.MenuItem
	mData    *systray.MenuItem

	// VPN Configuration - will be loaded from .env file in main()
	vpnServerHost string
	vpnServerPort string

	// Development mode - disables auto-connect
	devMode bool

	// Peer list
	connectedPeers []*PeerInfo
	peerMenuItems  map[string]*systray.MenuItem // Map peer VPN address to menu item

	// Extension manager
	extensionManager *ExtensionManager
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

// performVPNUpdate rebuilds VPN client and menu-bar app for full system updates
func performVPNUpdate() error {
	log.Println("🔄 VPN core update! Starting full rebuild...")

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

	// Rebuild VPN client
	log.Println("Rebuilding VPN client...")
	buildClientScript := filepath.Join(repoDir, "build-client.sh")
	buildClientCmd := exec.Command(buildClientScript)
	buildClientCmd.Dir = repoDir
	if output, err := buildClientCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("VPN client build failed: %v\n%s", err, output)
	}

	// Rebuild menu-bar app
	log.Println("Rebuilding menu bar app...")
	buildScript := filepath.Join(repoDir, "build-menubar.sh")
	buildCmd := exec.Command(buildScript)
	buildCmd.Dir = repoDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("menu-bar build failed: %v\n%s", err, output)
	}

	log.Println("✅ Full update complete! Restarting app...")

	// Restart the app (which will start new VPN client)
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

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		// Check if signal file exists
		data, err := os.ReadFile(signalFile)
		if err != nil {
			continue // File doesn't exist or can't read
		}

		updateMessage := strings.TrimSpace(string(data))
		log.Printf("🔔 Update signal received from VPN server: %s", updateMessage)

		// Remove signal file
		os.Remove(signalFile)

		// Handle component-specific updates
		switch {
		case updateMessage == "UPDATE_VIDEO":
			log.Println("[UPDATE] Restarting video extension...")
			if extensionManager != nil {
				go func() {
					if err := extensionManager.RestartExtension("video"); err != nil {
						log.Printf("[UPDATE] Failed to restart video extension: %v", err)
					}
				}()
			}

		case updateMessage == "UPDATE_MENU":
			log.Println("[UPDATE] Restarting menu-bar (self)...")
			if err := performUpdate(); err != nil {
				log.Printf("[UPDATE] Failed to update menu-bar: %v", err)
			}

		case updateMessage == "UPDATE_VPN" || updateMessage == "UPDATE_ALL":
			log.Println("[UPDATE] Full system update (VPN core + all components)...")
			// Stop extensions first
			if extensionManager != nil {
				extensionManager.StopAll()
			}
			// Perform full VPN update (rebuilds VPN client + menu-bar)
			if err := performVPNUpdate(); err != nil {
				log.Printf("[UPDATE] Failed to update: %v", err)
			}

		default:
			// Check if it's an extension update (UPDATE_<extensionname>)
			if strings.HasPrefix(updateMessage, "UPDATE_") {
				extName := strings.ToLower(strings.TrimPrefix(updateMessage, "UPDATE_"))
				log.Printf("[UPDATE] Restarting extension: %s", extName)
				if extensionManager != nil {
					go func() {
						if err := extensionManager.RestartExtension(extName); err != nil {
							log.Printf("[UPDATE] Failed to restart %s extension: %v", extName, err)
						}
					}()
				}
			} else {
				log.Printf("[UPDATE] Unknown update signal: %s", updateMessage)
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

	// Initialize VPN configuration from environment (after .env is loaded)
	vpnServerHost = getEnv("VPN_SERVER_HOST", "95.217.238.72")
	vpnServerPort = getEnv("VPN_SERVER_PORT", "443")
	log.Printf("VPN Server: %s:%s", vpnServerHost, vpnServerPort)

	// Initialize extension manager
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	repoDir := filepath.Dir(filepath.Dir(exePath))
	extensionManager = NewExtensionManager(repoDir)

	// Register video extension
	videoExtPath := filepath.Join(repoDir, "extensions", "video", "video-extension")
	extensionManager.RegisterExtension("video", videoExtPath, []string{"--vpn-port", "8889"})
	log.Printf("[EXT] Registered video extension: %s", videoExtPath)

	// Register SSH extension
	sshExtPath := filepath.Join(repoDir, "extensions", "ssh", "ssh-extension")
	extensionManager.RegisterExtension("ssh", sshExtPath, []string{"--vpn-port", "8889"})
	log.Printf("[EXT] Registered SSH extension: %s", sshExtPath)

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

	// Connected Family section (will be populated dynamically)
	mFamilyHeader := systray.AddMenuItem("👨‍👩‍👧‍👦 Connected Family", "Family members on VPN")
	mFamilyHeader.Disable()

	// Initialize peer menu items map
	peerMenuItems = make(map[string]*systray.MenuItem)

	systray.AddSeparator()

	// About and Quit
	mAbout := systray.AddMenuItem("About", "About this VPN")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit VPN Manager")

	// Start data updater
	go dataUpdater()

	// Start peer list updater
	go peerListUpdater()

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
	// SINGLETON: Check if VPN client is already running
	checkCmd := exec.Command("pgrep", "-f", "vpn-client")
	if output, err := checkCmd.Output(); err == nil && len(output) > 0 {
		log.Println("⚠️  VPN client already running - skipping duplicate start")
		// Assume already connected
		vpnState.Connected = true
		mStatus.SetTitle("● Connected")
		mToggle.SetTitle("Disconnect from VPN")
		updateConnectionDetails()
		updateMenuBarIcon()
		return
	}

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
	cmd := exec.Command("sudo", "-S", vpnClientPath, "-server", serverAddr, "-encrypt", "-tls", "--no-timeout")

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

	// Wait a moment for VPN to establish, then get real IP and start extensions
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

			// Start extensions
			if extensionManager != nil {
				log.Println("[EXT] Starting extensions...")
				if err := extensionManager.StartAll(); err != nil {
					log.Printf("[EXT] Failed to start extensions: %v", err)
				}
			}

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

	// Stop extensions first
	if extensionManager != nil {
		log.Println("[EXT] Stopping extensions...")
		extensionManager.StopAll()
	}

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
	log.Println("showAbout() called - generating HTML...")
	version := getVersionInfo()

	// Create HTML with cyan background and white text
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body {
            background-color: #00FFFF;
            color: white;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
            padding: 40px;
            margin: 0;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
        }
        .container {
            text-align: center;
            background-color: #00CED1;
            padding: 30px;
            border-radius: 15px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.3);
        }
        h1 {
            font-size: 32px;
            margin-bottom: 20px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        p {
            font-size: 16px;
            line-height: 1.6;
            margin: 10px 0;
        }
        .version {
            background-color: rgba(255,255,255,0.1);
            padding: 15px;
            border-radius: 8px;
            margin: 20px 0;
            font-family: monospace;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Family VPN Manager</h1>
        <p><strong>CYAN EDITION - PROVES BINARY REBUILD!</strong></p>
        <p>Secure, encrypted VPN built from scratch</p>
        <p>with AES-256-GCM encryption.</p>
        <p><strong>Server:</strong> Helsinki, Finland</p>
        <p><strong>Encryption:</strong> AES-256-GCM</p>
        <div class="version">%s</div>
        <p>Made with love for the family. ❤️</p>
        <p>💕 2025</p>
    </div>
</body>
</html>`, version)

	// Write HTML to temp file (don't delete it - let OS clean up /tmp)
	tmpfile, err := os.CreateTemp("", "about-*.html")
	if err != nil {
		log.Printf("Failed to create temp file: %v", err)
		return
	}
	// Note: Not removing file so browser has time to load it

	if _, err := tmpfile.WriteString(html); err != nil {
		log.Printf("Failed to write HTML: %v", err)
		return
	}
	tmpfile.Close()

	// Open in default browser
	log.Printf("Opening About page: %s", tmpfile.Name())
	cmd := exec.Command("open", tmpfile.Name())
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	} else {
		log.Println("Browser opened successfully")
	}
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

// peerListUpdater periodically reads the peer list file
func peerListUpdater() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !vpnState.Connected {
			// Clear peers when disconnected
			if len(connectedPeers) > 0 {
				connectedPeers = nil
				updatePeerMenu()
			}
			continue
		}

		// Read peer list from file
		homeDir, err := os.UserHomeDir()
		if err != nil {
			continue
		}

		peerFile := filepath.Join(homeDir, ".family-vpn-peers.json")
		data, err := os.ReadFile(peerFile)
		if err != nil {
			// File might not exist yet
			continue
		}

		var peers []*PeerInfo
		if err := json.Unmarshal(data, &peers); err != nil {
			log.Printf("Failed to parse peer list: %v", err)
			continue
		}

		// Update if changed
		if !peersEqual(connectedPeers, peers) {
			connectedPeers = peers
			updatePeerMenu()
		}
	}
}

// peersEqual checks if two peer lists are equal
func peersEqual(a, b []*PeerInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].VPNAddress != b[i].VPNAddress || a[i].Hostname != b[i].Hostname {
			return false
		}
	}
	return true
}

// updatePeerMenu updates the peer menu items
func updatePeerMenu() {
	// Remove all existing peer menu items
	for _, item := range peerMenuItems {
		item.Hide()
	}
	peerMenuItems = make(map[string]*systray.MenuItem)

	// Add new peer menu items
	for _, peer := range connectedPeers {
		// Create menu item: "🖥️  MacBook-Air (10.8.0.2)"
		label := fmt.Sprintf("🖥️  %s (%s)", peer.Hostname, peer.VPNAddress)

		item := systray.AddMenuItem(label, "Connected device")
		peerMenuItems[peer.VPNAddress] = item

		// Add submenu items for this peer
		videoCallItem := item.AddSubMenuItem("📹 Video Call", "Start video call")
		screenShareItem := item.AddSubMenuItem("🖥️ Screen Sharing", "Remote desktop access")
		sshTerminalItem := item.AddSubMenuItem("💻 SSH Terminal", "Open terminal to peer")

		// Start click handlers
		go handleVideoCallClick(videoCallItem, peer)
		go handleScreenShareClick(screenShareItem, peer)
		go handleSSHTerminalClick(sshTerminalItem, peer)
	}

	log.Printf("Updated peer menu: %d peers", len(connectedPeers))
}

// handleVideoCallClick handles video call button clicks
func handleVideoCallClick(item *systray.MenuItem, peer *PeerInfo) {
	for {
		<-item.ClickedCh
		log.Printf("Starting video call with %s (%s)", peer.Hostname, peer.VPNAddress)
		startVideoCall(peer)
	}
}

// handleScreenShareClick handles screen sharing button clicks
func handleScreenShareClick(item *systray.MenuItem, peer *PeerInfo) {
	for {
		<-item.ClickedCh
		log.Printf("Opening remote access to %s (%s)", peer.Hostname, peer.VPNAddress)
		openRemoteAccess(peer.VPNAddress)
	}
}

// handleSSHTerminalClick handles SSH terminal button clicks
func handleSSHTerminalClick(item *systray.MenuItem, peer *PeerInfo) {
	for {
		<-item.ClickedCh
		log.Printf("Opening SSH terminal to %s (%s)", peer.Hostname, peer.VPNAddress)
		openSSHTerminal(peer)
	}
}

// getUsernameForPeer determines the SSH username for a peer based on hostname
func getUsernameForPeer(peer *PeerInfo) string {
	// Simple mapping based on hostname patterns
	hostname := strings.ToLower(peer.Hostname)

	// Extract username from hostname like "Miguels-MacBook-Air.local" → "miguel_lemos"
	if strings.Contains(hostname, "miguel") {
		return "miguel_lemos"
	} else if strings.Contains(hostname, "anastasiia") {
		return "anastasiia"
	}

	// Fallback to current user
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// openSSHTerminal opens an SSH terminal to the peer via SSH extension
func openSSHTerminal(peer *PeerInfo) {
	username := getUsernameForPeer(peer)

	// Trigger SSH extension on localhost:8891
	url := fmt.Sprintf("http://localhost:8891/ssh?peer=%s&name=%s&username=%s",
		peer.VPNAddress, peer.Hostname, username)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to trigger SSH extension: %v", err)
		dialog.Message("Failed to open SSH terminal\n\nMake sure SSH extension is running.\n\nError: %v", err).Title("SSH Error").Error()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("SSH extension returned error: %s", resp.Status)
		dialog.Message("SSH extension error: %s", resp.Status).Title("SSH Error").Error()
	} else {
		log.Printf("SSH terminal opened to %s", peer.Hostname)
	}
}

// openRemoteAccess opens macOS Screen Sharing to the specified IP
func openRemoteAccess(vpnIP string) {
	// Use macOS Screen Sharing URL scheme
	// vnc://username@hostname or vnc://ip-address
	url := fmt.Sprintf("vnc://%s", vpnIP)

	cmd := exec.Command("open", url)
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open Screen Sharing: %v", err)
		dialog.Message("Failed to open Screen Sharing to %s\n\nError: %v", vpnIP, err).Title("Remote Access Error").Error()
	} else {
		log.Printf("Opened Screen Sharing to %s", vpnIP)
	}
}

// startVideoCall initiates a video call with the specified peer
func startVideoCall(peer *PeerInfo) {
	log.Printf("[VIDEO] Initiating call with %s (%s)", peer.Hostname, peer.VPNAddress)

	// Video extension will handle everything via IPC
	// Just trigger it by opening the video call URL
	// The extension listens on localhost:8890 (or dynamic port)
	url := fmt.Sprintf("http://localhost:8890/?peer=%s&name=%s", peer.VPNAddress, peer.Hostname)

	// Open browser window - the video extension will handle the rest
	cmd := exec.Command("open", url)
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open video call window: %v", err)
		dialog.Message("Failed to open video call window\n\nMake sure the video extension is running.\n\nError: %v", err).Title("Video Call Error").Error()
	} else {
		log.Printf("Opened video call with %s (%s)", peer.Hostname, peer.VPNAddress)
	}
}


func onExit() {
	// Stop all extensions first
	if extensionManager != nil {
		log.Println("[EXT] Stopping all extensions...")
		extensionManager.StopAll()
	}

	// Disconnect VPN if connected
	if vpnState.Connected && vpnState.Process != nil {
		log.Println("Disconnecting VPN before exit...")
		if err := vpnState.Process.Process.Kill(); err != nil {
			log.Printf("Failed to kill VPN client: %v", err)
		}
	}
	fmt.Println("Family VPN Manager exited")
}
