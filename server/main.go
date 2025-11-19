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
	"net/http"
	"os"
	"os/exec"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/songgao/water"
)

const (
	MTU         = 1400  // Reduced to account for encryption overhead (GCM adds ~28 bytes)
	TUN_DEVICE  = "tun0"
	VPN_NETWORK = "10.8.0.0/24"
	SERVER_IP   = "10.8.0.1"
)

// PeerInfo represents a connected VPN client
type PeerInfo struct {
	Hostname   string `json:"hostname"`
	VPNAddress string `json:"vpn_address"`
	PublicIP   string `json:"public_ip"`
	ConnectedAt string `json:"connected_at"`
	OS         string `json:"os"`
}

type VPNServer struct {
	listenAddr   string
	encryption   bool
	key          []byte
	tunIface     *water.Interface
	clients      map[net.Conn]bool
	clientsMutex sync.RWMutex
	tlsConfig    *tls.Config
	useTLS       bool
	// Peer registry for remote access
	peers         map[string]*PeerInfo  // key: VPN IP address
	peersMutex    sync.RWMutex
	nextClientIP  int  // Counter for assigning IPs (10.8.0.2, 10.8.0.3, etc.)
	// Peer-to-peer routing
	peerConnections map[string]net.Conn // key: VPN IP address, value: client connection
	peerEncryption  map[string]bool     // key: VPN IP address, value: wants encryption
}

func NewVPNServer(listenAddr string, encryption bool, key []byte) *VPNServer {
	return &VPNServer{
		listenAddr:      listenAddr,
		encryption:      encryption,
		key:             key,
		clients:         make(map[net.Conn]bool),
		peers:           make(map[string]*PeerInfo),
		peerConnections: make(map[string]net.Conn),
		peerEncryption:  make(map[string]bool),
		nextClientIP:    2, // Start from 10.8.0.2 (10.8.0.1 is server)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *VPNServer) setupTUN() error {
	// Create TUN interface using water library
	config := water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: TUN_DEVICE,
		},
	}

	iface, err := water.New(config)
	if err != nil {
		return fmt.Errorf("failed to create TUN device: %v", err)
	}
	s.tunIface = iface

	log.Printf("Created TUN device: %s", iface.Name())

	// Delete any existing IP (ignore errors)
	exec.Command("ip", "addr", "flush", "dev", iface.Name()).Run()

	// Assign IP address
	cmd := exec.Command("ip", "addr", "add", SERVER_IP+"/24", "dev", iface.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to assign IP to %s: %v - %s", iface.Name(), err, string(output))
	}

	// Increase TX queue length to prevent packet drops during bursts
	// Default 500 is too small for high-throughput VPN traffic
	cmd = exec.Command("ip", "link", "set", "dev", iface.Name(), "txqueuelen", "10000")
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: failed to set txqueuelen: %v", err)
	} else {
		log.Printf("TUN TX queue length set to 10000 (prevents burst packet drops)")
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", "dev", iface.Name(), "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring interface up: %v", err)
	}

	// Enable IP forwarding
	cmd = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %v", err)
	}

	// Configure TCP MSS clamping to prevent fragmentation issues with encryption
	// MSS = MTU (1400) - IP header (20) - TCP header (20) = 1360
	// This ensures TCP segments fit within VPN MTU after adding encryption overhead (~28 bytes)
	cmd = exec.Command("iptables", "-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360")
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: failed to set TCP MSS clamping: %v (downloads may fail)", err)
	} else {
		log.Printf("TCP MSS clamping enabled (MSS=1360) for MTU=%d", MTU)
	}

	log.Printf("TUN device %s configured with IP %s", iface.Name(), SERVER_IP)
	return nil
}

