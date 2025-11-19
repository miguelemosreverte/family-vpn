package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/songgao/water"
)

const (
	MTU        = 1400 // Reduced to account for encryption overhead (GCM adds ~28 bytes)
	TUN_DEVICE = "tun0"
	SERVER_IP  = "10.8.0.1"
)

// PeerInfo represents a connected VPN peer
type PeerInfo struct {
	Hostname    string `json:"hostname"`
	VPNAddress  string `json:"vpn_address"`
	PublicIP    string `json:"public_ip"`
	ConnectedAt string `json:"connected_at"`
	OS          string `json:"os"`
}

type VPNClient struct {
	serverAddr   string
	encryption   bool
	key          []byte
	tunIface     *water.Interface
	conn         net.Conn
	enabled      bool
	originalGW   string
	tunName      string
	noTimeout    bool // If true, run indefinitely (for production use)
	useTLS       bool // If true, use TLS to look like HTTPS
	assignedIP   string // VPN IP assigned by server
	peers        []*PeerInfo // List of connected peers
	peersMutex   sync.RWMutex
}

func NewVPNClient(serverAddr string, encryption bool, key []byte, noTimeout bool, useTLS bool) *VPNClient {
	return &VPNClient{
		serverAddr: serverAddr,
		encryption: encryption,
		key:        key,
		enabled:    false,
		noTimeout:  noTimeout,
		useTLS:     useTLS,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *VPNClient) setupTUN() error {
	// Create TUN interface using water library (cross-platform)
	config := water.Config{
		DeviceType: water.TUN,
	}

	// On Linux, we can specify the device name
	if runtime.GOOS == "linux" {
		config.Name = TUN_DEVICE
	}

	iface, err := water.New(config)
	if err != nil {
		return fmt.Errorf("failed to create TUN device: %v", err)
	}
	c.tunIface = iface
	c.tunName = iface.Name()

	log.Printf("Created TUN device: %s", c.tunName)

	// Use assigned IP from server
	clientIP := c.assignedIP
	if clientIP == "" {
		return fmt.Errorf("no VPN IP assigned by server")
	}

	// Configure IP address based on OS
	if runtime.GOOS == "darwin" {
		// macOS uses ifconfig
		cmd := exec.Command("ifconfig", c.tunName, clientIP, SERVER_IP, "up")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to configure %s: %v", c.tunName, err)
		}
	} else {
		// Linux uses ip command
		cmd := exec.Command("ip", "addr", "add", clientIP+"/24", "dev", c.tunName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to assign IP: %v", err)
		}

		cmd = exec.Command("ip", "link", "set", "dev", c.tunName, "up")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to bring interface up: %v", err)
		}
	}

	log.Printf("TUN device %s configured with IP %s", c.tunName, clientIP)
	return nil
}

func (c *VPNClient) getDefaultGateway() (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("sh", "-c", "route -n get default | grep gateway | awk '{print $2}'")
	} else {
		cmd = exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}'")
	}
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	result := string(output)
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result, nil
}

func (c *VPNClient) routeAllTraffic() error {
	// Save original default gateway
	gw, err := c.getDefaultGateway()
	if err != nil {
		return fmt.Errorf("failed to get default gateway: %v", err)
	}
	c.originalGW = gw
	log.Printf("Original gateway: %s", c.originalGW)

	serverHost, _, _ := net.SplitHostPort(c.serverAddr)

	if runtime.GOOS == "darwin" {
		// macOS routing
		// Add route to VPN server through original gateway
		cmd := exec.Command("route", "-n", "add", "-host", serverHost, c.originalGW)
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to add server route: %v", err)
		}

		// Delete default route
		cmd = exec.Command("route", "-n", "delete", "default")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to delete default route: %v", err)
		}

		// Add default route through VPN
		cmd = exec.Command("route", "-n", "add", "-net", "default", SERVER_IP)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to add VPN route: %v", err)
		}
	} else {
		// Linux routing
		cmd := exec.Command("ip", "route", "add", serverHost, "via", c.originalGW)
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to add server route: %v", err)
		}

		cmd = exec.Command("ip", "route", "del", "default")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to delete default route: %v", err)
		}

		cmd = exec.Command("ip", "route", "add", "default", "via", SERVER_IP, "dev", c.tunName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to add VPN route: %v", err)
		}
	}

	// Configure DNS to use fast public resolvers through VPN
	// This prevents DNS leaks and improves privacy
	if runtime.GOOS == "darwin" {
		// macOS: Use networksetup to configure DNS
		cmd := exec.Command("networksetup", "-setdnsservers", "Wi-Fi", "1.1.1.1", "8.8.8.8")
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to set DNS servers: %v (DNS may leak)", err)
		} else {
			log.Println("DNS configured: 1.1.1.1 (Cloudflare), 8.8.8.8 (Google) through VPN")
		}
	} else {
		// Linux: Modify /etc/resolv.conf
		// TODO: Implement for Linux if needed
	}

	log.Println("All traffic now routed through VPN")
	return nil
}

