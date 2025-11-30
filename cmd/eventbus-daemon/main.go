// eventbus-daemon maintains a persistent WebSocket connection to the event bus server
// and exposes a Unix socket for fast CLI communication.
//
// Architecture:
//   ┌─────────────────┐     Unix Socket      ┌──────────────────┐
//   │  eventbus-cli   │◄────────────────────►│  eventbus-daemon │
//   │  (fast queries) │    /tmp/eventbus.sock│  (persistent WS) │
//   └─────────────────┘                       └────────┬─────────┘
//                                                      │ WebSocket
//                                                      │ (always on)
//                                                      ▼
//                                              ┌──────────────┐
//                                              │    Server    │
//                                              └──────────────┘
//
// The daemon:
// 1. Maintains persistent WebSocket connection with auto-reconnect
// 2. Listens on Unix socket for CLI commands
// 3. Caches state (versions, peers) for instant CLI responses
// 4. Handles incoming events (updates.available) and executes them
// 5. Reports version to server on connect

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	socketPath       = "/tmp/family-vpn-eventbus.sock"
	pidFile          = "/tmp/family-vpn-eventbus.pid"
	logFile          = "/tmp/family-vpn-eventbus.log"
	serverURL        = "ws://10.8.0.1:9000/ws"
	reconnectDelay   = 5 * time.Second
	pingInterval     = 30 * time.Second
	pongTimeout      = 90 * time.Second
	maxEventHistory  = 10000            // Max events to keep in memory
	eventRetention   = 24 * time.Hour   // Keep events for 24 hours
)

// Daemon holds the daemon state
type Daemon struct {
	mu sync.RWMutex

	// WebSocket connection
	wsConn     *websocket.Conn
	wsConnMu   sync.Mutex
	connected  bool
	lastPong   time.Time
	reconnects int

	// Cached state from server - keyed by hostname (stable) not VPN IP (floating)
	versions      map[string]string // hostname -> git_commit
	hostnameToIP  map[string]string // hostname -> vpn_ip (for SSH routing)
	serverVersion string
	peers         []Peer
	lastSnapshot  time.Time

	// Event history (circular buffer, 24h retention)
	eventHistory []EventRecord
	eventIndex   int // Next write position in circular buffer

	// Client statistics
	clientStats map[string]*ClientStats // hostname -> stats
	startedAt   time.Time               // When daemon started

	// Local info
	hostname  string
	vpnIP     string
	gitCommit string
	repoPath  string

	// Control
	shutdown chan struct{}
	logger   *log.Logger
}

// Peer represents a connected VPN peer
type Peer struct {
	Hostname    string `json:"hostname"`
	VPNAddress  string `json:"vpn_address"`
	PublicIP    string `json:"public_ip"`
	ConnectedAt string `json:"connected_at"`
	OS          string `json:"os"`
}

// CLIRequest is sent from CLI to daemon via Unix socket
type CLIRequest struct {
	Command string                 `json:"cmd"`
	Args    map[string]interface{} `json:"args,omitempty"`
}

// CLIResponse is sent from daemon to CLI
type CLIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// Event from WebSocket
type Event struct {
	Namespace string                 `json:"ns"`
	Timestamp int64                  `json:"ts"`
	Sequence  uint64                 `json:"seq"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// EventRecord stores an event with additional metadata for history
type EventRecord struct {
	Namespace  string                 `json:"ns"`
	Timestamp  time.Time              `json:"ts"`
	Hostname   string                 `json:"hostname,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	ReceivedAt time.Time              `json:"received_at"`
}

