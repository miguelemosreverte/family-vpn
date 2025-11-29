// Package protocol defines the update notification protocol for Family VPN.
//
// This is Layer 0 - the foundation that all other components build upon.
// It defines a simple, extensible message format for real-time updates.
//
// Design Principles:
// 1. VPN connection is sacred - never restart unless core binary changes
// 2. WebSocket is the transport - all notifications flow through it
// 3. Git is the source of truth - versions are commits
// 4. Components are independent - each domain handles its own updates
//
// Protocol Stack:
//   Layer 4: UI Components (HTML/CSS/JS that hot-swap)
//   Layer 3: Extensions (Plugins that reload via IPC)
//   Layer 2: Desktop Shell (Electron that reloads renderer)
//   Layer 1: VPN Core (Go binary - only restarts for breaking changes)
//   Layer 0: This protocol (Always on, routes messages by domain)

package protocol

import (
	"encoding/json"
	"time"
)

// Domain identifies which component should handle the update
type Domain string

const (
	// DomainAll broadcasts to all components (use sparingly)
	DomainAll Domain = "all"

	// DomainCore is the VPN tunnel binary - requires restart
	DomainCore Domain = "core"

	// DomainServer is the VPN server - handled server-side
	DomainServer Domain = "server"

	// DomainDesktop is the Electron shell - may require app restart
	DomainDesktop Domain = "desktop"

	// DomainUI is the renderer HTML/CSS/JS - hot-reloads without restart
	DomainUI Domain = "ui"

	// DomainMenubar is the menu bar app - can self-update
	DomainMenubar Domain = "menubar"

	// DomainExtension is a specific extension - reloads via IPC
	DomainExtension Domain = "extension"
)

// Action defines what the receiver should do
type Action string

const (
	// ActionReload means pull latest and reload (no restart)
	ActionReload Action = "reload"

	// ActionRestart means pull latest and restart the component
	ActionRestart Action = "restart"

	// ActionNotify means just show a notification, user decides
	ActionNotify Action = "notify"
)

// UpdateMessage is the canonical format for all update notifications.
// This is the artifact - it will not change.
type UpdateMessage struct {
	// Version is the protocol version (always 1 for now)
	Version int `json:"v"`

	// Domain identifies which component should handle this
	Domain Domain `json:"domain"`

	// Action tells the receiver what to do
	Action Action `json:"action"`

	// Commit is the Git commit hash (source of truth)
	Commit string `json:"commit,omitempty"`

	// Target is for extension-specific updates (e.g., "video", "ssh")
	Target string `json:"target,omitempty"`

	// ChangedFiles lists which files changed (for smart reload decisions)
	ChangedFiles []string `json:"files,omitempty"`

	// Timestamp when the update was pushed
	Timestamp time.Time `json:"ts"`

	// Message is optional human-readable description
	Message string `json:"msg,omitempty"`
}

// NewUpdateMessage creates a properly formatted update message
func NewUpdateMessage(domain Domain, action Action) *UpdateMessage {
	return &UpdateMessage{
		Version:   1,
		Domain:    domain,
		Action:    action,
		Timestamp: time.Now().UTC(),
	}
}

// WithCommit adds the Git commit hash
func (m *UpdateMessage) WithCommit(commit string) *UpdateMessage {
	m.Commit = commit
	return m
}

// WithTarget sets the extension target
func (m *UpdateMessage) WithTarget(target string) *UpdateMessage {
	m.Target = target
	return m
}

// WithFiles sets the changed files list
func (m *UpdateMessage) WithFiles(files []string) *UpdateMessage {
	m.ChangedFiles = files
	return m
}

// WithMessage adds a human-readable description
func (m *UpdateMessage) WithMessage(msg string) *UpdateMessage {
	m.Message = msg
	return m
}

// JSON serializes the message
func (m *UpdateMessage) JSON() ([]byte, error) {
	return json.Marshal(m)
}

// MustJSON serializes or panics (for known-good messages)
func (m *UpdateMessage) MustJSON() []byte {
	b, err := m.JSON()
	if err != nil {
		panic(err)
	}
	return b
}

// ParseUpdateMessage deserializes an update message
func ParseUpdateMessage(data []byte) (*UpdateMessage, error) {
	var m UpdateMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// RequiresRestart returns true if this update requires process restart
func (m *UpdateMessage) RequiresRestart() bool {
	return m.Action == ActionRestart || m.Domain == DomainCore
}

// ShouldHandle returns true if this receiver should handle the message
func (m *UpdateMessage) ShouldHandle(receiverDomain Domain, receiverTarget string) bool {
	// All domain matches everyone
	if m.Domain == DomainAll {
		return true
	}

	// Direct domain match
	if m.Domain == receiverDomain {
		// For extensions, also check target
		if m.Domain == DomainExtension && m.Target != "" {
			return m.Target == receiverTarget
		}
		return true
	}

	return false
}

// Example usage:
//
// Server sending UI hot-reload:
//   msg := NewUpdateMessage(DomainUI, ActionReload).
//       WithCommit("abc123").
//       WithFiles([]string{"renderer/app.js", "renderer/style.css"})
//   server.BroadcastWebSocket(msg.MustJSON())
//
// Client receiving:
//   msg, _ := ParseUpdateMessage(wsMessage)
//   if msg.ShouldHandle(DomainUI, "") {
//       if msg.RequiresRestart() {
//           restartApp()
//       } else {
//           hotReload()
//       }
//   }