func (c *VPNClient) restoreRouting() error {
	if runtime.GOOS == "darwin" {
		// macOS routing restoration
		cmd := exec.Command("route", "-n", "delete", "default")
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to delete VPN route: %v", err)
		}

		if c.originalGW != "" {
			cmd = exec.Command("route", "-n", "add", "-net", "default", c.originalGW)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to restore default route: %v", err)
			}
			log.Println("Routing restored to original gateway")
		}

		// Restore DNS to DHCP (automatic)
		cmd = exec.Command("networksetup", "-setdnsservers", "Wi-Fi", "Empty")
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to restore DNS: %v", err)
		} else {
			log.Println("DNS restored to automatic (DHCP)")
		}
	} else {
		// Linux routing restoration
		cmd := exec.Command("ip", "route", "del", "default", "dev", c.tunName)
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to delete VPN route: %v", err)
		}

		if c.originalGW != "" {
			cmd = exec.Command("ip", "route", "add", "default", "via", c.originalGW)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to restore default route: %v", err)
			}
			log.Println("Routing restored to original gateway")
		}
	}

	return nil
}

func (c *VPNClient) cleanupTUN() error {
	if c.tunIface != nil {
		c.tunIface.Close()
	}

	if runtime.GOOS == "darwin" {
		// macOS cleanup - utun devices are automatically removed when closed
		log.Printf("TUN device %s closed", c.tunName)
	} else {
		// Linux cleanup
		cmd := exec.Command("ip", "link", "set", "dev", c.tunName, "down")
		cmd.Run()

		cmd = exec.Command("ip", "tuntap", "del", "mode", "tun", "dev", c.tunName)
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to delete TUN device: %v", err)
		}
		log.Printf("TUN device %s removed", c.tunName)
	}

	return nil
}

