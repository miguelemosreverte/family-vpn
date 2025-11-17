package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime/pprof"
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

type VPNServer struct {
	listenAddr string
	encryption bool
	key        []byte
	tunIface   *water.Interface
}

func NewVPNServer(listenAddr string, encryption bool, key []byte) *VPNServer {
	return &VPNServer{
		listenAddr: listenAddr,
		encryption: encryption,
		key:        key,
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

func (s *VPNServer) handleClient(conn net.Conn) {
	defer conn.Close()
	log.Printf("Client connected from %s", conn.RemoteAddr())

	// Tune TCP socket for high throughput
	if tcpConn, ok := conn.(*net.TCPConn); ok {
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

	// Channel for graceful shutdown
	done := make(chan bool)

	// TUN -> Client (egress)
	go func() {
		buffer := make([]byte, MTU)
		lengthBuf := make([]byte, 4) // Reuse length buffer
		writer := bufio.NewWriterSize(conn, 131072) // 128KB buffer for downloads
		var writerMutex sync.Mutex // Protect writer from concurrent access

		// Diagnostics
		var packetsSent, flushCount, totalBytesSent int64
		var lastReport = time.Now()
		// Timing breakdown (in microseconds)
		var timeTunRead, timeEncrypt, timeMutexWait, timeNetWrite, timeFlush int64

		// Background flusher: flush every 1ms for low latency while allowing batching
		flushDone := make(chan bool)
		go func() {
			ticker := time.NewTicker(1 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					writerMutex.Lock()
					buffered := writer.Buffered()
					if buffered > 0 {
						writer.Flush()
						flushCount++
					}
					writerMutex.Unlock()
				case <-flushDone:
					return
				}
			}
		}()
		defer close(flushDone)

		// Stats reporter
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
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
					log.Printf("[SERVER-EGRESS] %.0f pkt/s, %.2f Mbps, %.1f pkt/flush", pps, mbps, avgBatch)
					log.Printf("[TIMING] TUN:%.0fµs Encrypt:%.0fµs Mutex:%.0fµs NetWrite:%.0fµs Flush:%.0fµs",
						avgTunRead, avgEncrypt, avgMutex, avgNetWrite, avgFlush)
					packetsSent, totalBytesSent, flushCount = 0, 0, 0
					timeTunRead, timeEncrypt, timeMutexWait, timeNetWrite, timeFlush = 0, 0, 0, 0, 0
					lastReport = time.Now()
				case <-done:
					return
				}
			}
		}()

		for {
			// Measure TUN read
			t0 := time.Now()
			n, err := s.tunIface.Read(buffer)
			timeTunRead += time.Since(t0).Microseconds()
			if err != nil {
				log.Printf("TUN read error: %v", err)
				done <- true
				return
			}

			packet := buffer[:n]

			// Measure encryption
			t1 := time.Now()
			var encrypted []byte
			if clientWantsEncryption {
				encrypted, err = s.encryptData(packet)
				if err != nil {
					log.Printf("Encryption error: %v", err)
					continue
				}
			} else {
				encrypted = packet
			}
			timeEncrypt += time.Since(t1).Microseconds()

			// Send packet length first (4 bytes), then packet using buffered writer
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

			// Flush immediately if buffer is getting full (≥64KB, half of 128KB buffer)
			// This ensures fast TCP ramp-up without waiting for 1ms timer
			if writer.Buffered() >= 65536 {
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

func (s *VPNServer) Start() error {
	if err := s.setupTUN(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	defer listener.Close()

	log.Printf("VPN server listening on %s (encryption: %v)", s.listenAddr, s.encryption)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go s.handleClient(conn)
	}
}

func main() {
	port := flag.String("port", "8888", "Port to listen on")
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
	log.Fatal(server.Start())
}
