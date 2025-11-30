package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	pidFile        = "/tmp/family-vpn-watchdog.pid"
	lockFile       = "/tmp/family-vpn-watchdog.lock"
	maxLaunchRetry = 10 // Max desktop app launch attempts before longer backoff
)

// Watchdog is the central orchestrator for Family VPN
// It manages all components and ensures system health
type Watchdog struct {
	menuBarPID         int
	desktopPID         int
	vpnClientPID       int
	lastHealthy        time.Time
	internetOK         bool
	vpnConnected       bool
	guiSessionActive   bool
	consoleUserUID     string
	consoleUserName    string
	desktopLaunchCount int
	lastDesktopAttempt time.Time
	startTime          time.Time
}

var (
	watchdog *Watchdog
	logFile  *os.File
)

func main() {
	// Setup logging first (before singleton check so we can log)
	var err error
	logFile, err = os.OpenFile("/tmp/family-vpn-watchdog.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	// SINGLETON ENFORCEMENT: Ensure only one watchdog instance runs
	if !acquireSingleton() {
		log.Println("[SINGLETON] Another watchdog instance is already running. Exiting.")
		os.Exit(0)
	}
	defer releaseSingleton()

	log.Println("╔════════════════════════════════════════════════════════════╗")
	log.Println("║         FAMILY VPN WATCHDOG STARTING                       ║")
	log.Println("╚════════════════════════════════════════════════════════════╝")

	watchdog = &Watchdog{
		lastHealthy: time.Now(),
		startTime:   time.Now(),
	}

	// Inspect and log environment at startup
	watchdog.inspectEnvironment()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("[SIGNAL] Shutdown signal received, cleaning up...")
		watchdog.shutdown()
		os.Exit(0)
	}()

	// Initial cleanup
	watchdog.cleanup()

	// Start components
	watchdog.startComponents()

	// Main health monitoring loop
	watchdog.monitorLoop()
}

// inspectEnvironment logs detailed information about the runtime environment
func (w *Watchdog) inspectEnvironment() {
	log.Println("┌─────────────────────────────────────────────────────────────┐")
	log.Println("│ ENVIRONMENT INSPECTION                                      │")
	log.Println("└─────────────────────────────────────────────────────────────┘")

	// Process info
	log.Printf("[ENV] Watchdog PID: %d", os.Getpid())
	log.Printf("[ENV] Parent PID: %d", os.Getppid())
	log.Printf("[ENV] User ID: %d", os.Getuid())
	log.Printf("[ENV] Working Dir: %s", mustGetwd())

	// Console user (who is logged into GUI)
	w.updateConsoleUser()
	log.Printf("[ENV] Console User: %s (UID: %s)", w.consoleUserName, w.consoleUserUID)

	// Check GUI session
	w.guiSessionActive = w.checkGUISession()
	log.Printf("[ENV] GUI Session Active: %v", w.guiSessionActive)

	// Check WindowServer
	wsRunning := w.isProcessRunning("WindowServer")
	log.Printf("[ENV] WindowServer Running: %v", wsRunning)

	// Check Dock
	dockRunning := w.isProcessRunning("Dock")
	log.Printf("[ENV] Dock Running: %v", dockRunning)

	// Check loginwindow
	loginwindowRunning := w.isProcessRunning("loginwindow")
	log.Printf("[ENV] loginwindow Running: %v", loginwindowRunning)

	// Check screen lock status
	screenLocked := w.isScreenLocked()
	log.Printf("[ENV] Screen Locked: %v", screenLocked)

	// List launchd bootstrap domains available
	w.logBootstrapDomains()

	log.Println("─────────────────────────────────────────────────────────────")
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "(unknown)"
	}
	return dir
}

