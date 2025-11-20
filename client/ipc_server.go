package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// IPCServer provides HTTP API for extensions
type IPCServer struct {
	port          int
	client        *VPNClient
	signalQueue   map[string][]map[string]interface{} // extension -> signals
	queueMutex    sync.RWMutex
}

// NewIPCServer creates a new IPC server
func NewIPCServer(port int, client *VPNClient) *IPCServer {
	return &IPCServer{
		port:        port,
		client:      client,
		signalQueue: make(map[string][]map[string]interface{}),
	}
}

// Start starts the IPC HTTP server
func (s *IPCServer) Start() error {
	mux := http.NewServeMux()

	// Extension API endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/peers", s.handleGetPeers)
	mux.HandleFunc("/signal/send", s.handleSendSignal)
	mux.HandleFunc("/signal/poll", s.handlePollSignals)

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	log.Printf("[IPC] Starting server on http://%s", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[IPC] Server error: %v", err)
		}
	}()

	return nil
}

// handleHealth returns health status
func (s *IPCServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"enabled": s.client.enabled,
	})
}

// handleGetPeers returns list of connected peers
func (s *IPCServer) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	s.client.peersMutex.RLock()
	peers := s.client.peers
	s.client.peersMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// handleSendSignal sends a signal to a peer via VPN
func (s *IPCServer) handleSendSignal(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Peer string `json:"peer"`
		Data string `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Write to outgoing signal file (same mechanism as before)
	homeDir, err := getRealUserHomeDir()
	if err != nil {
		http.Error(w, "Failed to get home dir", http.StatusInternalServerError)
		return
	}

	signalFile := filepath.Join(homeDir, fmt.Sprintf(".family-vpn-video-out-%s", payload.Peer))
	message := fmt.Sprintf("%s:%s", payload.Peer, payload.Data)

	if err := os.WriteFile(signalFile, []byte(message), 0644); err != nil {
		http.Error(w, "Failed to write signal", http.StatusInternalServerError)
		return
	}

	log.Printf("[IPC] Signal sent to peer %s", payload.Peer)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

// handlePollSignals polls for incoming signals for an extension
func (s *IPCServer) handlePollSignals(w http.ResponseWriter, r *http.Request) {
	extension := r.URL.Query().Get("extension")
	if extension == "" {
		http.Error(w, "Extension name required", http.StatusBadRequest)
		return
	}

	s.queueMutex.Lock()
	signals := s.signalQueue[extension]
	s.signalQueue[extension] = nil // Clear queue
	s.queueMutex.Unlock()

	if signals == nil {
		signals = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(signals)
}

// QueueSignal queues an incoming signal for an extension
func (s *IPCServer) QueueSignal(extension, peerIP string, data []byte) {
	signal := map[string]interface{}{
		"peer": peerIP,
		"data": string(data),
	}

	s.queueMutex.Lock()
	s.signalQueue[extension] = append(s.signalQueue[extension], signal)
	s.queueMutex.Unlock()

	log.Printf("[IPC] Queued signal for extension '%s' from peer %s", extension, peerIP)
}