func (c *VPNClient) encrypt(data []byte) ([]byte, error) {
	if !c.encryption {
		return data, nil
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func (c *VPNClient) decrypt(data []byte) ([]byte, error) {
	if !c.encryption {
		return data, nil
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func (c *VPNClient) Connect() error {
	var conn net.Conn
	var err error

	// Connect to server with or without TLS
	if c.useTLS {
		// Use TLS to look like HTTPS traffic
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // Skip cert verification for self-signed certs
		}
		conn, err = tls.Dial("tcp", c.serverAddr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to connect to server with TLS: %v", err)
		}
		log.Printf("Connected to VPN server at %s with TLS (looks like HTTPS!)", c.serverAddr)
	} else {
		// Plain TCP connection
		conn, err = net.Dial("tcp", c.serverAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to server: %v", err)
		}
		log.Printf("Connected to VPN server at %s", c.serverAddr)
	}

	// Tune TCP socket for high throughput
	if tlsConn, ok := conn.(*tls.Conn); ok {
		// For TLS connections, get underlying TCP connection
		if tcpConn, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			tcpConn.SetReadBuffer(1024 * 1024)  // 1MB receive buffer
			tcpConn.SetWriteBuffer(1024 * 1024) // 1MB send buffer
			tcpConn.SetNoDelay(true)            // Disable Nagle's algorithm for low latency
			log.Printf("TCP socket tuned: 1MB buffers, NoDelay enabled")
		}
	} else if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetReadBuffer(1024 * 1024)  // 1MB receive buffer
		tcpConn.SetWriteBuffer(1024 * 1024) // 1MB send buffer
		tcpConn.SetNoDelay(true)            // Disable Nagle's algorithm for low latency
		log.Printf("TCP socket tuned: 1MB buffers, NoDelay enabled")
	}

	c.conn = conn

	// Send encryption preference to server
	encryptByte := byte(0)
	if c.encryption {
		encryptByte = byte(1)
	}
	if _, err := conn.Write([]byte{encryptByte}); err != nil {
		return fmt.Errorf("failed to send encryption preference: %v", err)
	}
	log.Printf("Encryption: %v", c.encryption)

	// Send peer info to server
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown"
	}
	peerInfo := &PeerInfo{
		Hostname: hostname,
		OS:       runtime.GOOS,
	}
	peerInfoJSON, err := json.Marshal(peerInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal peer info: %v", err)
	}

	// Send peer info length + data
	peerInfoLen := make([]byte, 4)
	binary.BigEndian.PutUint32(peerInfoLen, uint32(len(peerInfoJSON)))
	if _, err := conn.Write(peerInfoLen); err != nil {
		return fmt.Errorf("failed to send peer info length: %v", err)
	}
	if _, err := conn.Write(peerInfoJSON); err != nil {
		return fmt.Errorf("failed to send peer info: %v", err)
	}

	// Receive assigned VPN IP from server
	vpnIPLenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, vpnIPLenBuf); err != nil {
		return fmt.Errorf("failed to read VPN IP length: %v", err)
	}
	vpnIPLen := binary.BigEndian.Uint32(vpnIPLenBuf)

	vpnIPBuf := make([]byte, vpnIPLen)
	if _, err := io.ReadFull(conn, vpnIPBuf); err != nil {
		return fmt.Errorf("failed to read VPN IP: %v", err)
	}
	c.assignedIP = string(vpnIPBuf)
	log.Printf("[PEERS] Assigned VPN IP: %s", c.assignedIP)

	// Setup TUN with assigned IP
	if err := c.setupTUN(); err != nil {
		return err
	}

	// Route traffic
	if err := c.routeAllTraffic(); err != nil {
		c.cleanupTUN()
		return err
	}

	c.enabled = true
	done := make(chan bool)

	// TUN -> Server (egress)
	go func() {
		buffer := make([]byte, MTU)
		lengthBuf := make([]byte, 4) // Reuse length buffer
		writer := bufio.NewWriter(conn) // Buffered writer
		var writerMutex sync.Mutex // Protect writer from concurrent access

		// Diagnostics
		var packetsSent, flushCount, totalBytesSent int64
		var lastReport = time.Now()
		// Timing breakdown (in microseconds)
		var timeTunRead, timeEncrypt, timeMutexWait, timeNetWrite, timeFlush int64

		// Background flusher: aggressive flushing for fast TCP ramp-up
		// Flush immediately when buffer ≥ 4KB, OR every 1ms for smaller amounts
		go func() {
			ticker := time.NewTicker(1 * time.Millisecond)
			defer ticker.Stop()
			for c.enabled {
				<-ticker.C
				writerMutex.Lock()
				buffered := writer.Buffered()
				if buffered > 0 {
					writer.Flush()
					flushCount++
				}
				writerMutex.Unlock()
			}
		}()

		// Stats reporter
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for c.enabled {
				<-ticker.C
				elapsed := time.Since(lastReport).Seconds()
				pps := float64(packetsSent) / elapsed
				mbps := (float64(totalBytesSent) * 8) / (elapsed * 1000000)
				avgBatch := float64(packetsSent) / float64(flushCount)
				if flushCount == 0 {
					avgBatch = 0
				}
				// Calculate average time per operation in microseconds
				var avgTunRead, avgEncrypt, avgMutex, avgNetWrite, avgFlush float64
				if packetsSent > 0 {
					avgTunRead = float64(timeTunRead) / float64(packetsSent)
					avgEncrypt = float64(timeEncrypt) / float64(packetsSent)
					avgMutex = float64(timeMutexWait) / float64(packetsSent)
					avgNetWrite = float64(timeNetWrite) / float64(packetsSent)
					avgFlush = float64(timeFlush) / float64(packetsSent)
				}
				log.Printf("[EGRESS] %.0f pkt/s, %.2f Mbps, %.1f pkt/flush", pps, mbps, avgBatch)
				log.Printf("[TIMING] TUN:%.0fµs Encrypt:%.0fµs Mutex:%.0fµs NetWrite:%.0fµs Flush:%.0fµs",
					avgTunRead, avgEncrypt, avgMutex, avgNetWrite, avgFlush)
				packetsSent, totalBytesSent, flushCount = 0, 0, 0
				timeTunRead, timeEncrypt, timeMutexWait, timeNetWrite, timeFlush = 0, 0, 0, 0, 0
				lastReport = time.Now()
			}
		}()

		for c.enabled {
			// Measure TUN read
			t0 := time.Now()
			n, err := c.tunIface.Read(buffer)
			timeTunRead += time.Since(t0).Microseconds()
			if err != nil {
				log.Printf("TUN read error: %v", err)
				done <- true
				return
			}

			packet := buffer[:n]

			// Measure encryption
			t1 := time.Now()
			encrypted, err := c.encrypt(packet)
			timeEncrypt += time.Since(t1).Microseconds()
			if err != nil {
				log.Printf("Encryption error: %v", err)
				continue
			}

			// Send packet length first, then packet using buffered writer
			binary.BigEndian.PutUint32(lengthBuf, uint32(len(encrypted)))

			// Measure mutex wait time
			t2 := time.Now()
			writerMutex.Lock()
			timeMutexWait += time.Since(t2).Microseconds()

			// Measure network writes
			t3 := time.Now()
			if _, err := writer.Write(lengthBuf); err != nil {
				writerMutex.Unlock()
				log.Printf("Failed to send length: %v", err)
				done <- true
				return
			}
			if _, err := writer.Write(encrypted); err != nil {
				writerMutex.Unlock()
				log.Printf("Failed to send packet: %v", err)
				done <- true
				return
			}
			timeNetWrite += time.Since(t3).Microseconds()

			// Flush immediately if buffer reaches 2KB to minimize latency during TCP slow start
			// Lower threshold than server (2KB vs 4KB) since client initiates most connections
			// This prevents micro-bursts that confuse TCP congestion control
			if writer.Buffered() >= 2048 {
				t4 := time.Now()
				writer.Flush()
				timeFlush += time.Since(t4).Microseconds()
				flushCount++
			}
			writerMutex.Unlock()
			// Update stats
			packetsSent++
			totalBytesSent += int64(len(encrypted))
			// Note: Periodic flusher handles remaining small batches
		}
	}()

	// Server -> TUN (ingress)
	go func() {
		lengthBuf := make([]byte, 4)
		packetBuf := make([]byte, MTU*2) // Reuse packet buffer (sized for encrypted packets)
		reader := bufio.NewReader(conn)  // Buffered reader

		// Diagnostics
		var packetsRecv, totalBytesRecv int64
		var lastReport = time.Now()
		// Timing breakdown (in microseconds)
		var timeNetRead, timeDecrypt, timeTunWrite int64

		// Stats reporter
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for c.enabled {
				<-ticker.C
				elapsed := time.Since(lastReport).Seconds()
				pps := float64(packetsRecv) / elapsed
				mbps := (float64(totalBytesRecv) * 8) / (elapsed * 1000000)
				// Calculate average time per operation in microseconds
				var avgNetRead, avgDecrypt, avgTunWrite float64
				if packetsRecv > 0 {
					avgNetRead = float64(timeNetRead) / float64(packetsRecv)
					avgDecrypt = float64(timeDecrypt) / float64(packetsRecv)
					avgTunWrite = float64(timeTunWrite) / float64(packetsRecv)
				}
				log.Printf("[INGRESS] %.0f pkt/s, %.2f Mbps", pps, mbps)
				log.Printf("[TIMING] NetRead:%.0fµs Decrypt:%.0fµs TUNWrite:%.0fµs",
					avgNetRead, avgDecrypt, avgTunWrite)
				packetsRecv, totalBytesRecv = 0, 0
				timeNetRead, timeDecrypt, timeTunWrite = 0, 0, 0
				lastReport = time.Now()
			}
		}()

		for c.enabled {
			// Measure network read (length + packet)
			t0 := time.Now()
			// Read packet length
			if _, err := io.ReadFull(reader, lengthBuf); err != nil {
				log.Printf("Failed to read length: %v", err)
				done <- true
				return
			}

			length := binary.BigEndian.Uint32(lengthBuf)
			if length > MTU*2 { // Sanity check
				log.Printf("Invalid packet length: %d", length)
				done <- true
				return
			}

			// Reuse buffer by slicing to the exact length needed
			if _, err := io.ReadFull(reader, packetBuf[:length]); err != nil {
				log.Printf("Failed to read packet: %v", err)
				done <- true
				return
			}
			timeNetRead += time.Since(t0).Microseconds()

			// Measure decryption
			t1 := time.Now()
			packet, err := c.decrypt(packetBuf[:length])
			timeDecrypt += time.Since(t1).Microseconds()
			if err != nil {
				log.Printf("Decryption error: %v", err)
				continue
			}

			// Check if this is a control message
			if len(packet) > 5 && string(packet[:5]) == "CTRL:" {
				c.handleControlMessage(packet[5:])
				continue
			}

			// Measure TUN write
			t2 := time.Now()
			if _, err := c.tunIface.Write(packet); err != nil {
				log.Printf("TUN write error: %v", err)
				done <- true
				return
			}
			timeTunWrite += time.Since(t2).Microseconds()

			// Update stats
			packetsRecv++
			totalBytesRecv += int64(len(packet))
		}
	}()

	// Monitor outgoing video signals and send to peers via VPN
	go c.monitorOutgoingVideoSignals(conn)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Setup timeout for development safety (60 seconds by default)
	var timeoutChan <-chan time.Time
	if !c.noTimeout {
		log.Println("Development mode: VPN will automatically shut down after 60 seconds")
		log.Println("Use --no-timeout flag to run indefinitely")
		timeoutChan = time.After(60 * time.Second)
	} else {
		log.Println("Running in production mode (no timeout)")
	}

	select {
	case <-done:
		log.Println("Connection lost")
	case <-sigChan:
		log.Println("Shutting down...")
	case <-timeoutChan:
		log.Println("Safety timeout reached (60 seconds). Shutting down...")
	}

	return c.Disconnect()
}