// updateConsoleUser gets the current console user info
func (w *Watchdog) updateConsoleUser() {
	// Get console user UID
	cmd := exec.Command("stat", "-f", "%u", "/dev/console")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[ENV] Failed to get console user UID: %v", err)
		w.consoleUserUID = ""
		w.consoleUserName = ""
		return
	}
	w.consoleUserUID = strings.TrimSpace(string(output))

	// Get username from UID
	cmd = exec.Command("id", "-un", w.consoleUserUID)
	output, err = cmd.Output()
	if err != nil {
		w.consoleUserName = "(unknown)"
	} else {
		w.consoleUserName = strings.TrimSpace(string(output))
	}
}

// checkGUISession checks if a GUI session is available
func (w *Watchdog) checkGUISession() bool {
	// Check if WindowServer is running and accessible
	cmd := exec.Command("pgrep", "-x", "WindowServer")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if console user is valid (not root/loginwindow)
	if w.consoleUserUID == "" || w.consoleUserUID == "0" {
		return false
	}

	// Verify GUI bootstrap namespace is accessible
	uid, _ := strconv.Atoi(w.consoleUserUID)
	if uid < 500 {
		return false // System user, not a real user
	}

	return true
}

// isScreenLocked checks if the screen is locked
func (w *Watchdog) isScreenLocked() bool {
	cmd := exec.Command("bash", "-c", "ioreg -n Root -d1 -a | grep -q CGSSessionScreenIsLocked")
	err := cmd.Run()
	return err == nil
}

// logBootstrapDomains logs available launchd bootstrap domains
func (w *Watchdog) logBootstrapDomains() {
	cmd := exec.Command("launchctl", "print", "user/"+w.consoleUserUID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ENV] Cannot access user bootstrap domain: %v", err)
	} else {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 5 {
			log.Printf("[ENV] User bootstrap domain accessible (first 5 lines):")
			for i := 0; i < 5 && i < len(lines); i++ {
				log.Printf("[ENV]   %s", lines[i])
			}
		}
	}
}

// cleanup removes stale locks and zombie processes
func (w *Watchdog) cleanup() {
	log.Println("[CLEANUP] Performing cleanup...")

	// Kill any zombie or stuck Family VPN processes
	cmd := exec.Command("pkill", "-9", "-f", "Family VPN")
	output, _ := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("[CLEANUP] pkill output: %s", string(output))
	}
	time.Sleep(500 * time.Millisecond)

	// Remove Electron singleton locks
	homeDir, _ := os.UserHomeDir()
	singletonDir := filepath.Join(homeDir, "Library", "Application Support", "Family VPN")

	files := []string{"SingletonLock", "SingletonSocket", "SingletonCookie"}
	for _, f := range files {
		path := filepath.Join(singletonDir, f)
		if err := os.Remove(path); err == nil {
			log.Printf("[CLEANUP] Removed stale lock: %s", f)
		}
	}

	// Clean GPU cache that might be corrupted
	gpuCache := filepath.Join(singletonDir, "GPUCache")
	if err := os.RemoveAll(gpuCache); err == nil {
		log.Printf("[CLEANUP] Cleaned GPU cache")
	}

	log.Println("[CLEANUP] Complete")
}

// startComponents launches menu bar, desktop app, and ensures VPN connectivity
func (w *Watchdog) startComponents() {
	log.Println("[START] Starting components...")

	// Start menu bar first (it manages VPN connection)
	w.startMenuBar()
	time.Sleep(2 * time.Second)

	// Start desktop app
	w.startDesktopApp()
}

// startMenuBar launches the menu bar app
func (w *Watchdog) startMenuBar() {
	// Check if already running (properly, not zombie)
	if w.isProcessRunning("family-vpn-menubar") {
		log.Println("[MENUBAR] Already running, skipping start")
		return
	}

	log.Println("[MENUBAR] Starting menu bar app...")
	cmd := exec.Command("/usr/local/bin/family-vpn-menubar")
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		log.Printf("[MENUBAR] FAILED to start: %v", err)
		return
	}

	w.menuBarPID = cmd.Process.Pid
	log.Printf("[MENUBAR] Started with PID %d", w.menuBarPID)

	// Verify it's actually running after a moment
	time.Sleep(500 * time.Millisecond)
	if w.isProcessRunning("family-vpn-menubar") {
		log.Printf("[MENUBAR] Verified running")
	} else {
		log.Printf("[MENUBAR] WARNING: Process not detected after start!")
	}
}

