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

	// Time Series Configuration - ~50MB memory budget
	// Each data point ~100 bytes, budget for ~500K points total
	tsLatencyBucketSize    = 5 * time.Minute   // Aggregate latency every 5 min
	tsThroughputBucketSize = 15 * time.Minute  // Aggregate throughput every 15 min
	tsMaxLatencyPoints     = 288 * 7           // 7 days at 5min buckets = 2016 points
	tsMaxThroughputPoints  = 96 * 7            // 7 days at 15min buckets = 672 points

	// Benchmark Configuration
	benchmarkInterval      = 1 * time.Hour     // Run benchmark every hour
	benchmarkIdleThreshold = 30 * time.Second  // Only run if no traffic for 30s
	benchmarkPayloadSizes  = "64,512,1024,4096" // Payload sizes to test (bytes)
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

	// Time Series Storage (memory-efficient bucketed data)
	latencySeries    *TimeSeries // Latency measurements
	throughputSeries *TimeSeries // Throughput measurements
	lastTrafficTime  time.Time   // Last time traffic was detected
	lastBenchmarkAt  time.Time   // Last benchmark run time
	benchmarkRunning bool        // Prevent concurrent benchmarks

	// Local info
	hostname  string
	vpnIP     string
	gitCommit string
	repoPath  string

	// Control
	shutdown chan struct{}
	logger   *log.Logger
}

// TimeSeries stores time-bucketed metrics with automatic aggregation
type TimeSeries struct {
	mu         sync.RWMutex
	bucketSize time.Duration
	maxPoints  int
	points     []TimeSeriesPoint
	name       string
}

