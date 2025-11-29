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
	"strings"
	"syscall"
	"time"
)

// Watchdog is the central orchestrator for Family VPN
// It manages all components and ensures system health
type Watchdog struct {
	menuBarPID    int
	desktopPID    int
	vpnClientPID  int
	lastHealthy   time.Time
	internetOK    bool
	vpnConnected  bool
}

var (
	watchdog *Watchdog
	logFile  *os.File
)

func main() {
	// Setup logging
	var err error
	logFile, err = os.OpenFile("/tmp/family-vpn-watchdog.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("=== Family VPN Watchdog Starting ===")

	watchdog = &Watchdog{
		lastHealthy: time.Now(),
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutdown signal received, cleaning up...")
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

// cleanup removes stale locks and zombie processes
func (w *Watchdog) cleanup() {
	log.Println("Performing cleanup...")

	// Kill any zombie or stuck Family VPN processes
	exec.Command("pkill", "-9", "-f", "Family VPN").Run()
	time.Sleep(500 * time.Millisecond)

	// Remove Electron singleton locks
	homeDir, _ := os.UserHomeDir()
	singletonDir := filepath.Join(homeDir, "Library", "Application Support", "Family VPN")

	files := []string{"SingletonLock", "SingletonSocket", "SingletonCookie"}
	for _, f := range files {
		path := filepath.Join(singletonDir, f)
		if err := os.Remove(path); err == nil {
			log.Printf("Removed stale lock: %s", f)
		}
	}

	// Clean GPU cache that might be corrupted
	gpuCache := filepath.Join(singletonDir, "GPUCache")
	os.RemoveAll(gpuCache)

	log.Println("Cleanup complete")
}

// startComponents launches menu bar, desktop app, and ensures VPN connectivity
func (w *Watchdog) startComponents() {
	log.Println("Starting components...")

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
		log.Println("Menu bar already running")
		return
	}

	log.Println("Starting menu bar app...")
	cmd := exec.Command("/usr/local/bin/family-vpn-menubar")
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start menu bar: %v", err)
		return
	}

	w.menuBarPID = cmd.Process.Pid
	log.Printf("Menu bar started with PID %d", w.menuBarPID)
}

// startDesktopApp launches the desktop dashboard
func (w *Watchdog) startDesktopApp() {
	appPath := "/Applications/Family VPN.app"
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		log.Println("Desktop app not installed, skipping")
		return
	}

	// Check if already running (properly, not zombie)
	if w.isProcessRunning("Family VPN.app/Contents/MacOS/Family VPN") {
		log.Println("Desktop app already running")
		return
	}

	log.Println("Starting desktop app...")

	// Use 'open' command which handles GUI session properly
	cmd := exec.Command("open", "-a", "Family VPN")
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to start desktop app via 'open': %v", err)
		// Fallback: try direct execution
		directCmd := exec.Command("/Applications/Family VPN.app/Contents/MacOS/Family VPN")
		if err := directCmd.Start(); err != nil {
			log.Printf("Failed to start desktop app directly: %v", err)
		} else {
			w.desktopPID = directCmd.Process.Pid
			log.Printf("Desktop app started directly with PID %d", w.desktopPID)
		}
	} else {
		log.Println("Desktop app started via 'open' command")
	}
}

// isProcessRunning checks if a process is actually running (not zombie)
func (w *Watchdog) isProcessRunning(pattern string) bool {
	// Use ps to get process state
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
	// Try multiple endpoints
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

	// Also try ping as fallback
	cmd := exec.Command("ping", "-c", "1", "-W", "2", "8.8.8.8")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// restoreInternet attempts to restore internet connectivity
func (w *Watchdog) restoreInternet() {
	log.Println("Attempting to restore internet connectivity...")

	// Method 1: Wi-Fi restart (no sudo needed)
	wifiInterface := w.getWiFiInterface()
	if wifiInterface != "" {
		log.Printf("Restarting Wi-Fi interface: %s", wifiInterface)
		exec.Command("networksetup", "-setairportpower", wifiInterface, "off").Run()
		time.Sleep(2 * time.Second)
		exec.Command("networksetup", "-setairportpower", wifiInterface, "on").Run()
		time.Sleep(5 * time.Second)

		if w.checkInternet() {
			log.Println("Internet restored via Wi-Fi restart")
			return
		}
	}

	// Method 2: Try the routing fix script
	log.Println("Trying routing fix script...")
	cmd := exec.Command("/usr/local/bin/family-vpn-fix-routing")
	cmd.Run()
	time.Sleep(3 * time.Second)

	if w.checkInternet() {
		log.Println("Internet restored via routing fix")
		return
	}

	log.Println("Failed to restore internet - may need manual intervention")
}

// getWiFiInterface returns the Wi-Fi interface name
func (w *Watchdog) getWiFiInterface() string {
	cmd := exec.Command("bash", "-c",
		"networksetup -listallhardwareports | awk '/Wi-Fi/{getline; print $2}'")
	output, err := cmd.Output()
	if err != nil {
		return "en0" // Default fallback
	}
	return strings.TrimSpace(string(output))
}

// monitorLoop is the main health monitoring loop
func (w *Watchdog) monitorLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		w.healthCheck()
	}
}

// healthCheck performs all health checks and recovery actions
func (w *Watchdog) healthCheck() {
	// Check internet connectivity
	internetOK := w.checkInternet()

	if !internetOK && w.internetOK {
		// Internet just went down
		log.Println("WARNING: Internet connectivity lost!")
		w.restoreInternet()
	}
	w.internetOK = internetOK

	if internetOK {
		w.lastHealthy = time.Now()
	}

	// Check if menu bar is running
	if !w.isProcessRunning("family-vpn-menubar") {
		log.Println("Menu bar not running, restarting...")
		w.startMenuBar()
	}

	// Check if desktop app is running (only if it was installed)
	appPath := "/Applications/Family VPN.app"
	if _, err := os.Stat(appPath); err == nil {
		if !w.isProcessRunning("Family VPN.app/Contents/MacOS/Family VPN") {
			log.Println("Desktop app not running, checking for stale locks...")
			// Clean locks before restart
			w.cleanDesktopLocks()
			time.Sleep(1 * time.Second)
			w.startDesktopApp()
		}
	}

	// Log status periodically
	log.Printf("Health: Internet=%v, MenuBar=%v, Desktop=%v",
		internetOK,
		w.isProcessRunning("family-vpn-menubar"),
		w.isProcessRunning("Family VPN.app"))
}

// cleanDesktopLocks removes Electron singleton locks
func (w *Watchdog) cleanDesktopLocks() {
	homeDir, _ := os.UserHomeDir()
	singletonDir := filepath.Join(homeDir, "Library", "Application Support", "Family VPN")

	files := []string{"SingletonLock", "SingletonSocket", "SingletonCookie"}
	for _, f := range files {
		path := filepath.Join(singletonDir, f)
		os.Remove(path)
	}
	log.Println("Cleaned desktop app locks")
}

// shutdown gracefully shuts down all components
func (w *Watchdog) shutdown() {
	log.Println("Shutting down components...")

	// Kill menu bar
	if w.menuBarPID > 0 {
		syscall.Kill(w.menuBarPID, syscall.SIGTERM)
	}
	exec.Command("pkill", "-f", "family-vpn-menubar").Run()

	// Kill desktop app
	exec.Command("pkill", "-f", "Family VPN").Run()

	// Restore routing
	w.restoreInternet()

	log.Println("Shutdown complete")
}

// loadEnv loads environment variables from .env file
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
		log.Printf("Loaded environment from %s", path)
		return
	}
}