// startDesktopApp launches the desktop dashboard with comprehensive logging
func (w *Watchdog) startDesktopApp() {
	appPath := "/Applications/Family VPN.app"
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		log.Println("[DESKTOP] App not installed at /Applications/Family VPN.app, skipping")
		return
	}

	// Check if already running (properly, not zombie)
	if w.isProcessRunning("Family VPN.app/Contents/MacOS/Family VPN") {
		log.Println("[DESKTOP] Already running, skipping start")
		return
	}

	// Rate limit desktop launch attempts (max once per 30 seconds)
	if time.Since(w.lastDesktopAttempt) < 30*time.Second && w.desktopLaunchCount > 0 {
		log.Printf("[DESKTOP] Rate limited - last attempt was %v ago", time.Since(w.lastDesktopAttempt))
		return
	}

	w.lastDesktopAttempt = time.Now()
	w.desktopLaunchCount++

	log.Println("┌─────────────────────────────────────────────────────────────┐")
	log.Printf("│ DESKTOP APP LAUNCH ATTEMPT #%d                              │", w.desktopLaunchCount)
	log.Println("└─────────────────────────────────────────────────────────────┘")

	// Re-check GUI environment
	w.updateConsoleUser()
	w.guiSessionActive = w.checkGUISession()

	log.Printf("[DESKTOP] Console User: %s (UID: %s)", w.consoleUserName, w.consoleUserUID)
	log.Printf("[DESKTOP] GUI Session Active: %v", w.guiSessionActive)
	log.Printf("[DESKTOP] Screen Locked: %v", w.isScreenLocked())

	if !w.guiSessionActive {
		log.Println("[DESKTOP] ERROR: No active GUI session detected")
		log.Println("[DESKTOP] Desktop app requires a logged-in GUI user")
		return
	}

	if w.consoleUserUID == "" || w.consoleUserUID == "0" {
		log.Println("[DESKTOP] ERROR: No valid console user (UID=0 or empty)")
		return
	}

	// Clean locks before attempting launch
	w.cleanDesktopLocks()

	// Try multiple launch methods with detailed logging
	// Note: The order matters - put most likely to succeed first
	methods := []struct {
		name string
		fn   func() (bool, string)
	}{
		{"launchctl asuser open", w.launchViaLaunchctlAsuser},
		{"System Events open location", w.launchViaSystemEventsOpenLocation},
		{"System Events launch", w.launchViaSystemEventsLaunch},
		{"direct open command", w.launchViaDirectOpen},
		{"osascript tell Finder", w.launchViaOsascriptFinder},
		{"osascript tell application", w.launchViaOsascriptApp},
		{"sudo -u with DISPLAY", w.launchViaSudoUser},
	}

	for i, method := range methods {
		log.Printf("[DESKTOP] Method %d: %s", i+1, method.name)
		success, output := method.fn()
		if success {
			log.Printf("[DESKTOP] SUCCESS via %s", method.name)
			// Verify the app is actually running
			time.Sleep(2 * time.Second)
			if w.isProcessRunning("Family VPN.app/Contents/MacOS/Family VPN") {
				log.Printf("[DESKTOP] VERIFIED: App is running")
				w.desktopLaunchCount = 0 // Reset counter on success
				return
			} else {
				log.Printf("[DESKTOP] WARNING: Method returned success but app not detected!")
			}
		} else {
			log.Printf("[DESKTOP] FAILED: %s", output)
		}
	}

	log.Println("[DESKTOP] All launch methods exhausted")
	log.Printf("[DESKTOP] Total failed attempts: %d", w.desktopLaunchCount)
}