// encryptData always encrypts the data (doesn't check s.encryption flag)
func (s *VPNServer) encryptData(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
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

// decryptData always decrypts the data (doesn't check s.encryption flag)
func (s *VPNServer) decryptData(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
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

// getDestinationIP extracts the destination IP from an IP packet
func getDestinationIP(packet []byte) string {
	if len(packet) < 20 {
		return "" // Invalid IP packet
	}
	// IP header destination is at bytes 16-19
	return fmt.Sprintf("%d.%d.%d.%d", packet[16], packet[17], packet[18], packet[19])
}

// registerPeer adds a new peer to the registry and broadcasts updated list
func (s *VPNServer) registerPeer(vpnIP, hostname, publicIP, os string, conn net.Conn, wantsEncryption bool) {
	s.peersMutex.Lock()
	s.peers[vpnIP] = &PeerInfo{
		Hostname:    hostname,
		VPNAddress:  vpnIP,
		PublicIP:    publicIP,
		ConnectedAt: time.Now().Format(time.RFC3339),
		OS:          os,
	}
	s.peerConnections[vpnIP] = conn
	s.peerEncryption[vpnIP] = wantsEncryption
	s.peersMutex.Unlock()

	log.Printf("[PEERS] Registered: %s (%s) at %s", hostname, os, vpnIP)
	s.broadcastPeerList()
}

// unregisterPeer removes a peer and broadcasts updated list
func (s *VPNServer) unregisterPeer(vpnIP string) {
	s.peersMutex.Lock()
	if peer, exists := s.peers[vpnIP]; exists {
		log.Printf("[PEERS] Unregistered: %s at %s", peer.Hostname, vpnIP)
		delete(s.peers, vpnIP)
	}
	delete(s.peerConnections, vpnIP)
	delete(s.peerEncryption, vpnIP)
	s.peersMutex.Unlock()

	s.broadcastPeerList()
}

// broadcastPeerList sends current peer list to all clients
func (s *VPNServer) broadcastPeerList() {
	s.peersMutex.RLock()
	peerList := make([]*PeerInfo, 0, len(s.peers))
	for _, peer := range s.peers {
		peerList = append(peerList, peer)
	}
	s.peersMutex.RUnlock()

	peerJSON, err := json.Marshal(peerList)
	if err != nil {
		log.Printf("[PEERS] Failed to marshal peer list: %v", err)
		return
	}

	command := "PEER_LIST:" + string(peerJSON)
	s.broadcastControlMessage(command)
	log.Printf("[PEERS] Broadcasted peer list to all clients (%d peers)", len(peerList))
}

// broadcastControlMessage sends a control message to all connected clients
func (s *VPNServer) broadcastControlMessage(command string) {
	s.clientsMutex.RLock()
	clientCount := len(s.clients)
	s.clientsMutex.RUnlock()

	log.Printf("[CONTROL] Broadcasting '%s' to %d client(s)", command, clientCount)

	// Create control message packet
	message := append([]byte("CTRL:"), []byte(command)...)

	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	for conn := range s.clients {
		go func(c net.Conn) {
			// Encrypt the control message
			encrypted, err := s.encryptData(message)
			if err != nil {
				log.Printf("[CONTROL] Failed to encrypt message for %s: %v", c.RemoteAddr(), err)
				return
			}

			// Send length + encrypted packet
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, uint32(len(encrypted)))

			if _, err := c.Write(lengthBuf); err != nil {
				log.Printf("[CONTROL] Failed to send length to %s: %v", c.RemoteAddr(), err)
				return
			}

			if _, err := c.Write(encrypted); err != nil {
				log.Printf("[CONTROL] Failed to send message to %s: %v", c.RemoteAddr(), err)
				return
			}

			log.Printf("[CONTROL] Sent '%s' to %s", command, c.RemoteAddr())
		}(conn)
	}
}

// sendToPeer sends a control message to a specific peer
func (s *VPNServer) sendToPeer(peerIP, command string) error {
	s.peersMutex.RLock()
	conn, exists := s.peerConnections[peerIP]
	wantsEncryption, encryptExists := s.peerEncryption[peerIP]
	s.peersMutex.RUnlock()

	if !exists || !encryptExists {
		return fmt.Errorf("peer %s not found or not connected", peerIP)
	}

	log.Printf("[CONTROL] Sending '%s' to peer %s", command, peerIP)

	// Create control message packet
	message := append([]byte("CTRL:"), []byte(command)...)

	// Encrypt if peer wants encryption
	var toSend []byte
	if wantsEncryption {
		encrypted, err := s.encryptData(message)
		if err != nil {
			return fmt.Errorf("failed to encrypt message: %v", err)
		}
		toSend = encrypted
	} else {
		toSend = message
	}

	// Send length + packet
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(toSend)))

	if _, err := conn.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to send length: %v", err)
	}

	if _, err := conn.Write(toSend); err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}

	log.Printf("[CONTROL] Sent to peer %s successfully", peerIP)
	return nil
}

