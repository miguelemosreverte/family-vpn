package main

import (
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

	"github.com/miguelemosreverte/family-vpn/extensions/framework"
	"github.com/miguelemosreverte/family-vpn/ipc"
)

// SSHExtension implements SSH terminal access to peers
type SSHExtension struct {
	*framework.ExtensionBase
	vpnClient *ipc.VPNClient
	port      int
}

// NewSSHExtension creates a new SSH extension
func NewSSHExtension(vpnPort int) *SSHExtension {
	return &SSHExtension{
		ExtensionBase: framework.NewExtensionBase("ssh", "1.0.1"), // Test: component-aware deployment
		vpnClient:     ipc.NewVPNClient(vpnPort),
		port:          8891, // Different from video (8890)
	}
}

// Start starts the SSH extension
func (e *SSHExtension) Start() error {
	// Check VPN core is running
	if err := e.vpnClient.Health(); err != nil {
		return err
	}

	// Start HTTP server for triggering SSH
	mux := http.NewServeMux()
	mux.HandleFunc("/ssh", e.handleSSH)
	mux.HandleFunc("/setup-ssh-key", e.handleSetupSSHKey)
	mux.HandleFunc("/health", e.handleHealth)

	addr := fmt.Sprintf("0.0.0.0:%d", e.port)
	log.Printf("[SSH] Starting server on http://%s (accessible via VPN)", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[SSH] Server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the SSH extension
func (e *SSHExtension) Stop() error {
	return nil
}

// Health returns true if extension is healthy
func (e *SSHExtension) Health() bool {
	if e.vpnClient == nil {
		return false
	}
	return e.vpnClient.Health() == nil
}

// handleHealth returns health status
func (e *SSHExtension) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "ssh-extension",
	})
}

// handleSSH opens an SSH terminal to the specified peer
func (e *SSHExtension) handleSSH(w http.ResponseWriter, r *http.Request) {
	peerIP := r.URL.Query().Get("peer")
	peerName := r.URL.Query().Get("name")
	username := r.URL.Query().Get("username")

	if peerIP == "" {
		http.Error(w, "peer parameter required", http.StatusBadRequest)
		return
	}

	// If no username provided, use current user
	if username == "" {
		if u, err := user.Current(); err == nil {
			username = u.Username
		}
	}

	target := peerIP
	if username != "" {
		target = fmt.Sprintf("%s@%s", username, peerIP)
	}

	log.Printf("[SSH] Opening SSH terminal to %s (%s)", peerName, target)

	// Open Terminal with SSH command
	// On macOS, use osascript to open Terminal.app with SSH
	script := fmt.Sprintf(`tell application "Terminal"
		activate
		do script "ssh %s"
	end tell`, target)

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		log.Printf("[SSH] Failed to open terminal: %v", err)
		http.Error(w, fmt.Sprintf("Failed to open terminal: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "opened",
		"peer":   peerIP,
		"name":   peerName,
	})
}

// handleSetupSSHKey receives a public key and adds it to authorized_keys
func (e *SSHExtension) handleSetupSSHKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	// Read public key from request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusBadRequest)
		return
	}
	publicKey := string(body)

	// Get user's home directory
	usr, err := user.Current()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user: %v", err), http.StatusInternalServerError)
		return
	}

	// Create .ssh directory if it doesn't exist
	sshDir := filepath.Join(usr.HomeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create .ssh dir: %v", err), http.StatusInternalServerError)
		return
	}

	// Add key to authorized_keys
	authKeysPath := filepath.Join(sshDir, "authorized_keys")
	f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open authorized_keys: %v", err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(publicKey + "\n"); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write key: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[SSH] Added SSH public key to authorized_keys")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "SSH key added to authorized_keys",
	})
}

func main() {
	vpnPort := flag.Int("vpn-port", 8889, "VPN core IPC port")
	flag.Parse()

	ext := NewSSHExtension(*vpnPort)
	if err := ext.Run(ext); err != nil {
		log.Fatalf("Extension failed: %v", err)
	}
}
