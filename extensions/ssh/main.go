package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"

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
		ExtensionBase: framework.NewExtensionBase("ssh", "1.0.0"),
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
	mux.HandleFunc("/health", e.handleHealth)

	addr := fmt.Sprintf("127.0.0.1:%d", e.port)
	log.Printf("[SSH] Starting server on http://%s", addr)

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

	if peerIP == "" {
		http.Error(w, "peer parameter required", http.StatusBadRequest)
		return
	}

	log.Printf("[SSH] Opening SSH terminal to %s (%s)", peerName, peerIP)

	// Open Terminal with SSH command
	// On macOS, use osascript to open Terminal.app with SSH
	script := fmt.Sprintf(`tell application "Terminal"
		activate
		do script "ssh %s"
	end tell`, peerIP)

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

func main() {
	vpnPort := flag.Int("vpn-port", 8889, "VPN core IPC port")
	flag.Parse()

	ext := NewSSHExtension(*vpnPort)
	if err := ext.Run(ext); err != nil {
		log.Fatalf("Extension failed: %v", err)
	}
}
