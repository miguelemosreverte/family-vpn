package videocall

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

//go:embed ui.html
var uiHTML string

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local server
	},
}

// VideoServer handles the video call web interface
type VideoServer struct {
	port       int
	listener   net.Listener
	peers      map[string]*websocket.Conn // peer VPN IP -> websocket
	peersMutex sync.RWMutex

	// Callback to send data to peer over VPN
	SendToPeer func(peerIP string, data []byte) error
}

// NewVideoServer creates a new video call server
func NewVideoServer() *VideoServer {
	return &VideoServer{
		peers: make(map[string]*websocket.Conn),
	}
}

// Start starts the HTTP server on a random available port
func (s *VideoServer) Start() (int, error) {
	// Listen on random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to start listener: %v", err)
	}
	s.listener = listener
	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleUI)
	mux.HandleFunc("/ws", s.handleWebSocket)

	go http.Serve(listener, mux)
	log.Printf("[VIDEO] Server started on http://127.0.0.1:%d", s.port)
	return s.port, nil
}

// Stop stops the video server
func (s *VideoServer) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// GetURL returns the URL to open in browser
func (s *VideoServer) GetURL(peerIP, peerName string) string {
	return fmt.Sprintf("http://127.0.0.1:%d?peer=%s&name=%s", s.port, peerIP, peerName)
}

// handleUI serves the video call HTML page
func (s *VideoServer) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(uiHTML))
}

// handleWebSocket handles WebSocket connections from the browser
func (s *VideoServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[VIDEO] Failed to upgrade websocket: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("[VIDEO] Browser WebSocket connected from %s", r.RemoteAddr)

	// Read messages from browser and forward to peer over VPN
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[VIDEO] WebSocket read error: %v", err)
			break
		}

		// Parse message to get peer IP
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[VIDEO] Failed to parse message: %v", err)
			continue
		}

		peerIP, ok := msg["peer"].(string)
		if !ok {
			log.Printf("[VIDEO] Message missing peer IP")
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "offer", "answer", "ice-candidate":
			// WebRTC signaling - forward to peer over VPN
			if s.SendToPeer != nil {
				if err := s.SendToPeer(peerIP, data); err != nil {
					log.Printf("[VIDEO] Failed to send to peer %s: %v", peerIP, err)
				}
			}
		default:
			log.Printf("[VIDEO] Unknown message type: %s (messageType=%d)", msgType, messageType)
		}
	}
}

// HandlePeerMessage handles incoming video messages from VPN peer
func (s *VideoServer) HandlePeerMessage(peerIP string, data []byte) {
	// Find browser WebSocket and forward message
	s.peersMutex.RLock()
	conn, exists := s.peers[peerIP]
	s.peersMutex.RUnlock()

	if !exists {
		log.Printf("[VIDEO] No browser connection for peer %s", peerIP)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[VIDEO] Failed to write to browser: %v", err)
	}
}

// RegisterPeer registers a browser WebSocket for a specific peer
func (s *VideoServer) RegisterPeer(peerIP string, conn *websocket.Conn) {
	s.peersMutex.Lock()
	s.peers[peerIP] = conn
	s.peersMutex.Unlock()
	log.Printf("[VIDEO] Registered browser connection for peer %s", peerIP)
}