func (s *VPNServer) handleClient(conn net.Conn) {
	defer conn.Close()

	// Extract public IP - handle both TLS and non-TLS connections
	var publicIP string
	if tlsConn, ok := conn.(*tls.Conn); ok {
		// For TLS connections, get the underlying connection's remote address
		publicIP = tlsConn.RemoteAddr().String()
		if tcpAddr, ok := tlsConn.RemoteAddr().(*net.TCPAddr); ok {
			publicIP = tcpAddr.IP.String()
		}
	} else if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		publicIP = tcpAddr.IP.String()
	} else {
		publicIP = conn.RemoteAddr().String()
	}
	log.Printf("Client connected from %s", publicIP)

	// Register client connection
	s.clientsMutex.Lock()
	s.clients[conn] = true
	s.clientsMutex.Unlock()

	var assignedVPNIP string

	// Unregister client on disconnect
	defer func() {
		s.clientsMutex.Lock()
		delete(s.clients, conn)
		s.clientsMutex.Unlock()

		if assignedVPNIP != "" {
			s.unregisterPeer(assignedVPNIP)
		}
		log.Printf("Client disconnected: %s", publicIP)
	}()

	// Tune TCP socket for high throughput - handle both TLS and non-TLS
	var tcpConn *net.TCPConn
	if tlsConn, ok := conn.(*tls.Conn); ok {
		// For TLS connections, get underlying TCP connection
		if underlying, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			tcpConn = underlying
		}
	} else if directTCP, ok := conn.(*net.TCPConn); ok {
		tcpConn = directTCP
	}

	if tcpConn != nil {
		tcpConn.SetReadBuffer(1024 * 1024)  // 1MB receive buffer
		tcpConn.SetWriteBuffer(1024 * 1024) // 1MB send buffer
		tcpConn.SetNoDelay(true)            // Disable Nagle's algorithm for low latency
		log.Printf("TCP socket tuned: 1MB buffers, NoDelay enabled")
	}

	// Read client's encryption preference
	encryptByte := make([]byte, 1)
	if _, err := conn.Read(encryptByte); err != nil {
		log.Printf("Failed to read client encryption preference: %v", err)
		return
	}
	clientWantsEncryption := encryptByte[0] == 1
	log.Printf("Client encryption preference: %v", clientWantsEncryption)

	// Read peer info length (4 bytes)
	peerInfoLenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, peerInfoLenBuf); err != nil {
		log.Printf("Failed to read peer info length: %v", err)
		return
	}
	peerInfoLen := binary.BigEndian.Uint32(peerInfoLenBuf)

	// Read peer info JSON
	peerInfoBuf := make([]byte, peerInfoLen)
	if _, err := io.ReadFull(conn, peerInfoBuf); err != nil {
		log.Printf("Failed to read peer info: %v", err)
		return
	}

	var peerInfo PeerInfo
	if err := json.Unmarshal(peerInfoBuf, &peerInfo); err != nil {
		log.Printf("Failed to parse peer info: %v", err)
		return
	}

	// Assign VPN IP address
	s.peersMutex.Lock()
	assignedVPNIP = fmt.Sprintf("10.8.0.%d", s.nextClientIP)
	s.nextClientIP++
	s.peersMutex.Unlock()

	// Send assigned VPN IP back to client
	vpnIPBytes := []byte(assignedVPNIP)
	vpnIPLen := make([]byte, 4)
	binary.BigEndian.PutUint32(vpnIPLen, uint32(len(vpnIPBytes)))
	if _, err := conn.Write(vpnIPLen); err != nil {
		log.Printf("Failed to send VPN IP length: %v", err)
		return
	}
	if _, err := conn.Write(vpnIPBytes); err != nil {
		log.Printf("Failed to send VPN IP: %v", err)
		return
	}

	// Register peer in registry
	s.registerPeer(assignedVPNIP, peerInfo.Hostname, publicIP, peerInfo.OS, conn, clientWantsEncryption)
	log.Printf("[PEERS] Assigned %s to %s (%s)", assignedVPNIP, peerInfo.Hostname, peerInfo.OS)

	// Channel for graceful shutdown
	done := make(chan bool)

	// Note: TUN -> Client (egress) is now handled by centralized router (startTUNRouter)
	// This allows proper peer-to-peer packet forwarding based on destination IP

	// Client -> TUN (ingress)
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
			for {
				select {
				case <-ticker.C:
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
					log.Printf("[SERVER-INGRESS] %.0f pkt/s, %.2f Mbps", pps, mbps)
					log.Printf("[TIMING] NetRead:%.0fµs Decrypt:%.0fµs TUNWrite:%.0fµs",
						avgNetRead, avgDecrypt, avgTunWrite)
					packetsRecv, totalBytesRecv = 0, 0
					timeNetRead, timeDecrypt, timeTunWrite = 0, 0, 0
					lastReport = time.Now()
				case <-done:
					return
				}
			}
		}()

		for {
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
			var packet []byte
			var err error
			if clientWantsEncryption {
				packet, err = s.decryptData(packetBuf[:length])
				if err != nil {
					log.Printf("Decryption error: %v", err)
					continue
				}
			} else {
				packet = packetBuf[:length]
			}
			timeDecrypt += time.Since(t1).Microseconds()

			// Check if this is a VIDEO_CALL control message
			if len(packet) > 17 && string(packet[:17]) == "CTRL:VIDEO_CALL:" {
				// Extract peer IP and data: peerIP:data
				videoData := string(packet[17:])
				parts := strings.SplitN(videoData, ":", 2)
				if len(parts) == 2 {
					targetPeerIP := parts[0]
					signalData := parts[1]
					log.Printf("[VIDEO] Forwarding video call signal to peer %s", targetPeerIP)
					go s.sendToPeer(targetPeerIP, "VIDEO_CALL:"+signalData)
				}
				continue // Don't write control messages to TUN
			}

			// Measure TUN write
			t2 := time.Now()
			if _, err := s.tunIface.Write(packet); err != nil {
				log.Printf("TUN write error: %v (packet size: %d)", err, len(packet))
				done <- true
				return
			}
			timeTunWrite += time.Since(t2).Microseconds()

			// Update stats
			packetsRecv++
			totalBytesRecv += int64(len(packet))
		}
	}()

	<-done
	log.Printf("Client %s disconnected", conn.RemoteAddr())
}