// launchViaLaunchctlAsuser uses launchctl asuser to launch in user context
func (w *Watchdog) launchViaLaunchctlAsuser() (bool, string) {
	cmd := exec.Command("launchctl", "asuser", w.consoleUserUID, "open", "-a", "Family VPN")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// launchViaDirectOpen tries direct open command
func (w *Watchdog) launchViaDirectOpen() (bool, string) {
	cmd := exec.Command("open", "-a", "Family VPN")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// launchViaSudoUser uses sudo -u to run as the console user with proper env
func (w *Watchdog) launchViaSudoUser() (bool, string) {
	// Read sudo password from .env
	sudoPass := w.getSudoPassword()
	if sudoPass == "" {
		return false, "No SUDO_PASSWORD in .env"
	}

	// Use sudo to run as user in their GUI context
	// The key is setting the proper environment variables
	script := fmt.Sprintf(`export HOME=/Users/%s
export USER=%s
export DISPLAY=:0
/usr/bin/open -a "Family VPN"`, w.consoleUserName, w.consoleUserName)

	cmd := exec.Command("sudo", "-u", w.consoleUserName, "bash", "-c", script)

	// Pipe the password to sudo
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return false, fmt.Sprintf("stdin pipe error: %v", err)
	}

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(sudoPass + "\n"))
	}()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// launchViaOsascriptFinder uses AppleScript via Finder
