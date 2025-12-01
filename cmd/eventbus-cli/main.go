// eventbus-cli is a CLI tool for testing the Event Bus WebSocket API
//
// Usage:
//   eventbus-cli subscribe "versions.*"     # Subscribe to version events
//   eventbus-cli subscribe "*"              # Subscribe to all events
//   eventbus-cli snapshot                    # Request current snapshot
//   eventbus-cli versions                    # Get all client versions
//   eventbus-cli publish versions.client    # Publish a test event (for testing)
//   eventbus-cli watch                       # Watch all events in real-time
//
// The CLI connects to the VPN server's WebSocket endpoint and displays events.
// This is the foundation for UI components - the UI uses the same protocol.
//
// FAST MODE: If eventbus-daemon is running, the CLI communicates via Unix socket
// for instant responses. Otherwise, it falls back to direct WebSocket connection.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const daemonSocket = "/tmp/family-vpn-eventbus.sock"

// CLIRequest is sent to daemon
type CLIRequest struct {
	Command string                 `json:"cmd"`
	Args    map[string]interface{} `json:"args,omitempty"`
}

// CLIResponse from daemon
type CLIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// sendToDaemon sends a command to the daemon and returns the response
func sendToDaemon(req CLIRequest) (*CLIResponse, error) {
	conn, err := net.DialTimeout("unix", daemonSocket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Send request
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	var resp CLIResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// isDaemonRunning checks if the daemon is running
func isDaemonRunning() bool {
	conn, err := net.DialTimeout("unix", daemonSocket, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

var (
	serverURL = flag.String("server", "ws://10.8.0.1:9000/ws", "WebSocket server URL")
	vpnIP     = flag.String("vpn-ip", "", "VPN IP address (auto-detected if not provided)")
	pretty    = flag.Bool("pretty", true, "Pretty print JSON output")
	timeout   = flag.Duration("timeout", 30*time.Second, "Connection timeout")
)

// Event matches protocol.Event from events.go
type Event struct {
	Namespace  string      `json:"ns"`
	Timestamp  int64       `json:"ts"`
	Sequence   uint64      `json:"seq"`
	Data       interface{} `json:"data"`
	SnapshotID string      `json:"sid,omitempty"`
}

// Snapshot matches protocol.Snapshot from events.go
type Snapshot struct {
	ID           string                 `json:"id"`
	Timestamp    int64                  `json:"ts"`
	State        map[string]interface{} `json:"state"`
	LastSequence uint64                 `json:"last_seq"`
}

// Message represents a WebSocket message
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
	// Event fields (when type is event)
	Namespace string      `json:"ns,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	// Snapshot fields
	ID    string                 `json:"id,omitempty"`
	State map[string]interface{} `json:"state,omitempty"`
	// Version fields
	Versions      map[string]string `json:"versions,omitempty"`
	ServerVersion string            `json:"server_version,omitempty"`
}

func printJSON(v interface{}) {
	var data []byte
	var err error
	if *pretty {
		data, err = json.MarshalIndent(v, "", "  ")
	} else {
		data, err = json.Marshal(v)
	}
	if err != nil {
		log.Printf("Error marshaling JSON: %v", err)
		return
	}
	fmt.Println(string(data))
}

func formatTimestamp(ms int64) string {
	return time.UnixMilli(ms).Format("15:04:05.000")
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func printEvent(msg map[string]interface{}) {
	ns, _ := msg["ns"].(string)
	ts, _ := msg["ts"].(float64)
	seq, _ := msg["seq"].(float64)
	data := msg["data"]

	fmt.Printf("\n[%s] seq=%d ns=%s\n", formatTimestamp(int64(ts)), int(seq), ns)
	if data != nil {
		dataJSON, _ := json.MarshalIndent(data, "  ", "  ")
		fmt.Printf("  data: %s\n", dataJSON)
	}
}

func connectWebSocket() (*websocket.Conn, error) {
	// Build URL with VPN IP
	u, err := url.Parse(*serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %v", err)
	}

	q := u.Query()
	if *vpnIP != "" {
		q.Set("vpn_ip", *vpnIP)
	} else {
		// Use a default for CLI testing
		q.Set("vpn_ip", "cli-client")
	}
	u.RawQuery = q.Encode()

	log.Printf("Connecting to %s", u.String())

	dialer := websocket.Dialer{
		HandshakeTimeout: *timeout,
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return conn, nil
}

func cmdSubscribe(patterns []string) {
	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Send subscription request
	sub := map[string]interface{}{
		"type":       "subscribe",
		"namespaces": patterns,
	}
	if err := conn.WriteJSON(sub); err != nil {
		log.Fatalf("Failed to send subscription: %v", err)
	}

	log.Printf("Subscribed to: %v", patterns)
	fmt.Println("Listening for events (Ctrl+C to stop)...")
	fmt.Println("─────────────────────────────────────────")

	// Handle Ctrl+C
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Read events
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				log.Printf("Read error: %v", err)
				return
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Parse error: %v", err)
				continue
			}

			// Check if this is an event (has ns field)
			if _, hasNS := msg["ns"]; hasNS {
				printEvent(msg)
			} else if msgType, _ := msg["type"].(string); msgType != "" {
				// Handle other message types
				fmt.Printf("\n[%s] type=%s\n", time.Now().Format("15:04:05"), msgType)
				printJSON(msg)
			}
		}
	}()

	<-interrupt
	fmt.Println("\nDisconnecting...")
}

func cmdSnapshot() {
	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Request snapshot
	req := map[string]interface{}{
		"type": "get_snapshot",
	}
	if err := conn.WriteJSON(req); err != nil {
		log.Fatalf("Failed to request snapshot: %v", err)
	}

	// Read response
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	fmt.Println("=== Current Snapshot ===")
	printJSON(msg)
}

func cmdVersions() {
	// Try daemon first for fast response
	if isDaemonRunning() {
		resp, err := sendToDaemon(CLIRequest{Command: "versions"})
		if err == nil && resp.Success {
			fmt.Println("=== Client Versions === (via daemon)")
			if data, ok := resp.Data.(map[string]interface{}); ok {
				// Get hostname to IP mapping for display
				hostnameToIP := make(map[string]string)
				if h2ip, ok := data["hostname_to_ip"].(map[string]interface{}); ok {
					for hostname, ip := range h2ip {
						hostnameToIP[hostname], _ = ip.(string)
					}
				}

				if versions, ok := data["versions"].(map[string]interface{}); ok {
					if len(versions) == 0 {
						fmt.Println("No clients connected")
					}
					for hostname, commit := range versions {
						commitStr, _ := commit.(string)
						if len(commitStr) > 8 {
							commitStr = commitStr[:8]
						}
						// Show hostname with IP if available
						if ip, hasIP := hostnameToIP[hostname]; hasIP && ip != "" {
							fmt.Printf("  %s (%s): %s\n", hostname, ip, commitStr)
						} else {
							fmt.Printf("  %s: %s\n", hostname, commitStr)
						}
					}
				}
				if serverVersion, ok := data["server_version"].(string); ok && serverVersion != "" {
					if len(serverVersion) > 8 {
						serverVersion = serverVersion[:8]
					}
					fmt.Printf("\nServer: %s\n", serverVersion)
				}
			}
			return
		}
	}

	// Fall back to direct WebSocket
	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Request client versions
	req := map[string]interface{}{
		"type": "get_client_versions",
	}
	if err := conn.WriteJSON(req); err != nil {
		log.Fatalf("Failed to request versions: %v", err)
	}

	// Read response
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	fmt.Println("=== Client Versions ===")

	if versions, ok := msg["versions"].(map[string]interface{}); ok {
		if len(versions) == 0 {
			fmt.Println("No clients connected")
		}
		for ip, commit := range versions {
			commitStr, _ := commit.(string)
			if len(commitStr) > 8 {
				commitStr = commitStr[:8]
			}
			fmt.Printf("  %s: %s\n", ip, commitStr)
		}
	}

	if serverVersion, ok := msg["server_version"].(string); ok {
		if len(serverVersion) > 8 {
			serverVersion = serverVersion[:8]
		}
		fmt.Printf("\nServer: %s\n", serverVersion)
	}
}

func cmdWatch() {
	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Subscribe to all events
	sub := map[string]interface{}{
		"type":       "subscribe",
		"namespaces": []string{"*"},
	}
	if err := conn.WriteJSON(sub); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	fmt.Println("=== Event Watch ===")
	fmt.Println("Watching all events (Ctrl+C to stop)...")
	fmt.Println("─────────────────────────────────────────")

	// Handle Ctrl+C
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Keep connection alive with pings
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteJSON(map[string]string{"type": "ping"}); err != nil {
				return
			}
		}
	}()

	// Read all messages
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				log.Printf("Read error: %v", err)
				return
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Parse error: %v", err)
				continue
			}

			msgType, _ := msg["type"].(string)

			// Skip pong responses
			if msgType == "pong" {
				continue
			}

			// Check for event
			if _, hasNS := msg["ns"]; hasNS {
				printEvent(msg)
			} else {
				fmt.Printf("\n[%s] type=%s\n", time.Now().Format("15:04:05"), msgType)
				printJSON(msg)
			}
		}
	}()

	<-interrupt
	fmt.Println("\nDisconnecting...")
}

func cmdPingHistory() {
	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Request ping history
	req := map[string]interface{}{
		"type": "get_ping_history",
	}
	if err := conn.WriteJSON(req); err != nil {
		log.Fatalf("Failed to request ping history: %v", err)
	}

	// Read response
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	fmt.Println("=== Ping History ===")

	if history, ok := msg["history"].([]interface{}); ok {
		if len(history) == 0 {
			fmt.Println("No ping history available")
			return
		}

		fmt.Printf("Total records: %d\n\n", len(history))
		fmt.Println("Last 10 pings:")
		fmt.Println("─────────────────────────────────────────")

		start := len(history) - 10
		if start < 0 {
			start = 0
		}

		for _, record := range history[start:] {
			if rec, ok := record.(map[string]interface{}); ok {
				ts, _ := rec["timestamp"].(float64)
				target, _ := rec["target"].(string)
				latency, _ := rec["latency"].(float64)
				success, _ := rec["success"].(bool)

				status := "✓"
				if !success {
					status = "✗"
				}

				fmt.Printf("[%s] %s %s %dms\n",
					formatTimestamp(int64(ts)),
					status,
					target,
					int(latency))
			}
		}
	}
}

func cmdUpdate(targets []string, toVersion string) {
	// Try daemon first
	if isDaemonRunning() {
		targetsInterface := make([]interface{}, len(targets))
		for i, t := range targets {
			targetsInterface[i] = t
		}
		resp, err := sendToDaemon(CLIRequest{
			Command: "update",
			Args: map[string]interface{}{
				"targets": targetsInterface,
				"version": toVersion,
			},
		})
		if err == nil && resp.Success {
			fmt.Println("=== Update Event Broadcast === (via daemon)")
			if len(targets) == 1 && targets[0] == "all" {
				fmt.Println("Target: All connected clients")
			} else {
				fmt.Printf("Targets: %v\n", targets)
			}
			if toVersion != "" {
				fmt.Printf("Version: %s\n", toVersion)
			} else {
				fmt.Println("Version: latest")
			}
			fmt.Println("\nClients will pull and update autonomously.")
			return
		}
	}

	// Fall back to direct WebSocket
	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Broadcast an updates.available event - all clients listen for this
	// The server just broadcasts, clients decide whether to act based on targets
	event := map[string]interface{}{
		"ns": "updates.available",
		"ts": time.Now().UnixMilli(),
		"data": map[string]interface{}{
			"targets": targets,
			"version": toVersion, // empty = latest
			"domain":  "all",
		},
	}

	if err := conn.WriteJSON(event); err != nil {
		log.Fatalf("Failed to broadcast update event: %v", err)
	}

	fmt.Println("=== Update Event Broadcast ===")
	if len(targets) == 1 && targets[0] == "all" {
		fmt.Println("Target: All connected clients")
	} else {
		fmt.Printf("Targets: %v\n", targets)
	}
	if toVersion != "" {
		fmt.Printf("Version: %s\n", toVersion)
	} else {
		fmt.Println("Version: latest")
	}
	fmt.Println("\nClients will pull and update autonomously.")
}

func cmdRollback(target string, toVersion string) {
	if toVersion == "" {
		fmt.Println("Error: rollback requires --to <commit> flag")
		fmt.Println("Usage: eventbus-cli rollback <vpn-ip> --to <commit>")
		os.Exit(1)
	}

	conn, err := connectWebSocket()
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer conn.Close()

	// Broadcast a rollback event - target client will listen and act
	event := map[string]interface{}{
		"ns": "updates.available",
		"ts": time.Now().UnixMilli(),
		"data": map[string]interface{}{
			"targets":  []string{target},
			"version":  toVersion,
			"rollback": true,
			"domain":   "all",
		},
	}

	if err := conn.WriteJSON(event); err != nil {
		log.Fatalf("Failed to broadcast rollback event: %v", err)
	}

	fmt.Println("=== Rollback Event Broadcast ===")
	fmt.Printf("Target: %s\n", target)
	fmt.Printf("Version: %s\n", toVersion)
	fmt.Println("\nClient will checkout and rebuild autonomously.")
}

func cmdPeers() {
	// Read from local cache file - written by VPN client
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}

	peersFile := homeDir + "/.family-vpn-peers.json"
	data, err := os.ReadFile(peersFile)
	if err != nil {
		fmt.Println("=== Connected Peers ===")
		fmt.Println("No peer data available (VPN client not connected?)")
		fmt.Printf("\nPeers file: %s\n", peersFile)
		return
	}

	var peers []map[string]interface{}
	if err := json.Unmarshal(data, &peers); err != nil {
		log.Fatalf("Failed to parse peers file: %v", err)
	}

	fmt.Println("=== Connected Peers ===")

	if len(peers) == 0 {
		fmt.Println("No peers connected")
		return
	}

	for _, p := range peers {
		vpnIP, _ := p["vpn_address"].(string)
		hostname, _ := p["hostname"].(string)
		osName, _ := p["os"].(string)
		connectedAt, _ := p["connected_at"].(string)
		publicIP, _ := p["public_ip"].(string)

		fmt.Printf("  %s\n", vpnIP)
		fmt.Printf("    Hostname: %s\n", hostname)
		fmt.Printf("    OS: %s\n", osName)
		if publicIP != "" {
			fmt.Printf("    Public IP: %s\n", publicIP)
		}
		if connectedAt != "" {
			// Parse connected_at and calculate uptime
			if t, err := time.Parse(time.RFC3339, connectedAt); err == nil {
				uptime := time.Since(t).Round(time.Second)
				fmt.Printf("    Uptime: %s\n", formatDuration(uptime))
				fmt.Printf("    Connected: %s\n", connectedAt)
			} else {
				fmt.Printf("    Connected: %s\n", connectedAt)
			}
		}
		fmt.Println()
	}
}

var toVersion = flag.String("to", "", "Target version/commit for rollback")
var lastDuration = flag.String("last", "", "Time duration for logs (e.g., 2h, 30m, 1d)")
var sinceTime = flag.String("since", "", "Start time for logs (RFC3339 or YYYY-MM-DD)")
var untilTime = flag.String("until", "", "End time for logs (RFC3339 or YYYY-MM-DD)")
var logLimit = flag.Int("limit", 100, "Maximum number of log entries to show")
var logNamespace = flag.String("ns", "", "Filter logs by namespace prefix")

func cmdStatus() {
	if !isDaemonRunning() {
		fmt.Println("=== Daemon Status ===")
		fmt.Println("Status: NOT RUNNING")
		fmt.Println("\nStart the daemon with:")
		fmt.Println("  eventbus-daemon &")
		return
	}

	resp, err := sendToDaemon(CLIRequest{Command: "status"})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== Daemon Status ===")
	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		return
	}

	if data, ok := resp.Data.(map[string]interface{}); ok {
		connected, _ := data["connected"].(bool)
		hostname, _ := data["hostname"].(string)
		vpnIP, _ := data["vpn_ip"].(string)
		gitCommit, _ := data["git_commit"].(string)
		reconnects, _ := data["reconnects"].(float64)
		peerCount, _ := data["peer_count"].(float64)
		versionCount, _ := data["version_count"].(float64)

		status := "DISCONNECTED"
		if connected {
			status = "CONNECTED"
		}

		fmt.Printf("Status: %s\n", status)
		fmt.Printf("Hostname: %s\n", hostname)
		fmt.Printf("VPN IP: %s\n", vpnIP)
		fmt.Printf("Git Commit: %s\n", gitCommit)
		fmt.Printf("Reconnects: %d\n", int(reconnects))
		fmt.Printf("Cached Peers: %d\n", int(peerCount))
		fmt.Printf("Cached Versions: %d\n", int(versionCount))
	}
}

func cmdLogs(hostname string) {
	if !isDaemonRunning() {
		fmt.Println("Error: Daemon not running. Start with: eventbus-daemon &")
		os.Exit(1)
	}

	args := map[string]interface{}{
		"hostname":  hostname,
		"namespace": *logNamespace,
		"limit":     float64(*logLimit),
		"since":     *sinceTime,
		"until":     *untilTime,
		"last":      *lastDuration,
	}

	resp, err := sendToDaemon(CLIRequest{Command: "logs", Args: args})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Println("=== Event Logs ===")

	if data, ok := resp.Data.(map[string]interface{}); ok {
		// Show filter info
		if filter, ok := data["filter"].(map[string]interface{}); ok {
			if h, _ := filter["hostname"].(string); h != "" {
				fmt.Printf("Hostname: %s\n", h)
			}
			if ns, _ := filter["namespace"].(string); ns != "" {
				fmt.Printf("Namespace: %s\n", ns)
			}
		}

		count, _ := data["count"].(float64)
		fmt.Printf("Events: %d\n", int(count))
		fmt.Println("─────────────────────────────────────────")

		if events, ok := data["events"].([]interface{}); ok {
			for _, e := range events {
				if ev, ok := e.(map[string]interface{}); ok {
					ns, _ := ev["ns"].(string)
					hostname, _ := ev["hostname"].(string)
					ts, _ := ev["ts"].(string)

					// Parse and format timestamp
					if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
						ts = t.Format("15:04:05")
					}

					if hostname != "" {
						fmt.Printf("[%s] %s: %s\n", ts, hostname, ns)
					} else {
						fmt.Printf("[%s] %s\n", ts, ns)
					}

					// Show data summary if present
					if d, ok := ev["data"].(map[string]interface{}); ok && len(d) > 0 {
						dataJSON, _ := json.MarshalIndent(d, "  ", "  ")
						fmt.Printf("  %s\n", dataJSON)
					}
				}
			}
		}

		if int(count) == 0 {
			fmt.Println("No events found matching the filter")
		}
	}
}

func cmdStats(hostname string) {
	if !isDaemonRunning() {
		fmt.Println("Error: Daemon not running. Start with: eventbus-daemon &")
		os.Exit(1)
	}

	args := map[string]interface{}{"hostname": hostname}
	resp, err := sendToDaemon(CLIRequest{Command: "stats", Args: args})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Println("=== Client Statistics ===")

	if data, ok := resp.Data.(map[string]interface{}); ok {
		// Daemon stats
		if uptime, ok := data["daemon_uptime"].(float64); ok {
			duration := time.Duration(uptime) * time.Second
			fmt.Printf("Daemon Uptime: %s\n", duration.Round(time.Second))
		}
		if reconnects, ok := data["reconnects"].(float64); ok {
			fmt.Printf("Daemon Reconnects: %d\n", int(reconnects))
		}
		if events, ok := data["event_count"].(float64); ok {
			fmt.Printf("Events in History: %d\n", int(events))
		}

		fmt.Println()

		// Client stats
		if clients, ok := data["clients"].(map[string]interface{}); ok {
			if len(clients) == 0 {
				fmt.Println("No client statistics available yet")
				return
			}

			for h, stats := range clients {
				fmt.Printf("--- %s ---\n", h)
				if s, ok := stats.(map[string]interface{}); ok {
					if vpnIP, ok := s["vpn_ip"].(string); ok && vpnIP != "" {
						fmt.Printf("  VPN IP: %s\n", vpnIP)
					}
					if version, ok := s["current_version"].(string); ok && version != "" {
						if len(version) > 8 {
							version = version[:8]
						}
						fmt.Printf("  Version: %s\n", version)
					}
					if connects, ok := s["connects"].(float64); ok {
						fmt.Printf("  Connects: %d\n", int(connects))
					}
					if disconnects, ok := s["disconnects"].(float64); ok {
						fmt.Printf("  Disconnects: %d\n", int(disconnects))
					}
					if deploys, ok := s["deployments"].(float64); ok {
						fmt.Printf("  Deployments: %d\n", int(deploys))
					}
					if success, ok := s["successful_deploys"].(float64); ok {
						fmt.Printf("  Successful: %d\n", int(success))
					}
					if failed, ok := s["failed_deploys"].(float64); ok && failed > 0 {
						fmt.Printf("  Failed: %d\n", int(failed))
					}
					if uptime, ok := s["uptime_seconds"].(float64); ok {
						duration := time.Duration(uptime) * time.Second
						fmt.Printf("  Tracked Since: %s ago\n", duration.Round(time.Second))
					}
					if lastSeen, ok := s["last_seen"].(string); ok {
						if t, err := time.Parse(time.RFC3339Nano, lastSeen); err == nil {
							fmt.Printf("  Last Seen: %s\n", time.Since(t).Round(time.Second))
						}
					}
				}
				fmt.Println()
			}
		} else if client, ok := data["client"].(map[string]interface{}); ok {
			// Single client stats
			if h, ok := client["hostname"].(string); ok {
				fmt.Printf("--- %s ---\n", h)
			}
			if vpnIP, ok := client["vpn_ip"].(string); ok && vpnIP != "" {
				fmt.Printf("  VPN IP: %s\n", vpnIP)
			}
			if version, ok := client["current_version"].(string); ok && version != "" {
				if len(version) > 8 {
					version = version[:8]
				}
				fmt.Printf("  Version: %s\n", version)
			}
			if connects, ok := client["connects"].(float64); ok {
				fmt.Printf("  Connects: %d\n", int(connects))
			}
			if disconnects, ok := client["disconnects"].(float64); ok {
				fmt.Printf("  Disconnects: %d\n", int(disconnects))
			}
			if deploys, ok := client["deployments"].(float64); ok {
				fmt.Printf("  Deployments: %d\n", int(deploys))
			}
		}
	}
}

func cmdHealth() {
	if !isDaemonRunning() {
		fmt.Println("=== System Health ===")
		fmt.Println("Daemon: NOT RUNNING")
		fmt.Println("\nStart the daemon with: eventbus-daemon &")
		return
	}

	resp, err := sendToDaemon(CLIRequest{Command: "health", Args: map[string]interface{}{}})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Println("=== System Health ===")

	if data, ok := resp.Data.(map[string]interface{}); ok {
		// Daemon health
		if daemon, ok := data["daemon"].(map[string]interface{}); ok {
			connected, _ := daemon["connected"].(bool)
			status := "DISCONNECTED"
			if connected {
				status = "CONNECTED"
			}
			fmt.Printf("WebSocket: %s\n", status)

			if uptime, ok := daemon["uptime"].(string); ok {
				fmt.Printf("Uptime: %s\n", uptime)
			}
			if reconnects, ok := daemon["reconnects"].(float64); ok {
				fmt.Printf("Reconnects: %d\n", int(reconnects))
			}
			if events, ok := daemon["event_count"].(float64); ok {
				fmt.Printf("Events (24h): %d\n", int(events))
			}
			if recent, ok := daemon["recent_events"].(float64); ok {
				fmt.Printf("Events (1h): %d\n", int(recent))
			}
		}

		fmt.Println()

		// Client health
		if clients, ok := data["clients"].(map[string]interface{}); ok {
			if total, ok := clients["total"].(float64); ok {
				fmt.Printf("Clients Tracked: %d\n", int(total))
			}

			if health, ok := clients["health"].(map[string]interface{}); ok && len(health) > 0 {
				fmt.Println("\nClient Health:")
				for hostname, status := range health {
					statusStr, _ := status.(string)
					indicator := ""
					switch statusStr {
					case "healthy":
						indicator = "[OK]"
					case "stale":
						indicator = "[?]"
					case "offline":
						indicator = "[X]"
					}
					fmt.Printf("  %s %s (%s)\n", indicator, hostname, statusStr)
				}
			}

			if versions, ok := clients["versions"].(map[string]interface{}); ok && len(versions) > 0 {
				fmt.Println("\nVersions:")
				for hostname, version := range versions {
					versionStr, _ := version.(string)
					if len(versionStr) > 8 {
						versionStr = versionStr[:8]
					}
					fmt.Printf("  %s: %s\n", hostname, versionStr)
				}
			}
		}
	}
}

func cmdBenchmark(action string) {
	if !isDaemonRunning() {
		fmt.Println("Error: Daemon not running. Start with: eventbus-daemon &")
		os.Exit(1)
	}

	args := map[string]interface{}{"action": action}
	resp, err := sendToDaemon(CLIRequest{Command: "benchmark", Args: args})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	// Output as JSON for UI consumption
	printJSON(resp.Data)
}

func cmdTimeSeries(series string) {
	if !isDaemonRunning() {
		fmt.Println("Error: Daemon not running. Start with: eventbus-daemon &")
		os.Exit(1)
	}

	args := map[string]interface{}{
		"series": series,
		"last":   *lastDuration,
	}
	resp, err := sendToDaemon(CLIRequest{Command: "timeseries", Args: args})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	// Output as JSON for UI consumption
	printJSON(resp.Data)
}

func printUsage() {
	fmt.Println(`eventbus-cli - Event Bus CLI for Family VPN

Usage:
  eventbus-cli [flags] <command> [args...]

Commands:
  status                    Show daemon status
  versions                  Get all client versions
  peers                     List connected peers
  logs [hostname]           View event logs (Splunk-like)
  stats [hostname]          View client statistics
  health                    System health summary
  benchmark [run|status|latest]  Latency/throughput benchmarks
  timeseries [latency|throughput]  View time series data (JSON)
  update <target>           Trigger update on client(s)
  rollback <vpn-ip> --to <commit>  Rollback client to specific version
  subscribe <patterns...>   Subscribe to event patterns
  snapshot                  Get current state snapshot
  watch                     Watch all events in real-time
  ping-history              Get ping history

Update Targets:
  update all                Update all connected clients
  update 10.8.0.4           Update specific client by VPN IP
  update 10.8.0.4 10.8.0.5  Update multiple clients

Benchmark Actions:
  benchmark                 Show benchmark summary
  benchmark run             Manually trigger a benchmark
  benchmark status          Show benchmark scheduler status
  benchmark latest          Show latest benchmark results

Flags:`)
	flag.PrintDefaults()

	fmt.Println(`
Examples:
  eventbus-cli versions                      # Get client versions
  eventbus-cli peers                         # List connected peers
  eventbus-cli logs                          # View all event logs
  eventbus-cli logs --last 2h                # View logs from last 2 hours
  eventbus-cli logs MacBook-Air.local        # View logs for specific client
  eventbus-cli logs --ns versions            # Filter by namespace prefix
  eventbus-cli stats                         # View all client statistics
  eventbus-cli stats MacBook-Air.local       # View stats for specific client
  eventbus-cli health                        # System health summary
  eventbus-cli benchmark                     # Benchmark summary (JSON)
  eventbus-cli benchmark run                 # Manually run benchmark
  eventbus-cli timeseries latency            # Get latency time series (JSON)
  eventbus-cli timeseries --last 24h         # Get last 24h of data
  eventbus-cli update all                    # Update all clients to latest
  eventbus-cli update 10.8.0.4               # Update specific client
  eventbus-cli rollback 10.8.0.4 --to abc123 # Rollback to specific commit
  eventbus-cli subscribe "versions.*"        # Watch version events
  eventbus-cli watch                         # Watch everything

Event Patterns:
  versions.*    - All version events
  health.*      - All health events (ping, latency, uptime)
  peers.*       - Peer join/leave events
  system.*      - System events (connect, disconnect, snapshot)
  updates.*     - Update events
  benchmark.*   - Benchmark results
  *             - All events`)
}

func main() {
	flag.Usage = printUsage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "status":
		cmdStatus()

	case "subscribe":
		if len(args) < 2 {
			fmt.Println("Error: subscribe requires at least one pattern")
			fmt.Println("Usage: eventbus-cli subscribe <pattern> [pattern...]")
			os.Exit(1)
		}
		patterns := args[1:]
		cmdSubscribe(patterns)

	case "snapshot":
		cmdSnapshot()

	case "versions":
		cmdVersions()

	case "peers":
		cmdPeers()

	case "logs":
		hostname := ""
		// Filter out flags from positional args (flags should come before command for proper parsing)
		for i := 1; i < len(args); i++ {
			arg := args[i]
			// Skip flag-like arguments and their values
			if strings.HasPrefix(arg, "-") {
				// If it's a flag that takes a value (--last, --ns, etc), skip the next arg too
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
				}
				continue
			}
			hostname = arg
			break
		}
		cmdLogs(hostname)

	case "stats":
		hostname := ""
		if len(args) > 1 {
			hostname = args[1]
		}
		cmdStats(hostname)

	case "health":
		cmdHealth()

	case "update":
		if len(args) < 2 {
			fmt.Println("Error: update requires at least one target")
			fmt.Println("Usage: eventbus-cli update <vpn-ip|all> [vpn-ip...]")
			os.Exit(1)
		}
		targets := args[1:]
		cmdUpdate(targets, *toVersion)

	case "rollback":
		if len(args) < 2 {
			fmt.Println("Error: rollback requires a target")
			fmt.Println("Usage: eventbus-cli rollback <vpn-ip> --to <commit>")
			os.Exit(1)
		}
		target := args[1]
		cmdRollback(target, *toVersion)

	case "watch":
		cmdWatch()

	case "ping-history", "pings":
		cmdPingHistory()

	case "benchmark":
		action := ""
		if len(args) > 1 {
			action = args[1]
		}
		cmdBenchmark(action)

	case "timeseries", "ts":
		series := ""
		if len(args) > 1 {
			series = args[1]
		}
		cmdTimeSeries(series)

	case "help":
		printUsage()

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}