// startTUNRouter starts the centralized TUN packet router
// This goroutine reads all packets from TUN and routes them to the correct peer
func (s *VPNServer) startTUNRouter() {
	go func() {
		buffer := make([]byte, MTU)
		log.Printf("[ROUTER] Starting centralized TUN packet router")

		for {
			// Read packet from TUN device
			n, err := s.tunIface.Read(buffer)
			if err != nil {
				log.Printf("[ROUTER] TUN read error: %v", err)
				continue
			}

			packet := make([]byte, n)
			copy(packet, buffer[:n])

			// Parse destination IP from packet
			destIP := getDestinationIP(packet)
			if destIP == "" {
				log.Printf("[ROUTER] Invalid IP packet, skipping")
				continue
			}

			// Look up which peer owns this destination IP
			s.peersMutex.RLock()
			targetConn, connExists := s.peerConnections[destIP]
			wantsEncryption, encryptExists := s.peerEncryption[destIP]
			s.peersMutex.RUnlock()

			if !connExists || !encryptExists {
				// Destination is not a connected peer, skip
				// (might be Internet-bound traffic, which is handled elsewhere)
				continue
			}

			// Encrypt if needed
			var toSend []byte
			if wantsEncryption {
				encrypted, err := s.encryptData(packet)
				if err != nil {
					log.Printf("[ROUTER] Encryption error for %s: %v", destIP, err)
					continue
				}
				toSend = encrypted
			} else {
				toSend = packet
			}

			// Send packet length (4 bytes) + packet to target peer
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, uint32(len(toSend)))

			// Write to connection (errors are expected if peer disconnects)
			if _, err := targetConn.Write(lengthBuf); err != nil {
				continue
			}
			if _, err := targetConn.Write(toSend); err != nil {
				continue
			}
		}
	}()
}