// TimeSeriesPoint represents an aggregated data point
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"ts"`
	Min       float64   `json:"min"`
	Max       float64   `json:"max"`
	Avg       float64   `json:"avg"`
	Count     int       `json:"count"`
	Sum       float64   `json:"sum"`
	P50       float64   `json:"p50,omitempty"` // Median
	P95       float64   `json:"p95,omitempty"` // 95th percentile
	P99       float64   `json:"p99,omitempty"` // 99th percentile
}

// BenchmarkResult stores results of a latency/throughput test
type BenchmarkResult struct {
	Timestamp      time.Time              `json:"timestamp"`
	Target         string                 `json:"target"`          // "server" or peer hostname
	LatencyMs      float64                `json:"latency_ms"`      // Round-trip time
	ThroughputMbps float64                `json:"throughput_mbps"` // Megabits per second
	PacketLoss     float64                `json:"packet_loss"`     // 0.0 - 1.0
	Jitter         float64                `json:"jitter_ms"`       // Latency variation
	PayloadSize    int                    `json:"payload_size"`    // Bytes tested
	Samples        int                    `json:"samples"`         // Number of measurements
	Details        map[string]interface{} `json:"details,omitempty"`
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
		versions:         make(map[string]string),
		hostnameToIP:     make(map[string]string),
		eventHistory:     make([]EventRecord, 0, maxEventHistory),
		clientStats:      make(map[string]*ClientStats),
		latencySeries:    NewTimeSeries("latency", tsLatencyBucketSize, tsMaxLatencyPoints),
		throughputSeries: NewTimeSeries("throughput", tsThroughputBucketSize, tsMaxThroughputPoints),
		lastTrafficTime:  time.Now(),
		startedAt:        time.Now(),
		shutdown:         make(chan struct{}),
		logger:           logger,
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

	// Start benchmark scheduler (runs hourly when idle)
	go daemon.benchmarkScheduler()

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
	case "benchmark":
		return d.cmdBenchmark(req.Args)
	case "timeseries":
		return d.cmdTimeSeries(req.Args)
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

// ============================================================================
// Time Series Implementation
// ============================================================================

// NewTimeSeries creates a new time series with the given bucket size and max points
func NewTimeSeries(name string, bucketSize time.Duration, maxPoints int) *TimeSeries {
	return &TimeSeries{
		name:       name,
		bucketSize: bucketSize,
		maxPoints:  maxPoints,
		points:     make([]TimeSeriesPoint, 0, maxPoints),
	}
}

// Add adds a value to the time series, aggregating into the current bucket
func (ts *TimeSeries) Add(value float64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	bucketTime := now.Truncate(ts.bucketSize)

	// Check if we can add to the current bucket
	if len(ts.points) > 0 {
		lastIdx := len(ts.points) - 1
		if ts.points[lastIdx].Timestamp.Equal(bucketTime) {
			// Update existing bucket
			p := &ts.points[lastIdx]
			p.Count++
			p.Sum += value
			p.Avg = p.Sum / float64(p.Count)
			if value < p.Min {
				p.Min = value
			}
			if value > p.Max {
				p.Max = value
			}
			return
		}
	}

	// Create new bucket
	newPoint := TimeSeriesPoint{
		Timestamp: bucketTime,
		Min:       value,
		Max:       value,
		Avg:       value,
		Sum:       value,
		Count:     1,
	}

	// Enforce max points limit
	if len(ts.points) >= ts.maxPoints {
		// Remove oldest point
		ts.points = ts.points[1:]
	}

	ts.points = append(ts.points, newPoint)
}

// AddWithPercentiles adds a value and updates percentile calculations
func (ts *TimeSeries) AddWithPercentiles(values []float64) {
	if len(values) == 0 {
		return
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	bucketTime := now.Truncate(ts.bucketSize)

	// Calculate statistics
	var sum, min, max float64
	min = values[0]
	max = values[0]
	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	avg := sum / float64(len(values))

	// Sort for percentiles
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sortFloat64s(sorted)

	p50 := percentile(sorted, 50)
	p95 := percentile(sorted, 95)
	p99 := percentile(sorted, 99)

	newPoint := TimeSeriesPoint{
		Timestamp: bucketTime,
		Min:       min,
		Max:       max,
		Avg:       avg,
		Sum:       sum,
		Count:     len(values),
		P50:       p50,
		P95:       p95,
		P99:       p99,
	}

	// Enforce max points limit
	if len(ts.points) >= ts.maxPoints {
		ts.points = ts.points[1:]
	}

	ts.points = append(ts.points, newPoint)
}

// GetPoints returns all points in the time series
func (ts *TimeSeries) GetPoints() []TimeSeriesPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]TimeSeriesPoint, len(ts.points))
	copy(result, ts.points)
	return result
}

// GetPointsSince returns points since a given time
func (ts *TimeSeries) GetPointsSince(since time.Time) []TimeSeriesPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []TimeSeriesPoint
	for _, p := range ts.points {
		if p.Timestamp.After(since) || p.Timestamp.Equal(since) {
			result = append(result, p)
		}
	}
	return result
}

// GetLatest returns the most recent point
func (ts *TimeSeries) GetLatest() *TimeSeriesPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if len(ts.points) == 0 {
		return nil
	}
	p := ts.points[len(ts.points)-1]
	return &p
}

// MemoryUsage returns estimated memory usage in bytes
func (ts *TimeSeries) MemoryUsage() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	// Each point is roughly 100 bytes (timestamps, floats, etc.)
	return len(ts.points) * 100
}

// Helper: sort float64 slice
func sortFloat64s(a []float64) {
	for i := 0; i < len(a)-1; i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

// Helper: calculate percentile from sorted slice
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * float64(p) / 100.0)
	return sorted[idx]
}

// ============================================================================
// Benchmark System
// ============================================================================

// benchmarkScheduler runs benchmarks hourly when idle
func (d *Daemon) benchmarkScheduler() {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	d.logger.Println("[BENCHMARK] Scheduler started (runs hourly when idle)")

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.checkAndRunBenchmark()
		}
	}
}

// checkAndRunBenchmark checks if conditions are met to run a benchmark
func (d *Daemon) checkAndRunBenchmark() {
	d.mu.Lock()
	// Check if benchmark is already running
	if d.benchmarkRunning {
		d.mu.Unlock()
		return
	}

	// Check if enough time has passed since last benchmark
	if time.Since(d.lastBenchmarkAt) < benchmarkInterval {
		d.mu.Unlock()
		return
	}

	// Check if system is idle (no traffic for threshold period)
	if time.Since(d.lastTrafficTime) < benchmarkIdleThreshold {
		d.mu.Unlock()
		return
	}

	// Check if we're connected
	if !d.connected {
		d.mu.Unlock()
		return
	}

	d.benchmarkRunning = true
	d.mu.Unlock()

	// Run benchmark in background
	go func() {
		d.runBenchmark()

		d.mu.Lock()
		d.benchmarkRunning = false
		d.lastBenchmarkAt = time.Now()
		d.mu.Unlock()
	}()
}

// runBenchmark executes latency and throughput tests
func (d *Daemon) runBenchmark() {
	d.logger.Println("[BENCHMARK] Starting benchmark run...")

	results := []BenchmarkResult{}

	// Test 1: Latency to VPN server (10.8.0.1)
	latencyResult := d.measureLatency("10.8.0.1", 10) // 10 samples
	if latencyResult != nil {
		results = append(results, *latencyResult)
		d.latencySeries.Add(latencyResult.LatencyMs)
	}

	// Test 2: Throughput test (simple WebSocket echo)
	throughputResult := d.measureThroughput("10.8.0.1", 4096, 5) // 4KB payload, 5 samples
	if throughputResult != nil {
		results = append(results, *throughputResult)
		d.throughputSeries.Add(throughputResult.ThroughputMbps)
	}

	// Publish benchmark results as event
	if len(results) > 0 {
		d.sendWSMessage(map[string]interface{}{
			"ns": "benchmark.completed",
			"ts": time.Now().UnixMilli(),
			"data": map[string]interface{}{
				"hostname": d.hostname,
				"results":  results,
			},
		})

		d.recordEvent("benchmark.completed", d.hostname, map[string]interface{}{
			"result_count": len(results),
		})
	}

	d.logger.Printf("[BENCHMARK] Completed with %d results", len(results))
}

// measureLatency measures round-trip latency using ping
func (d *Daemon) measureLatency(target string, samples int) *BenchmarkResult {
	latencies := make([]float64, 0, samples)

	for i := 0; i < samples; i++ {
		start := time.Now()

		// Use ping command for accurate latency
		cmd := exec.Command("ping", "-c", "1", "-W", "1", target)
		if err := cmd.Run(); err != nil {
			continue
		}

		latency := time.Since(start).Seconds() * 1000 // Convert to ms
		latencies = append(latencies, latency)

		time.Sleep(100 * time.Millisecond) // Small delay between samples
	}

	if len(latencies) == 0 {
		return nil
	}

	// Calculate statistics
	var sum, min, max float64
	min = latencies[0]
	max = latencies[0]
	for _, l := range latencies {
		sum += l
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}
	avg := sum / float64(len(latencies))

	// Calculate jitter (variation in latency)
	var jitterSum float64
	for _, l := range latencies {
		diff := l - avg
		if diff < 0 {
			diff = -diff
		}
		jitterSum += diff
	}
	jitter := jitterSum / float64(len(latencies))

	packetLoss := float64(samples-len(latencies)) / float64(samples)

	return &BenchmarkResult{
		Timestamp:      time.Now(),
		Target:         target,
		LatencyMs:      avg,
		PacketLoss:     packetLoss,
		Jitter:         jitter,
		Samples:        len(latencies),
		PayloadSize:    64, // ICMP ping default
		ThroughputMbps: 0,  // Not applicable for latency test
		Details: map[string]interface{}{
			"min_ms": min,
			"max_ms": max,
			"avg_ms": avg,
		},
	}
}

// measureThroughput measures data transfer speed via WebSocket
func (d *Daemon) measureThroughput(target string, payloadSize int, samples int) *BenchmarkResult {
	throughputs := make([]float64, 0, samples)

	// Create test payload
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	for i := 0; i < samples; i++ {
		// Measure time to send/receive via WebSocket
		start := time.Now()

		// Send benchmark ping through WebSocket
		err := d.sendWSMessage(map[string]interface{}{
			"type":      "benchmark_ping",
			"timestamp": start.UnixNano(),
			"size":      payloadSize,
		})
		if err != nil {
			continue
		}

		// Approximate throughput (actual measurement would need echo response)
		elapsed := time.Since(start).Seconds()
		if elapsed > 0 {
			// Calculate Mbps: (bytes * 8) / (seconds * 1000000)
			mbps := float64(payloadSize*8) / (elapsed * 1000000)
			throughputs = append(throughputs, mbps)
		}

		time.Sleep(200 * time.Millisecond)
	}

	if len(throughputs) == 0 {
		return nil
	}

	// Calculate average throughput
	var sum float64
	for _, t := range throughputs {
		sum += t
	}
	avg := sum / float64(len(throughputs))

	return &BenchmarkResult{
		Timestamp:      time.Now(),
		Target:         target,
		ThroughputMbps: avg,
		PayloadSize:    payloadSize,
		Samples:        len(throughputs),
		LatencyMs:      0,
		PacketLoss:     float64(samples-len(throughputs)) / float64(samples),
		Details: map[string]interface{}{
			"payload_bytes": payloadSize,
			"method":        "websocket",
		},
	}
}

// updateTrafficTime should be called whenever traffic is detected
func (d *Daemon) updateTrafficTime() {
	d.mu.Lock()
	d.lastTrafficTime = time.Now()
	d.mu.Unlock()
}

// ============================================================================
// Benchmark CLI Handlers
// ============================================================================

// cmdBenchmark handles the benchmark CLI command
func (d *Daemon) cmdBenchmark(args map[string]interface{}) CLIResponse {
	action, _ := args["action"].(string)

	switch action {
	case "run":
		// Manually trigger a benchmark
		d.mu.Lock()
		if d.benchmarkRunning {
			d.mu.Unlock()
			return CLIResponse{Success: false, Error: "benchmark already running"}
		}
		d.benchmarkRunning = true
		d.mu.Unlock()

		go func() {
			d.runBenchmark()
			d.mu.Lock()
			d.benchmarkRunning = false
			d.lastBenchmarkAt = time.Now()
			d.mu.Unlock()
		}()

		return CLIResponse{
			Success: true,
			Data:    map[string]string{"message": "Benchmark started"},
		}

	case "status":
		d.mu.RLock()
		status := map[string]interface{}{
			"running":        d.benchmarkRunning,
			"last_run":       d.lastBenchmarkAt.Format(time.RFC3339),
			"next_eligible":  d.lastBenchmarkAt.Add(benchmarkInterval).Format(time.RFC3339),
			"idle_threshold": benchmarkIdleThreshold.String(),
			"interval":       benchmarkInterval.String(),
		}
		d.mu.RUnlock()
		return CLIResponse{Success: true, Data: status}

	case "latest":
		// Return latest benchmark results from time series
		latencyLatest := d.latencySeries.GetLatest()
		throughputLatest := d.throughputSeries.GetLatest()

		data := map[string]interface{}{
			"latency":    latencyLatest,
			"throughput": throughputLatest,
		}
		return CLIResponse{Success: true, Data: data}

	default:
		// Default: return summary
		d.mu.RLock()
		summary := map[string]interface{}{
			"running":              d.benchmarkRunning,
			"last_run":             d.lastBenchmarkAt.Format(time.RFC3339),
			"latency_points":       len(d.latencySeries.GetPoints()),
			"throughput_points":    len(d.throughputSeries.GetPoints()),
			"latency_memory_kb":    d.latencySeries.MemoryUsage() / 1024,
			"throughput_memory_kb": d.throughputSeries.MemoryUsage() / 1024,
		}
		d.mu.RUnlock()
		return CLIResponse{Success: true, Data: summary}
	}
}

// cmdTimeSeries handles the timeseries CLI command
func (d *Daemon) cmdTimeSeries(args map[string]interface{}) CLIResponse {
	series, _ := args["series"].(string)
	lastStr, _ := args["last"].(string)

	// Determine which series to query
	var ts *TimeSeries
	switch series {
	case "latency":
		ts = d.latencySeries
	case "throughput":
		ts = d.throughputSeries
	default:
		// Return both series summary
		return CLIResponse{
			Success: true,
			Data: map[string]interface{}{
				"latency": map[string]interface{}{
					"name":       "latency",
					"points":     len(d.latencySeries.GetPoints()),
					"bucket_size": tsLatencyBucketSize.String(),
					"max_points": tsMaxLatencyPoints,
					"memory_kb":  d.latencySeries.MemoryUsage() / 1024,
				},
				"throughput": map[string]interface{}{
					"name":       "throughput",
					"points":     len(d.throughputSeries.GetPoints()),
					"bucket_size": tsThroughputBucketSize.String(),
					"max_points": tsMaxThroughputPoints,
					"memory_kb":  d.throughputSeries.MemoryUsage() / 1024,
				},
				"total_memory_kb": (d.latencySeries.MemoryUsage() + d.throughputSeries.MemoryUsage()) / 1024,
			},
		}
	}

	// Get points, optionally filtered by time
	var points []TimeSeriesPoint
	if lastStr != "" {
		since := parseLastDuration(lastStr)
		points = ts.GetPointsSince(since)
	} else {
		points = ts.GetPoints()
	}

	return CLIResponse{
		Success: true,
		Data: map[string]interface{}{
			"series":     series,
			"points":     points,
			"count":      len(points),
			"memory_kb":  ts.MemoryUsage() / 1024,
		},
	}
}
