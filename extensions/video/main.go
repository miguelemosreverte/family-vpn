package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/miguelemosreverte/family-vpn/extensions/framework"
	"github.com/miguelemosreverte/family-vpn/ipc"
	videocall "github.com/miguelemosreverte/family-vpn/video-call"
)

// VideoExtension implements video calling functionality
type VideoExtension struct {
	*framework.ExtensionBase
	vpnClient   *ipc.VPNClient
	videoServer *videocall.VideoServer
}

// NewVideoExtension creates a new video calling extension
func NewVideoExtension(vpnPort int) *VideoExtension {
	return &VideoExtension{
		ExtensionBase: framework.NewExtensionBase("video", "1.0.0"),
		vpnClient:     ipc.NewVPNClient(vpnPort),
	}
}

// Start starts the video extension
func (e *VideoExtension) Start() error {
	// Check VPN core is running
	if err := e.vpnClient.Health(); err != nil {
		return err
	}

	// Start video server
	e.videoServer = videocall.NewVideoServer()

	// Wire up SendToPeer to use VPN IPC
	e.videoServer.SendToPeer = func(peerIP string, data []byte) error {
		return e.vpnClient.SendSignal(peerIP, data)
	}

	port, err := e.videoServer.Start()
	if err != nil {
		return err
	}

	log.Printf("[VIDEO] Server started on port %d", port)

	// Subscribe to incoming video signals from VPN
	go e.monitorIncomingSignals()

	return nil
}

// Stop stops the video extension
func (e *VideoExtension) Stop() error {
	if e.videoServer != nil {
		return e.videoServer.Stop()
	}
	return nil
}

// Health returns true if extension is healthy
func (e *VideoExtension) Health() bool {
	if e.vpnClient == nil {
		return false
	}
	return e.vpnClient.Health() == nil
}

// monitorIncomingSignals monitors for incoming video call signals
func (e *VideoExtension) monitorIncomingSignals() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[VIDEO] Failed to get home dir: %v", err)
		return
	}

	signalFile := filepath.Join(homeDir, ".family-vpn-video-signal")
	lastContent := ""

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		data, err := os.ReadFile(signalFile)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		content := string(data)
		if content == lastContent || content == "" {
			continue
		}
		lastContent = content

		log.Printf("[VIDEO] Received incoming video signal")

		// Parse the signal
		var signal map[string]interface{}
		if err := json.Unmarshal(data, &signal); err != nil {
			log.Printf("[VIDEO] Failed to parse signal: %v", err)
			continue
		}

		// Handle different signal types
		sigType, _ := signal["type"].(string)
		switch sigType {
		case "call-start":
			// Auto-open video window for incoming call
			fromIP, _ := signal["from"].(string)
			fromName, _ := signal["fromName"].(string)
			if fromIP != "" {
				log.Printf("[VIDEO] Incoming call from %s (%s) - auto-opening", fromName, fromIP)
				go e.autoOpenVideoCall(fromIP, fromName)
			}
		case "offer", "answer", "ice-candidate":
			// Forward WebRTC signaling to video server
			if e.videoServer != nil {
				fromIP, _ := signal["peer"].(string)
				e.videoServer.HandlePeerMessage(fromIP, data)
			}
		}

		// Clear the file
		os.Remove(signalFile)
	}
}

// autoOpenVideoCall automatically opens a video call window
func (e *VideoExtension) autoOpenVideoCall(peerIP, peerName string) {
	if e.videoServer == nil {
		return
	}

	// Open browser window
	url := e.videoServer.GetURL(peerIP, peerName)
	cmd := exec.Command("open", url)
	if err := cmd.Start(); err != nil {
		log.Printf("[VIDEO] Failed to auto-open video window: %v", err)
	} else {
		log.Printf("[VIDEO] Auto-opened video call from %s", peerName)
	}
}

func main() {
	vpnPort := flag.Int("vpn-port", 8889, "VPN core IPC port")
	flag.Parse()

	ext := NewVideoExtension(*vpnPort)
	if err := ext.Run(ext); err != nil {
		log.Fatalf("Extension failed: %v", err)
	}
}
