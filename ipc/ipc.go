package ipc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VPNClient provides IPC interface to VPN core for extensions
type VPNClient struct {
	baseURL string
	client  *http.Client
}

// NewVPNClient creates a new IPC client to communicate with VPN core
func NewVPNClient(port int) *VPNClient {
	return &VPNClient{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SendSignal sends a control signal to a specific peer via VPN
func (c *VPNClient) SendSignal(peerIP string, data []byte) error {
	payload := map[string]interface{}{
		"peer": peerIP,
		"data": string(data),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	resp, err := c.client.Post(c.baseURL+"/signal/send", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send signal: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("signal send failed: %s - %s", resp.Status, string(body))
	}

	return nil
}

// GetPeers retrieves list of connected VPN peers
func (c *VPNClient) GetPeers() ([]map[string]interface{}, error) {
	resp, err := c.client.Get(c.baseURL + "/peers")
	if err != nil {
		return nil, fmt.Errorf("failed to get peers: %v", err)
	}
	defer resp.Body.Close()

	var peers []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, fmt.Errorf("failed to decode peers: %v", err)
	}

	return peers, nil
}

// SubscribeToSignals subscribes to incoming signals for this extension
func (c *VPNClient) SubscribeToSignals(extensionName string, handler func(peerIP string, data []byte)) error {
	// Poll for signals (could be upgraded to websocket later)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		resp, err := c.client.Get(c.baseURL + "/signal/poll?extension=" + extensionName)
		if err != nil {
			continue
		}

		var signals []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&signals); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, signal := range signals {
			peerIP, _ := signal["peer"].(string)
			data, _ := signal["data"].(string)
			if peerIP != "" && data != "" {
				handler(peerIP, []byte(data))
			}
		}
	}

	return nil
}

// Health checks if VPN core is running
func (c *VPNClient) Health() error {
	resp, err := c.client.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("VPN core not responding: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("VPN core unhealthy: %s", resp.Status)
	}

	return nil
}