// ClientStats tracks statistics for each client
type ClientStats struct {
	Hostname        string    `json:"hostname"`
	VPNIP           string    `json:"vpn_ip"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	Connects        int       `json:"connects"`
	Disconnects     int       `json:"disconnects"`
	Deployments     int       `json:"deployments"`
	SuccessfulDeploys int     `json:"successful_deploys"`
	FailedDeploys   int       `json:"failed_deploys"`
	CurrentVersion  string    `json:"current_version"`
	UptimeSeconds   int64     `json:"uptime_seconds"`
}

func main() {
	// Setup logging
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}
	defer f.Close()

	logger := log.New(f, "", log.Ldate|log.Ltime|log.Lmicroseconds)

	// Check for existing daemon
	if isRunning() {
		fmt.Println("Daemon already running")
		os.Exit(0)
	}

	// Write PID file
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		logger.Printf("Warning: could not write PID file: %v", err)
	}

	daemon := &Daemon{
		versions:     make(map[string]string),
		hostnameToIP: make(map[string]string),
		eventHistory: make([]EventRecord, 0, maxEventHistory),
		clientStats:  make(map[string]*ClientStats),
		startedAt:    time.Now(),
		shutdown:     make(chan struct{}),
		logger:       logger,
	}

	// Get local info
	daemon.hostname, _ = os.Hostname()
	daemon.vpnIP = getVPNIP()
	daemon.gitCommit = getGitCommit()
	daemon.repoPath = getRepoPath()

	logger.Println("╔════════════════════════════════════════════════════════════╗")
	logger.Println("║         EVENTBUS DAEMON STARTING                           ║")
	logger.Println("╚════════════════════════════════════════════════════════════╝")
	logger.Printf("[INFO] Hostname: %s", daemon.hostname)
	logger.Printf("[INFO] VPN IP: %s", daemon.vpnIP)
	logger.Printf("[INFO] Git Commit: %s", daemon.gitCommit)
	logger.Printf("[INFO] Repo Path: %s", daemon.repoPath)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Println("[SIGNAL] Shutdown signal received")
		close(daemon.shutdown)
	}()

	// Start Unix socket listener
	go daemon.startSocketListener()

	// Start WebSocket connection (with reconnect)
	go daemon.wsConnectionLoop()

	// Wait for shutdown
	<-daemon.shutdown

	// Cleanup
	daemon.cleanup()
	logger.Println("[SHUTDOWN] Complete")
}

func isRunning() bool {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}

	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	if pid <= 0 {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds - send signal 0 to check
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	return true
}

func (d *Daemon) cleanup() {
	d.wsConnMu.Lock()
	if d.wsConn != nil {
		d.wsConn.Close()
	}
	d.wsConnMu.Unlock()

	os.Remove(socketPath)
	os.Remove(pidFile)
}

// ============================================================================
// Unix Socket Listener - for CLI communication
// ============================================================================

func (d *Daemon) startSocketListener() {
	// Remove stale socket
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		d.logger.Printf("[SOCKET] Failed to listen: %v", err)
		return
	}
	defer listener.Close()

	// Make socket accessible
	os.Chmod(socketPath, 0666)

	d.logger.Printf("[SOCKET] Listening on %s", socketPath)

	for {
		select {
		case <-d.shutdown:
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		go d.handleCLIConnection(conn)
	}
}

func (d *Daemon) handleCLIConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	var req CLIRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		d.sendResponse(conn, CLIResponse{Success: false, Error: "invalid request"})
		return
	}

	resp := d.handleCommand(req)
	d.sendResponse(conn, resp)
}

func (d *Daemon) sendResponse(conn net.Conn, resp CLIResponse) {
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}

func (d *Daemon) handleCommand(req CLIRequest) CLIResponse {
	d.logger.Printf("[CLI] Command: %s", req.Command)

	switch req.Command {
	case "status":
		return d.cmdStatus()
	case "versions":
		return d.cmdVersions()
	case "peers":
		return d.cmdPeers()
	case "update":
		return d.cmdUpdate(req.Args)
	case "rollback":
		return d.cmdRollback(req.Args)
	case "broadcast":
		return d.cmdBroadcast(req.Args)
	case "logs":
		return d.cmdLogs(req.Args)
	case "stats":
		return d.cmdStats(req.Args)
	case "health":
		return d.cmdHealth(req.Args)
	default:
		return CLIResponse{Success: false, Error: "unknown command: " + req.Command}
	}
}

func (d *Daemon) cmdStatus() CLIResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := map[string]interface{}{
		"connected":      d.connected,
		"hostname":       d.hostname,
		"vpn_ip":         d.vpnIP,
		"git_commit":     d.gitCommit,
		"reconnects":     d.reconnects,
		"last_snapshot":  d.lastSnapshot.Format(time.RFC3339),
		"peer_count":     len(d.peers),
		"version_count":  len(d.versions),
		"server_version": d.serverVersion,
	}

	return CLIResponse{Success: true, Data: status}
}

func (d *Daemon) cmdVersions() CLIResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// If no cached versions, request from server
	if len(d.versions) == 0 {
		d.mu.RUnlock()
		d.requestVersions()
		d.mu.RLock()
	}

	return CLIResponse{
		Success: true,
		Data: map[string]interface{}{
			"versions":       d.versions,
			"hostname_to_ip": d.hostnameToIP,
			"server_version": d.serverVersion,
		},
	}
}

func (d *Daemon) cmdPeers() CLIResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return CLIResponse{Success: true, Data: d.peers}
}

func (d *Daemon) cmdUpdate(args map[string]interface{}) CLIResponse {
	targets, _ := args["targets"].([]interface{})
	version, _ := args["version"].(string)

	if len(targets) == 0 {
		return CLIResponse{Success: false, Error: "no targets specified"}
	}

	targetStrings := make([]string, len(targets))
	for i, t := range targets {
		targetStrings[i] = fmt.Sprintf("%v", t)
	}

	// Broadcast update event
	event := map[string]interface{}{
		"ns": "updates.available",
		"ts": time.Now().UnixMilli(),
		"data": map[string]interface{}{
			"targets": targetStrings,
			"version": version,
			"domain":  "all",
		},
	}

	if err := d.sendWSMessage(event); err != nil {
		return CLIResponse{Success: false, Error: err.Error()}
	}

	return CLIResponse{
		Success: true,
		Data:    map[string]string{"message": "Update broadcast sent"},
	}
}

func (d *Daemon) cmdRollback(args map[string]interface{}) CLIResponse {
	target, _ := args["target"].(string)
	version, _ := args["version"].(string)

	if target == "" || version == "" {
		return CLIResponse{Success: false, Error: "target and version required"}
	}

	event := map[string]interface{}{
		"ns": "updates.available",
		"ts": time.Now().UnixMilli(),
		"data": map[string]interface{}{
			"targets":  []string{target},
			"version":  version,
			"rollback": true,
			"domain":   "all",
		},
	}

	if err := d.sendWSMessage(event); err != nil {
		return CLIResponse{Success: false, Error: err.Error()}
	}

	return CLIResponse{
		Success: true,
		Data:    map[string]string{"message": "Rollback broadcast sent"},
	}
}

func (d *Daemon) cmdBroadcast(args map[string]interface{}) CLIResponse {
	ns, _ := args["ns"].(string)
	data, _ := args["data"].(map[string]interface{})

	if ns == "" {
		return CLIResponse{Success: false, Error: "namespace required"}
	}

	event := map[string]interface{}{
		"ns":   ns,
		"ts":   time.Now().UnixMilli(),
		"data": data,
	}

	if err := d.sendWSMessage(event); err != nil {
		return CLIResponse{Success: false, Error: err.Error()}
	}

	return CLIResponse{Success: true, Data: map[string]string{"message": "Broadcast sent"}}
}

// ============================================================================
// WebSocket Connection Management
// ============================================================================

func (d *Daemon) wsConnectionLoop() {
	for {
		select {
		case <-d.shutdown:
			return
		default:
		}

		if err := d.connectWebSocket(); err != nil {
			d.logger.Printf("[WS] Connection failed: %v", err)
			time.Sleep(reconnectDelay)
			continue
		}

		d.handleWebSocket()

		d.mu.Lock()
		d.connected = false
		d.reconnects++
		d.mu.Unlock()

		d.logger.Printf("[WS] Disconnected, reconnecting in %v...", reconnectDelay)
		time.Sleep(reconnectDelay)
	}
}

func (d *Daemon) connectWebSocket() error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("vpn_ip", d.vpnIP)
	q.Set("hostname", d.hostname)
	u.RawQuery = q.Encode()

	d.logger.Printf("[WS] Connecting to %s", u.String())

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	d.wsConnMu.Lock()
	d.wsConn = conn
	d.wsConnMu.Unlock()

	d.mu.Lock()
	d.connected = true
	d.lastPong = time.Now()
	d.mu.Unlock()

	d.logger.Println("[WS] Connected!")

	// Subscribe to all events
	d.sendWSMessage(map[string]interface{}{
		"type":       "subscribe",
		"namespaces": []string{"*"},
	})

	// Report our version
	d.sendWSMessage(map[string]interface{}{
		"ns": "versions.client",
		"ts": time.Now().UnixMilli(),
		"data": map[string]interface{}{
			"hostname": d.hostname,
			"vpn_ip":   d.vpnIP,
			"version":  d.gitCommit,
		},
	})

	// Request current versions
	d.requestVersions()

	return nil
}

func (d *Daemon) handleWebSocket() {
	// Start ping ticker
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	// Read messages in goroutine
	msgChan := make(chan []byte, 100)
	errChan := make(chan error, 1)

	go func() {
		for {
			_, msg, err := d.wsConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			msgChan <- msg
		}
	}()

	for {
		select {
		case <-d.shutdown:
			return

		case <-pingTicker.C:
			if err := d.sendWSMessage(map[string]string{"type": "ping"}); err != nil {
				d.logger.Printf("[WS] Ping failed: %v", err)
				return
			}

		case msg := <-msgChan:
			d.handleWSMessage(msg)

		case err := <-errChan:
			d.logger.Printf("[WS] Read error: %v", err)
			return
		}
	}
}

func (d *Daemon) sendWSMessage(msg interface{}) error {
	d.wsConnMu.Lock()
	defer d.wsConnMu.Unlock()

	if d.wsConn == nil {
		return fmt.Errorf("not connected")
	}

	return d.wsConn.WriteJSON(msg)
}

func (d *Daemon) requestVersions() {
	d.sendWSMessage(map[string]interface{}{
		"type": "get_client_versions",
	})
}

func (d *Daemon) handleWSMessage(data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	// Handle pong
	if msgType, _ := msg["type"].(string); msgType == "pong" {
		d.mu.Lock()
		d.lastPong = time.Now()
		d.mu.Unlock()
		return
	}

	// Handle versions response
	if versions, ok := msg["versions"].(map[string]interface{}); ok {
		d.mu.Lock()
		for ip, ver := range versions {
			if verStr, ok := ver.(string); ok {
				d.versions[ip] = verStr
			}
		}
		if sv, ok := msg["server_version"].(string); ok {
			d.serverVersion = sv
		}
		d.mu.Unlock()
		return
	}

	// Handle events (have ns field)
	if ns, ok := msg["ns"].(string); ok {
		d.handleEvent(ns, msg)
	}
}

func (d *Daemon) handleEvent(ns string, msg map[string]interface{}) {
	d.logger.Printf("[EVENT] %s", ns)

	// Extract hostname from event data for recording
	hostname := ""
	if data, ok := msg["data"].(map[string]interface{}); ok {
		hostname, _ = data["hostname"].(string)
	}

	// Record the event in history
	if data, ok := msg["data"].(map[string]interface{}); ok {
		d.recordEvent(ns, hostname, data)
	} else {
		d.recordEvent(ns, hostname, nil)
	}

	switch {
	case ns == "updates.available":
		d.handleUpdateEvent(msg)

	case strings.HasPrefix(ns, "versions."):
		d.handleVersionEvent(msg)

	case ns == "peers.joined":
		d.handlePeerJoined(msg)
		if hostname != "" {
			d.recordConnect(hostname)
		}

	case ns == "peers.left":
		d.handlePeerLeft(msg)
		if hostname != "" {
			d.recordDisconnect(hostname)
		}

	case ns == "system.snapshot":
		d.handleSnapshot(msg)
	}
}

func (d *Daemon) handleUpdateEvent(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}

	targets, _ := data["targets"].([]interface{})
	version, _ := data["version"].(string)
	isRollback, _ := data["rollback"].(bool)

	// Check if we're targeted
	targeted := false
	for _, t := range targets {
		target := fmt.Sprintf("%v", t)
		if target == "all" || target == d.vpnIP || target == d.hostname {
			targeted = true
			break
		}
	}

	if !targeted {
		d.logger.Printf("[UPDATE] Not targeted, ignoring")
		return
	}

	d.logger.Printf("[UPDATE] Targeted! version=%s rollback=%v", version, isRollback)

	// Execute update
	go d.executeUpdate(version, isRollback)
}

func (d *Daemon) executeUpdate(version string, isRollback bool) {
	d.logger.Printf("[UPDATE] Executing update to %s (rollback=%v)", version, isRollback)

	if d.repoPath == "" {
		d.logger.Printf("[UPDATE] No repo path configured")
		return
	}

	// Git fetch and checkout
	var cmd *exec.Cmd
	if version == "" {
		// Pull latest
		cmd = exec.Command("git", "-C", d.repoPath, "pull", "origin", "main")
	} else {
		// Checkout specific version
		cmd = exec.Command("bash", "-c", fmt.Sprintf(
			"cd %s && git fetch origin && git checkout %s",
			d.repoPath, version,
		))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Printf("[UPDATE] Git failed: %v - %s", err, string(output))
		return
	}
	d.logger.Printf("[UPDATE] Git: %s", string(output))

	// Rebuild
	rebuildCmd := exec.Command("bash", "-c", fmt.Sprintf(
		"cd %s && ./build.sh 2>&1 || make build 2>&1",
		d.repoPath,
	))
	output, err = rebuildCmd.CombinedOutput()
	if err != nil {
		d.logger.Printf("[UPDATE] Build failed: %v - %s", err, string(output))
	} else {
		d.logger.Printf("[UPDATE] Build: %s", string(output))
	}

	// Update our git commit
	d.gitCommit = getGitCommit()
	d.logger.Printf("[UPDATE] New commit: %s", d.gitCommit)

	// Report new version
	d.sendWSMessage(map[string]interface{}{
		"ns": "versions.client",
		"ts": time.Now().UnixMilli(),
		"data": map[string]interface{}{
			"hostname": d.hostname,
			"vpn_ip":   d.vpnIP,
			"version":  d.gitCommit,
		},
	})
}

func (d *Daemon) handleVersionEvent(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}

	hostname, _ := data["hostname"].(string)
	vpnIP, _ := data["vpn_ip"].(string)
	version, _ := data["version"].(string)

	// Use hostname as primary key (stable), fall back to vpn_ip only if no hostname
	if hostname != "" && version != "" {
		d.mu.Lock()
		d.versions[hostname] = version
		if vpnIP != "" {
			d.hostnameToIP[hostname] = vpnIP
		}
		d.mu.Unlock()

		// Update client stats
		d.updateClientSeen(hostname, vpnIP, version)
	} else if vpnIP != "" && version != "" {
		// Legacy: server sent only vpn_ip, store with vpn_ip as key
		d.mu.Lock()
		d.versions[vpnIP] = version
		d.mu.Unlock()
	}
}

func (d *Daemon) handlePeerJoined(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}

	peer := Peer{
		Hostname:    getString(data, "hostname"),
		VPNAddress:  getString(data, "vpn_address"),
		PublicIP:    getString(data, "public_ip"),
		OS:          getString(data, "os"),
		ConnectedAt: time.Now().Format(time.RFC3339),
	}

	d.mu.Lock()
	// Add if not exists
	exists := false
	for _, p := range d.peers {
		if p.VPNAddress == peer.VPNAddress {
			exists = true
			break
		}
	}
	if !exists {
		d.peers = append(d.peers, peer)
	}
	d.mu.Unlock()
}

func (d *Daemon) handlePeerLeft(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}

	vpnIP := getString(data, "vpn_address")
	if vpnIP == "" {
		return
	}

	d.mu.Lock()
	newPeers := make([]Peer, 0, len(d.peers))
	for _, p := range d.peers {
		if p.VPNAddress != vpnIP {
			newPeers = append(newPeers, p)
		}
	}
	d.peers = newPeers
	d.mu.Unlock()
}

func (d *Daemon) handleSnapshot(msg map[string]interface{}) {
	state, ok := msg["state"].(map[string]interface{})
	if !ok {
		return
	}

	d.mu.Lock()
	d.lastSnapshot = time.Now()

	// Update versions from snapshot
	if versions, ok := state["versions"].(map[string]interface{}); ok {
		for ip, ver := range versions {
			if verStr, ok := ver.(string); ok {
				d.versions[ip] = verStr
			}
		}
	}

	// Update peers from snapshot
	if peers, ok := state["peers"].([]interface{}); ok {
		d.peers = d.peers[:0]
		for _, p := range peers {
			if peerMap, ok := p.(map[string]interface{}); ok {
				d.peers = append(d.peers, Peer{
					Hostname:    getString(peerMap, "hostname"),
					VPNAddress:  getString(peerMap, "vpn_address"),
					PublicIP:    getString(peerMap, "public_ip"),
					OS:          getString(peerMap, "os"),
					ConnectedAt: getString(peerMap, "connected_at"),
				})
			}
		}
	}

	d.mu.Unlock()
}

// ============================================================================
// Event History & Stats Functions
// ============================================================================

// recordEvent stores an event in the history buffer
func (d *Daemon) recordEvent(ns string, hostname string, data map[string]interface{}) {
	now := time.Now()
	record := EventRecord{
		Namespace:  ns,
		Timestamp:  now,
		Hostname:   hostname,
		Data:       data,
		ReceivedAt: now,
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Append to history (circular buffer behavior)
	if len(d.eventHistory) < maxEventHistory {
		d.eventHistory = append(d.eventHistory, record)
	} else {
		d.eventHistory[d.eventIndex] = record
		d.eventIndex = (d.eventIndex + 1) % maxEventHistory
	}
}

// pruneOldEvents removes events older than retention period
func (d *Daemon) pruneOldEvents() {
	cutoff := time.Now().Add(-eventRetention)

	d.mu.Lock()
	defer d.mu.Unlock()

	newHistory := make([]EventRecord, 0, len(d.eventHistory))
	for _, e := range d.eventHistory {
		if e.ReceivedAt.After(cutoff) {
			newHistory = append(newHistory, e)
		}
	}
	d.eventHistory = newHistory
	d.eventIndex = 0
}

// getOrCreateStats gets or creates stats for a hostname
func (d *Daemon) getOrCreateStats(hostname string) *ClientStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	if stats, ok := d.clientStats[hostname]; ok {
		return stats
	}

	stats := &ClientStats{
		Hostname:  hostname,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
	d.clientStats[hostname] = stats
	return stats
}

// updateClientSeen updates last seen time for a client
func (d *Daemon) updateClientSeen(hostname, vpnIP, version string) {
	stats := d.getOrCreateStats(hostname)

	d.mu.Lock()
	defer d.mu.Unlock()

	stats.LastSeen = time.Now()
	stats.VPNIP = vpnIP
	stats.CurrentVersion = version
}

// recordConnect records a client connection
func (d *Daemon) recordConnect(hostname string) {
	stats := d.getOrCreateStats(hostname)

	d.mu.Lock()
	stats.Connects++
	stats.LastSeen = time.Now()
	d.mu.Unlock()

	d.recordEvent("stats.connect", hostname, map[string]interface{}{
		"hostname": hostname,
	})
}

// recordDisconnect records a client disconnection
func (d *Daemon) recordDisconnect(hostname string) {
	stats := d.getOrCreateStats(hostname)

	d.mu.Lock()
	stats.Disconnects++
	d.mu.Unlock()

	d.recordEvent("stats.disconnect", hostname, map[string]interface{}{
		"hostname": hostname,
	})
}

// recordDeployment records a deployment attempt
func (d *Daemon) recordDeployment(hostname string, success bool, version string) {
	stats := d.getOrCreateStats(hostname)

	d.mu.Lock()
	stats.Deployments++
	if success {
		stats.SuccessfulDeploys++
		stats.CurrentVersion = version
	} else {
		stats.FailedDeploys++
	}
	d.mu.Unlock()

	d.recordEvent("stats.deployment", hostname, map[string]interface{}{
		"hostname": hostname,
		"success":  success,
		"version":  version,
	})
}

// getEventsByFilter returns events matching the filter criteria
func (d *Daemon) getEventsByFilter(hostname string, since time.Time, until time.Time, namespace string, limit int) []EventRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []EventRecord
	for _, e := range d.eventHistory {
		// Apply filters
		if hostname != "" && e.Hostname != hostname {
			continue
		}
		if !since.IsZero() && e.ReceivedAt.Before(since) {
			continue
		}
		if !until.IsZero() && e.ReceivedAt.After(until) {
			continue
		}
		if namespace != "" && !strings.HasPrefix(e.Namespace, namespace) {
			continue
		}

		results = append(results, e)

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results
}

// ============================================================================
// CLI Command Handlers for Observability
// ============================================================================

func (d *Daemon) cmdLogs(args map[string]interface{}) CLIResponse {
	hostname, _ := args["hostname"].(string)
	namespace, _ := args["namespace"].(string)
	limitFloat, _ := args["limit"].(float64)
	limit := int(limitFloat)
	if limit == 0 {
		limit = 100 // Default limit
	}

	// Parse time filters
	var since, until time.Time
	if sinceStr, ok := args["since"].(string); ok && sinceStr != "" {
		since = parseTimeFilter(sinceStr)
	}
	if untilStr, ok := args["until"].(string); ok && untilStr != "" {
		until = parseTimeFilter(untilStr)
	}
	if lastStr, ok := args["last"].(string); ok && lastStr != "" {
		since = parseLastDuration(lastStr)
	}

	events := d.getEventsByFilter(hostname, since, until, namespace, limit)

	return CLIResponse{
		Success: true,
		Data: map[string]interface{}{
			"events": events,
			"count":  len(events),
			"filter": map[string]interface{}{
				"hostname":  hostname,
				"namespace": namespace,
				"since":     since.Format(time.RFC3339),
				"until":     until.Format(time.RFC3339),
				"limit":     limit,
			},
		},
	}
}

func (d *Daemon) cmdStats(args map[string]interface{}) CLIResponse {
	hostname, _ := args["hostname"].(string)

	d.mu.RLock()
	defer d.mu.RUnlock()

	// Calculate daemon uptime
	daemonUptime := time.Since(d.startedAt).Seconds()

	if hostname != "" {
		// Return stats for specific client
		if stats, ok := d.clientStats[hostname]; ok {
			stats.UptimeSeconds = int64(time.Since(stats.FirstSeen).Seconds())
			return CLIResponse{
				Success: true,
				Data: map[string]interface{}{
					"client":        stats,
					"daemon_uptime": daemonUptime,
				},
			}
		}
		return CLIResponse{Success: false, Error: "client not found: " + hostname}
	}

	// Return all client stats
	allStats := make(map[string]*ClientStats)
	for h, s := range d.clientStats {
		s.UptimeSeconds = int64(time.Since(s.FirstSeen).Seconds())
		allStats[h] = s
	}

	return CLIResponse{
		Success: true,
		Data: map[string]interface{}{
			"clients":        allStats,
			"client_count":   len(allStats),
			"daemon_uptime":  daemonUptime,
			"daemon_started": d.startedAt.Format(time.RFC3339),
			"reconnects":     d.reconnects,
			"event_count":    len(d.eventHistory),
		},
	}
}

func (d *Daemon) cmdHealth(args map[string]interface{}) CLIResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Calculate health metrics
	now := time.Now()
	daemonUptime := now.Sub(d.startedAt)

	// Count recent events (last hour)
	recentCutoff := now.Add(-1 * time.Hour)
	recentEvents := 0
	for _, e := range d.eventHistory {
		if e.ReceivedAt.After(recentCutoff) {
			recentEvents++
		}
	}

	// Get client health summary
	clientHealth := make(map[string]string)
	for hostname, stats := range d.clientStats {
		timeSinceLastSeen := now.Sub(stats.LastSeen)
		if timeSinceLastSeen < 2*time.Minute {
			clientHealth[hostname] = "healthy"
		} else if timeSinceLastSeen < 10*time.Minute {
			clientHealth[hostname] = "stale"
		} else {
			clientHealth[hostname] = "offline"
		}
	}

	return CLIResponse{
		Success: true,
		Data: map[string]interface{}{
			"daemon": map[string]interface{}{
				"connected":     d.connected,
				"uptime":        daemonUptime.String(),
				"uptime_secs":   daemonUptime.Seconds(),
				"reconnects":    d.reconnects,
				"last_pong":     d.lastPong.Format(time.RFC3339),
				"event_count":   len(d.eventHistory),
				"recent_events": recentEvents,
			},
			"clients": map[string]interface{}{
				"total":   len(d.clientStats),
				"health":  clientHealth,
				"versions": d.versions,
			},
			"timestamp": now.Format(time.RFC3339),
		},
	}
}

// parseTimeFilter parses various time formats
func parseTimeFilter(s string) time.Time {
	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// Try simple date
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	// Try datetime without timezone
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	return time.Time{}
}

// parseLastDuration parses "last X hours/minutes" style duration
func parseLastDuration(s string) time.Time {
	s = strings.ToLower(strings.TrimSpace(s))

	// Parse formats like "2h", "30m", "1d", "24h"
	var duration time.Duration
	var value int
	var unit string

	fmt.Sscanf(s, "%d%s", &value, &unit)
	if value == 0 {
		return time.Time{}
	}

	switch unit {
	case "s", "sec", "second", "seconds":
		duration = time.Duration(value) * time.Second
	case "m", "min", "minute", "minutes":
		duration = time.Duration(value) * time.Minute
	case "h", "hr", "hour", "hours":
		duration = time.Duration(value) * time.Hour
	case "d", "day", "days":
		duration = time.Duration(value) * 24 * time.Hour
	default:
		// Try parsing as Go duration
		if d, err := time.ParseDuration(s); err == nil {
			duration = d
		} else {
			return time.Time{}
		}
	}

	return time.Now().Add(-duration)
}

// ============================================================================
// Helper Functions
// ============================================================================

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getVPNIP() string {
	// Try to read from peers file first
	homeDir, _ := os.UserHomeDir()
	peersFile := filepath.Join(homeDir, ".family-vpn-peers.json")
	data, err := os.ReadFile(peersFile)
	if err == nil {
		var peers []map[string]interface{}
		if json.Unmarshal(data, &peers) == nil {
			hostname, _ := os.Hostname()
			for _, p := range peers {
				if h, _ := p["hostname"].(string); h == hostname {
					if ip, _ := p["vpn_address"].(string); ip != "" {
						return ip
					}
				}
			}
		}
	}

	// Try ifconfig for tun/utun interfaces
	cmd := exec.Command("bash", "-c",
		"ifconfig | grep -A1 'utun\\|tun0' | grep 'inet 10.8' | awk '{print $2}' | head -1")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(string(output))
	}

	return "unknown"
}

func getGitCommit() string {
	repoPath := getRepoPath()
	if repoPath == "" {
		return "unknown"
	}

	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(string(output))
}

func getRepoPath() string {
	// Check common locations
	paths := []string{
		os.Getenv("HOME") + "/Desktop/family-vpn",
		"/Users/" + os.Getenv("USER") + "/Desktop/family-vpn",
		"/root/family-vpn",
	}

	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p
		}
	}

	return ""
}