func (s *VPNServer) Start() error {
	if err := s.setupTUN(); err != nil {
		return err
	}

	// Start centralized TUN router for peer-to-peer traffic
	s.startTUNRouter()

	var listener net.Listener
	var err error

	if s.useTLS {
		// Use TLS listener to look like HTTPS traffic
		listener, err = tls.Listen("tcp", s.listenAddr, s.tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to listen with TLS: %v", err)
		}
		log.Printf("VPN server listening on %s with TLS (looks like HTTPS!) (encryption: %v)", s.listenAddr, s.encryption)
	} else {
		// Plain TCP listener
		listener, err = net.Listen("tcp", s.listenAddr)
		if err != nil {
			return fmt.Errorf("failed to listen: %v", err)
		}
		log.Printf("VPN server listening on %s (encryption: %v)", s.listenAddr, s.encryption)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go s.handleClient(conn)
	}
}

var globalServer *VPNServer

// webhookHandler handles GitHub webhook POSTs
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse webhook payload
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("[WEBHOOK] Failed to parse payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Check if this is a push to main branch
	ref, ok := payload["ref"].(string)
	if !ok || ref != "refs/heads/main" {
		log.Printf("[WEBHOOK] Ignoring non-main branch push: %s", ref)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("[WEBHOOK] Push to main detected! Notifying all clients...")

	// Broadcast update notification to all connected clients
	if globalServer != nil {
		globalServer.broadcastControlMessage("UPDATE_AVAILABLE")
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// updateInitHandler triggers server and client updates
func updateInitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse component parameter (e.g., ?component=video)
	component := r.URL.Query().Get("component")
	if component == "" {
		component = "all"
	}

	log.Printf("[UPDATE] Update initialization request received for component: %s", component)

	// Broadcast component-specific update message to all clients
	if globalServer != nil {
		updateMessage := fmt.Sprintf("UPDATE_%s", strings.ToUpper(component))
		log.Printf("[UPDATE] Broadcasting %s to all clients...", updateMessage)
		globalServer.broadcastControlMessage(updateMessage)
	}

	// Respond to the request before updating (so deploy script gets response)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Update initiated for component: %s\n", component)

	// Only restart server if component is "vpn" or "all"
	if component == "vpn" || component == "all" {
		// Spawn background goroutine to update and restart server
		go func() {
			log.Printf("[UPDATE] Starting server self-update in 2 seconds...")
			time.Sleep(2 * time.Second)

			// Execute update script
			updateScript := "/root/family-vpn/server-update.sh"
			cmd := exec.Command("/bin/bash", updateScript)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			log.Printf("[UPDATE] Executing update script: %s", updateScript)
			if err := cmd.Run(); err != nil {
				log.Printf("[UPDATE] Update script failed: %v", err)
			}
		}()
	} else {
		log.Printf("[UPDATE] Component %s does not require server restart", component)
	}
}

func main() {
	port := flag.String("port", "443", "Port to listen on (default 443 for HTTPS stealth)")
	webhookPort := flag.String("webhook-port", "9000", "Port for GitHub webhook server")
	tlsCert := flag.String("tls-cert", "certs/server.crt", "Path to TLS certificate")
	tlsKey := flag.String("tls-key", "certs/server.key", "Path to TLS private key")
	useTLS := flag.Bool("tls", true, "Use TLS to look like HTTPS (default true)")
	cpuprofile := flag.String("cpuprofile", "", "Write CPU profile to file")
	flag.Parse()

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

	// Generate or use a fixed key (in production, use proper key management)
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes for AES-256

	server := NewVPNServer(":"+*port, false, key)

	// Load TLS certificates if TLS is enabled
	if *useTLS {
		cert, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("Failed to load TLS certificates: %v", err)
		}
		server.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		server.useTLS = true
		log.Printf("TLS enabled: cert=%s, key=%s", *tlsCert, *tlsKey)
	}

	globalServer = server

	// Start HTTP server for webhooks and update endpoint
	http.HandleFunc("/webhook", webhookHandler)
	http.HandleFunc("/update/init", updateInitHandler)
	go func() {
		log.Printf("Starting HTTP server on port %s", *webhookPort)
		log.Printf("  - POST /webhook - GitHub webhook endpoint")
		log.Printf("  - POST /update/init - Trigger server and client updates")
		if err := http.ListenAndServe(":"+*webhookPort, nil); err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	log.Fatal(server.Start())
}