func (c *VPNClient) handleControlMessage(message []byte) {
	command := string(message)

	// Check if this is a PEER_LIST message
	if strings.HasPrefix(command, "PEER_LIST:") {
		peerListJSON := command[10:] // Skip "PEER_LIST:" prefix
		var peerList []*PeerInfo
		if err := json.Unmarshal([]byte(peerListJSON), &peerList); err != nil {
			log.Printf("[PEERS] Failed to parse peer list: %v", err)
			return
		}

		c.peersMutex.Lock()
		c.peers = peerList
		c.peersMutex.Unlock()

		log.Printf("[PEERS] Updated peer list: %d peers connected", len(peerList))
		for _, peer := range peerList {
			log.Printf("[PEERS]   - %s (%s) at %s", peer.Hostname, peer.OS, peer.VPNAddress)
		}

		// Write peer list to file for menu bar app
		c.writePeerListToFile()
		return
	}

	// Check if this is a VIDEO_CALL message
	if strings.HasPrefix(command, "VIDEO_CALL:") {
		log.Printf("[VIDEO] Received video call message")
		c.handleVideoCallMessage(command[11:]) // Skip "VIDEO_CALL:" prefix
		return
	}

	log.Printf("[CONTROL] Received: %s", command)

	switch command {
	case "UPDATE_AVAILABLE":
		log.Println("[CONTROL] Update available! Writing to update signal file...")
		// Write update signal that menu bar app will detect
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[CONTROL] Failed to get home dir: %v", err)
			return
		}
		signalFile := filepath.Join(homeDir, ".family-vpn-update-signal")
		if err := os.WriteFile(signalFile, []byte("update"), 0644); err != nil {
			log.Printf("[CONTROL] Failed to write update signal: %v", err)
		} else {
			log.Println("[CONTROL] Update signal written successfully")
		}
	default:
		log.Printf("[CONTROL] Unknown command: %s", command)
	}
}