func (w *Watchdog) launchViaOsascriptFinder() (bool, string) {
	script := `tell application "Finder"
		open application file "Family VPN.app" of folder "Applications" of startup disk
	end tell`

	cmd := exec.Command("launchctl", "asuser", w.consoleUserUID, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// launchViaOsascriptApp uses AppleScript to activate the app directly
func (w *Watchdog) launchViaOsascriptApp() (bool, string) {
	script := `tell application "Family VPN" to activate`

	cmd := exec.Command("launchctl", "asuser", w.consoleUserUID, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// launchViaSystemEventsOpenLocation uses System Events to open the app via file URL
func (w *Watchdog) launchViaSystemEventsOpenLocation() (bool, string) {
	script := `open location "file:///Applications/Family%20VPN.app"`

	cmd := exec.Command("launchctl", "asuser", w.consoleUserUID, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// launchViaSystemEventsLaunch uses System Events' launch command
func (w *Watchdog) launchViaSystemEventsLaunch() (bool, string) {
	// Using do shell script inside osascript to get proper GUI context
	script := `do shell script "open -a 'Family VPN'"`

	cmd := exec.Command("launchctl", "asuser", w.consoleUserUID, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("%v - %s", err, string(output))
	}
	return true, string(output)
}

// getSudoPassword reads sudo password from .env file
func (w *Watchdog) getSudoPassword() string {
	paths := []string{
		"/usr/local/.env",
		filepath.Join(os.Getenv("HOME"), "Desktop", "family-vpn", ".env"),
		"/Users/" + w.consoleUserName + "/Desktop/family-vpn/.env",
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "SUDO_PASSWORD=") {
				return strings.TrimPrefix(line, "SUDO_PASSWORD=")
			}
		}
	}
	return ""
}

// isProcessRunning checks if a process is actually running (not zombie)
func (w *Watchdog) isProcessRunning(pattern string) bool {
	// Use ps to get process state, excluding zombies
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("ps aux | grep '%s' | grep -v grep | grep -v '?E' | grep -v '?NE'", pattern))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// checkInternet tests internet connectivity
func (w *Watchdog) checkInternet() bool {
	endpoints := []string{
		"http://clients3.google.com/generate_204",
		"http://www.apple.com/library/test/success.html",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, url := range endpoints {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return true
		}
	}

	// Ping fallback
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// isVPNClientRunning checks if the VPN client process is actually running
// Uses a more precise check than simple pgrep to avoid false positives
func (w *Watchdog) isVPNClientRunning() bool {
	// Check for the actual VPN client binary (not grep or other matches)
	cmd := exec.Command("bash", "-c",
		"ps aux | grep '[f]amily-vpn-client' | grep -v grep")
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true
	}

	// Also check for tun interface as evidence of running VPN
	tunCmd := exec.Command("bash", "-c", "ifconfig | grep -q 'utun.*inet 10.8.0'")
	if tunCmd.Run() == nil {
		return true
	}

	return false
}

// triggerVPNReconnect signals the menu bar to reconnect the VPN
func (w *Watchdog) triggerVPNReconnect() {
	log.Println("[VPN] Triggering VPN reconnection via IPC...")

	// Method 1: Signal via the IPC endpoint (if menu bar is running)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("http://localhost:8889/vpn/reconnect", "application/json", nil)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			log.Println("[VPN] Successfully triggered reconnect via IPC")
			return
		}
	}

	// Method 2: Write a reconnect signal file that menu bar watches
	homeDir, _ := os.UserHomeDir()
	signalFile := filepath.Join(homeDir, ".family-vpn-reconnect-signal")
	if err := os.WriteFile(signalFile, []byte(time.Now().Format(time.RFC3339)), 0644); err == nil {
		log.Println("[VPN] Wrote reconnect signal file")
	}

	// Method 3: Kill any zombie vpn-client processes and let menu bar restart
	exec.Command("pkill", "-9", "-f", "family-vpn-client").Run()

	log.Println("[VPN] Reconnect signal sent - waiting for menu bar to handle")
}

// restoreInternet attempts to restore internet connectivity
func (w *Watchdog) restoreInternet() {
	log.Println("[NETWORK] Attempting to restore internet connectivity...")

	// Method 1: Wi-Fi restart
	wifiInterface := w.getWiFiInterface()
	if wifiInterface != "" {
		log.Printf("[NETWORK] Restarting Wi-Fi interface: %s", wifiInterface)
		exec.Command("networksetup", "-setairportpower", wifiInterface, "off").Run()
		time.Sleep(2 * time.Second)
		exec.Command("networksetup", "-setairportpower", wifiInterface, "on").Run()
		time.Sleep(5 * time.Second)

		if w.checkInternet() {
			log.Println("[NETWORK] Internet restored via Wi-Fi restart")
			return
		}
	}

	// Method 2: Routing fix script
	log.Println("[NETWORK] Trying routing fix script...")
	cmd := exec.Command("/usr/local/bin/family-vpn-fix-routing")
	output, _ := cmd.CombinedOutput()
	log.Printf("[NETWORK] Routing fix output: %s", string(output))
	time.Sleep(3 * time.Second)

	if w.checkInternet() {
		log.Println("[NETWORK] Internet restored via routing fix")
		return
	}

	log.Println("[NETWORK] Failed to restore internet - may need manual intervention")
}

// getWiFiInterface returns the Wi-Fi interface name
func (w *Watchdog) getWiFiInterface() string {
	cmd := exec.Command("bash", "-c",
		"networksetup -listallhardwareports | awk '/Wi-Fi/{getline; print $2}'")
	output, err := cmd.Output()
	if err != nil {
		return "en0"
	}
	return strings.TrimSpace(string(output))
}

// monitorLoop is the main health monitoring loop
func (w *Watchdog) monitorLoop() {
	log.Println("[MONITOR] Starting health monitoring loop (10s interval)")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	checkCount := 0
	for range ticker.C {
		checkCount++
		w.healthCheck(checkCount)
	}
}

// healthCheck performs all health checks and recovery actions
func (w *Watchdog) healthCheck(checkNum int) {
	// Only log header every 6 checks (1 minute)
	if checkNum%6 == 1 {
		log.Printf("[HEALTH] ═══════════════════ Check #%d ═══════════════════", checkNum)
		log.Printf("[HEALTH] Uptime: %v", time.Since(w.startTime).Round(time.Second))
	}

	// SINGLETON ENFORCEMENT: Ensure no duplicate processes
	w.ensureMenuBarSingleton()
	w.ensureDesktopAppSingleton()

	// Check internet connectivity
	internetOK := w.checkInternet()

	if !internetOK && w.internetOK {
		log.Println("[HEALTH] WARNING: Internet connectivity lost!")
		w.restoreInternet()
	}
	w.internetOK = internetOK

	if internetOK {
		w.lastHealthy = time.Now()
	}

	// Check menu bar
	menuBarRunning := w.isProcessRunning("family-vpn-menubar")
	if !menuBarRunning {
		log.Println("[HEALTH] Menu bar not running, restarting...")
		w.startMenuBar()
	}

	// Check desktop app
	appPath := "/Applications/Family VPN.app"
	desktopRunning := false
	if _, err := os.Stat(appPath); err == nil {
		desktopRunning = w.isProcessRunning("Family VPN.app/Contents/MacOS/Family VPN")
		if !desktopRunning {
			// Apply exponential backoff after many failed attempts
			backoffDuration := w.calculateBackoff()
			if time.Since(w.lastDesktopAttempt) < backoffDuration {
				// Silently skip - only log occasionally
				if checkNum%6 == 1 {
					log.Printf("[HEALTH] Desktop app not running (backoff: %v remaining)",
						backoffDuration-time.Since(w.lastDesktopAttempt))
				}
			} else {
				log.Println("[HEALTH] Desktop app not running, attempting launch...")
				w.cleanDesktopLocks()
				time.Sleep(500 * time.Millisecond)
				w.startDesktopApp()
			}
		} else {
			// Reset launch count on successful detection
			if w.desktopLaunchCount > 0 {
				log.Printf("[HEALTH] Desktop app now running, resetting launch counter")
				w.desktopLaunchCount = 0
			}
		}
	}

	// Check VPN client - the most critical component!
	vpnClientRunning := w.isVPNClientRunning()
	if !vpnClientRunning && internetOK && menuBarRunning {
		log.Println("[HEALTH] VPN client NOT running! Triggering reconnect...")
		w.triggerVPNReconnect()
	}

	// Status log (compact)
	if checkNum%6 == 1 {
		log.Printf("[HEALTH] Status: Net=%v Menu=%v Desktop=%v VPN=%v LaunchAttempts=%d",
			internetOK, menuBarRunning, desktopRunning, vpnClientRunning, w.desktopLaunchCount)
	}
}

// calculateBackoff returns the appropriate wait duration based on failed attempts
func (w *Watchdog) calculateBackoff() time.Duration {
	if w.desktopLaunchCount < 3 {
		return 30 * time.Second // Normal rate limiting
	} else if w.desktopLaunchCount < 5 {
		return 1 * time.Minute
	} else if w.desktopLaunchCount < 10 {
		return 5 * time.Minute
	} else {
		return 15 * time.Minute // Max backoff after 10 failed attempts
	}
}

// cleanDesktopLocks removes Electron singleton locks
func (w *Watchdog) cleanDesktopLocks() {
	homeDir, _ := os.UserHomeDir()
	singletonDir := filepath.Join(homeDir, "Library", "Application Support", "Family VPN")

	files := []string{"SingletonLock", "SingletonSocket", "SingletonCookie"}
	removed := 0
	for _, f := range files {
		path := filepath.Join(singletonDir, f)
		if err := os.Remove(path); err == nil {
			removed++
		}
	}
	if removed > 0 {
		log.Printf("[CLEANUP] Removed %d Electron singleton locks", removed)
	}
}

// shutdown gracefully shuts down all components
func (w *Watchdog) shutdown() {
	log.Println("[SHUTDOWN] Shutting down components...")

	if w.menuBarPID > 0 {
		syscall.Kill(w.menuBarPID, syscall.SIGTERM)
	}
	exec.Command("pkill", "-f", "family-vpn-menubar").Run()
	exec.Command("pkill", "-f", "Family VPN").Run()

	w.restoreInternet()

	log.Println("[SHUTDOWN] Complete")
}

// loadEnv loads environment variables from .env file (unused but kept for reference)
func loadEnv() {
	paths := []string{
		"/usr/local/.env",
		filepath.Join(os.Getenv("HOME"), "Desktop", "family-vpn", ".env"),
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
		log.Printf("[ENV] Loaded environment from %s", path)
		return
	}
}

// ============================================================================
// SINGLETON ENFORCEMENT
// ============================================================================

// acquireSingleton ensures only one watchdog instance runs at a time
// Uses a PID file with process existence verification
func acquireSingleton() bool {
	log.Printf("[SINGLETON] Checking for existing watchdog instance...")

	// Check if PID file exists
	data, err := os.ReadFile(pidFile)
	if err == nil {
		// PID file exists, check if the process is still running
		existingPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && existingPID > 0 {
			// Check if process exists and is a watchdog
			if isWatchdogProcessRunning(existingPID) {
				log.Printf("[SINGLETON] Existing watchdog found at PID %d", existingPID)
				return false
			}
			log.Printf("[SINGLETON] Stale PID file found (PID %d not running), removing", existingPID)
		}
	}

	// Also check by process name to be extra safe
	if count := countWatchdogProcesses(); count > 1 {
		log.Printf("[SINGLETON] Found %d watchdog processes running, this is instance #%d", count, count)
		// We're a duplicate, exit
		return false
	}

	// Write our PID to the file
	myPID := os.Getpid()
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", myPID)), 0644); err != nil {
		log.Printf("[SINGLETON] WARNING: Could not write PID file: %v", err)
		// Continue anyway - we verified no other instance is running
	}

	log.Printf("[SINGLETON] Acquired singleton lock, PID=%d", myPID)
	return true
}

// releaseSingleton removes the PID file on shutdown
func releaseSingleton() {
	if err := os.Remove(pidFile); err != nil {
		log.Printf("[SINGLETON] WARNING: Could not remove PID file: %v", err)
	} else {
		log.Printf("[SINGLETON] Released singleton lock")
	}
}

// isWatchdogProcessRunning checks if a specific PID is a running watchdog process
func isWatchdogProcessRunning(pid int) bool {
	// First check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0 to check
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	// Process exists, verify it's actually a watchdog
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	comm := strings.TrimSpace(string(output))
	return strings.Contains(comm, "watchdog") || strings.Contains(comm, "family-vpn")
}

// countWatchdogProcesses returns the number of watchdog processes running
func countWatchdogProcesses() int {
	cmd := exec.Command("pgrep", "-c", "family-vpn-watchdog")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count
}

// ============================================================================
// DESKTOP APP SINGLETON ENFORCEMENT
// ============================================================================

// ensureDesktopAppSingleton kills duplicate desktop app instances
func (w *Watchdog) ensureDesktopAppSingleton() {
	cmd := exec.Command("pgrep", "-f", "Family VPN.app/Contents/MacOS/Family VPN")
	output, err := cmd.Output()
	if err != nil {
		return // No processes found
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	if len(pids) <= 1 {
		return // 0 or 1 instance is fine
	}

	log.Printf("[SINGLETON] Found %d desktop app instances, keeping only the oldest", len(pids))

	// Keep the first (oldest) PID, kill the rest
	for i := 1; i < len(pids); i++ {
		pid, err := strconv.Atoi(pids[i])
		if err != nil {
			continue
		}
		log.Printf("[SINGLETON] Killing duplicate desktop app PID %d", pid)
		syscall.Kill(pid, syscall.SIGTERM)
	}
}

// ensureMenuBarSingleton kills duplicate menu bar instances
func (w *Watchdog) ensureMenuBarSingleton() {
	cmd := exec.Command("pgrep", "-f", "family-vpn-menubar")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	if len(pids) <= 1 {
		return
	}

	log.Printf("[SINGLETON] Found %d menu bar instances, keeping only the oldest", len(pids))

	for i := 1; i < len(pids); i++ {
		pid, err := strconv.Atoi(pids[i])
		if err != nil {
			continue
		}
		log.Printf("[SINGLETON] Killing duplicate menu bar PID %d", pid)
		syscall.Kill(pid, syscall.SIGTERM)
	}
}