// writePeerListToFile writes the peer list to a file for the menu bar app
func (c *VPNClient) writePeerListToFile() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[PEERS] Failed to get home dir: %v", err)
		return
	}

	peerFile := filepath.Join(homeDir, ".family-vpn-peers.json")

	c.peersMutex.RLock()
	peerJSON, err := json.MarshalIndent(c.peers, "", "  ")
	c.peersMutex.RUnlock()

	if err != nil {
		log.Printf("[PEERS] Failed to marshal peer list: %v", err)
		return
	}

	if err := os.WriteFile(peerFile, peerJSON, 0644); err != nil {
		log.Printf("[PEERS] Failed to write peer file: %v", err)
	} else {
		log.Printf("[PEERS] Wrote peer list to %s", peerFile)
	}
}

// handleVideoCallMessage handles incoming video call signaling from peers
func (c *VPNClient) handleVideoCallMessage(data string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[VIDEO] Failed to get home dir: %v", err)
		return
	}

	// Write video call signal to file for menu bar app to detect
	signalFile := filepath.Join(homeDir, ".family-vpn-video-signal")
	if err := os.WriteFile(signalFile, []byte(data), 0644); err != nil {
		log.Printf("[VIDEO] Failed to write video signal: %v", err)
	} else {
		log.Printf("[VIDEO] Wrote video call signal to %s", signalFile)
	}
}

// monitorOutgoingVideoSignals monitors for outgoing video signals and sends them via VPN
func (c *VPNClient) monitorOutgoingVideoSignals(conn net.Conn) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[VIDEO] Failed to get home dir: %v", err)
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	processedFiles := make(map[string]bool)

	for c.enabled {
		<-ticker.C

		// Scan for video signal files
		files, err := filepath.Glob(filepath.Join(homeDir, ".family-vpn-video-out-*"))
		if err != nil {
			continue
		}

		for _, file := range files {
			if processedFiles[file] {
				continue // Already processed
			}

			// Read signal data
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}

			content := string(data)
			if content == "" {
				continue
			}

			log.Printf("[VIDEO] Found outgoing video signal in %s", file)

			// Send via VPN as control message
			message := []byte("CTRL:VIDEO_CALL:" + content)

			// Encrypt the control message
			encrypted, err := c.encrypt(message)
			if err != nil {
				log.Printf("[VIDEO] Failed to encrypt video signal: %v", err)
				continue
			}

			// Send length + encrypted packet
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, uint32(len(encrypted)))

			if _, err := conn.Write(lengthBuf); err != nil {
				log.Printf("[VIDEO] Failed to send video signal length: %v", err)
				continue
			}

			if _, err := conn.Write(encrypted); err != nil {
				log.Printf("[VIDEO] Failed to send video signal: %v", err)
				continue
			}

			log.Printf("[VIDEO] Sent video signal to server")

			// Mark as processed and delete file
			processedFiles[file] = true
			os.Remove(file)
		}
	}
}

func (c *VPNClient) Disconnect() error {
	c.enabled = false

	if err := c.restoreRouting(); err != nil {
		log.Printf("Failed to restore routing: %v", err)
	}

	if c.conn != nil {
		c.conn.Close()
	}

	if err := c.cleanupTUN(); err != nil {
		log.Printf("Failed to cleanup TUN: %v", err)
	}

	log.Println("VPN disconnected")
	return nil
}

func main() {
	server := flag.String("server", "", "VPN server address (e.g., 95.217.238.72:443)")
	encrypt := flag.Bool("encrypt", false, "Enable encryption")
	useTLS := flag.Bool("tls", true, "Use TLS to look like HTTPS (default true)")
	cpuprofile := flag.String("cpuprofile", "", "Write CPU profile to file")
	noTimeout := flag.Bool("no-timeout", false, "Run indefinitely (default: 60s timeout for safety)")
	flag.Parse()

	if *server == "" {
		log.Fatal("Server address is required. Use -server flag")
	}

	// Start CPU profiling if requested
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
		log.Printf("CPU profiling enabled, writing to: %s", *cpuprofile)
	}

	// Use same key as server (in production, use proper key exchange)
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes for AES-256

	client := NewVPNClient(*server, *encrypt, key, *noTimeout, *useTLS)
	if err := client.Connect(); err != nil {
		log.Fatal(err)
	}
}
