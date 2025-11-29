// Package protocol provides the event-driven architecture for Family VPN.
//
// This is Layer 0 - the foundational event bus that all components use.
// It provides namespaced events, subscriptions, and periodic snapshots.
//
// Architecture:
//
//	┌─────────────────────────────────────────────────────────────────┐
//	│                         Event Bus                               │
//	├─────────────────────────────────────────────────────────────────┤
//	│  Namespaces:                                                    │
//	│    system.*        - Core system events (connect, disconnect)   │
//	│    health.*        - Ping, latency, uptime events               │
//	│    versions.*      - Client/server version tracking             │
//	│    updates.*       - Hot-reload and update notifications        │
//	│    peers.*         - Peer discovery and status                  │
//	│    dashboard.*     - Dashboard-specific events                  │
//	├─────────────────────────────────────────────────────────────────┤
//	│  Snapshots:                                                     │
//	│    Every 5 minutes, consolidated state is broadcast             │
//	│    New clients receive snapshot immediately on connect          │
//	└─────────────────────────────────────────────────────────────────┘
//
// Event Format:
//
//	{
//	  "ns": "versions.client_updated",    // Namespace.EventType
//	  "ts": 1234567890123,                // Unix milliseconds
//	  "seq": 42,                          // Sequence number
//	  "data": { ... }                     // Event-specific payload
//	}
//
// Subscription Format (client -> server):
//
//	{
//	  "type": "subscribe",
//	  "namespaces": ["versions.*", "health.ping"]
//	}

package protocol

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Event represents a namespaced event in the system.
// This is the canonical format - it will not change.
type Event struct {
	// Namespace is the event identifier (e.g., "versions.client_updated")
	Namespace string `json:"ns"`

	// Timestamp in Unix milliseconds
	Timestamp int64 `json:"ts"`

	// Sequence number for ordering (monotonically increasing)
	Sequence uint64 `json:"seq"`

	// Data is the event-specific payload
	Data interface{} `json:"data"`

	// SnapshotID links events to their snapshot epoch (optional)
	SnapshotID string `json:"sid,omitempty"`
}

// Snapshot represents a consolidated state at a point in time.
type Snapshot struct {
	// ID uniquely identifies this snapshot
	ID string `json:"id"`

	// Timestamp when snapshot was created
	Timestamp int64 `json:"ts"`

	// State is the consolidated state by namespace
	State map[string]interface{} `json:"state"`

	// LastSequence is the sequence number of the last event included
	LastSequence uint64 `json:"last_seq"`
}

// Subscription represents a client's event subscriptions.
type Subscription struct {
	// Patterns are namespace patterns (e.g., "versions.*", "health.ping")
	Patterns []string `json:"namespaces"`
}

// EventBus manages event publishing, subscriptions, and snapshots.
type EventBus struct {
	sequence    uint64
	subscribers map[string][]EventHandler
	subMutex    sync.RWMutex

	// Snapshot management
	currentSnapshot *Snapshot
	snapshotMutex   sync.RWMutex
	snapshotTicker  *time.Ticker
	snapshotDone    chan struct{}

	// State collectors for snapshot generation
	stateCollectors map[string]StateCollector
	collectorMutex  sync.RWMutex

	// Event history for replay (limited buffer)
	eventHistory []Event
	historyMutex sync.RWMutex
	maxHistory   int
}

// EventHandler is called when an event matches a subscription.
type EventHandler func(event Event)

// StateCollector provides current state for a namespace during snapshot.
type StateCollector func() interface{}

// NewEventBus creates a new event bus with snapshot interval.
func NewEventBus(snapshotInterval time.Duration) *EventBus {
	eb := &EventBus{
		subscribers:     make(map[string][]EventHandler),
		stateCollectors: make(map[string]StateCollector),
		eventHistory:    make([]Event, 0, 1000),
		maxHistory:      1000,
	}

	// Start snapshot ticker if interval > 0
	if snapshotInterval > 0 {
		eb.snapshotTicker = time.NewTicker(snapshotInterval)
		eb.snapshotDone = make(chan struct{})
		go eb.snapshotLoop()
	}

	return eb
}

// Stop shuts down the event bus.
func (eb *EventBus) Stop() {
	if eb.snapshotTicker != nil {
		eb.snapshotTicker.Stop()
		close(eb.snapshotDone)
	}
}

// snapshotLoop runs the periodic snapshot generator.
func (eb *EventBus) snapshotLoop() {
	for {
		select {
		case <-eb.snapshotTicker.C:
			eb.GenerateSnapshot()
		case <-eb.snapshotDone:
			return
		}
	}
}

// RegisterStateCollector registers a function to collect state for a namespace.
func (eb *EventBus) RegisterStateCollector(namespace string, collector StateCollector) {
	eb.collectorMutex.Lock()
	defer eb.collectorMutex.Unlock()
	eb.stateCollectors[namespace] = collector
}

// GenerateSnapshot creates a new snapshot from all registered collectors.
func (eb *EventBus) GenerateSnapshot() *Snapshot {
	eb.collectorMutex.RLock()
	state := make(map[string]interface{})
	for ns, collector := range eb.stateCollectors {
		state[ns] = collector()
	}
	eb.collectorMutex.RUnlock()

	snapshot := &Snapshot{
		ID:           time.Now().UTC().Format("20060102T150405Z"),
		Timestamp:    time.Now().UnixMilli(),
		State:        state,
		LastSequence: atomic.LoadUint64(&eb.sequence),
	}

	eb.snapshotMutex.Lock()
	eb.currentSnapshot = snapshot
	eb.snapshotMutex.Unlock()

	// Publish snapshot event
	eb.Publish("system.snapshot", snapshot)

	return snapshot
}

// GetSnapshot returns the current snapshot.
func (eb *EventBus) GetSnapshot() *Snapshot {
	eb.snapshotMutex.RLock()
	defer eb.snapshotMutex.RUnlock()
	return eb.currentSnapshot
}

// Subscribe registers a handler for events matching the pattern.
// Patterns support wildcards: "versions.*" matches all version events.
func (eb *EventBus) Subscribe(pattern string, handler EventHandler) {
	eb.subMutex.Lock()
	defer eb.subMutex.Unlock()
	eb.subscribers[pattern] = append(eb.subscribers[pattern], handler)
}

// Unsubscribe removes all handlers for a pattern.
func (eb *EventBus) Unsubscribe(pattern string) {
	eb.subMutex.Lock()
	defer eb.subMutex.Unlock()
	delete(eb.subscribers, pattern)
}

// Publish sends an event to all matching subscribers.
func (eb *EventBus) Publish(namespace string, data interface{}) Event {
	seq := atomic.AddUint64(&eb.sequence, 1)

	event := Event{
		Namespace: namespace,
		Timestamp: time.Now().UnixMilli(),
		Sequence:  seq,
		Data:      data,
	}

	// Add to history
	eb.historyMutex.Lock()
	if len(eb.eventHistory) >= eb.maxHistory {
		// Remove oldest 10%
		eb.eventHistory = eb.eventHistory[eb.maxHistory/10:]
	}
	eb.eventHistory = append(eb.eventHistory, event)
	eb.historyMutex.Unlock()

	// Dispatch to subscribers
	eb.dispatch(event)

	return event
}

// dispatch sends an event to all matching subscribers.
func (eb *EventBus) dispatch(event Event) {
	eb.subMutex.RLock()
	defer eb.subMutex.RUnlock()

	for pattern, handlers := range eb.subscribers {
		if matchPattern(pattern, event.Namespace) {
			for _, handler := range handlers {
				go handler(event) // Non-blocking dispatch
			}
		}
	}
}

// matchPattern checks if a namespace matches a pattern.
// Supports wildcards: "versions.*" matches "versions.client_updated"
func matchPattern(pattern, namespace string) bool {
	// Exact match
	if pattern == namespace {
		return true
	}

	// Wildcard match
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(namespace, prefix+".")
	}

	// Double wildcard matches everything
	if pattern == "*" || pattern == "**" {
		return true
	}

	return false
}

// GetEventsSince returns events after the given sequence number.
func (eb *EventBus) GetEventsSince(seq uint64) []Event {
	eb.historyMutex.RLock()
	defer eb.historyMutex.RUnlock()

	var result []Event
	for _, e := range eb.eventHistory {
		if e.Sequence > seq {
			result = append(result, e)
		}
	}
	return result
}

// JSON serializes an event to JSON.
func (e Event) JSON() ([]byte, error) {
	return json.Marshal(e)
}

// MustJSON serializes or panics (for known-good events).
func (e Event) MustJSON() []byte {
	b, err := e.JSON()
	if err != nil {
		panic(err)
	}
	return b
}

// ParseEvent deserializes an event from JSON.
func ParseEvent(data []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// ParseSubscription deserializes a subscription request.
func ParseSubscription(data []byte) (*Subscription, error) {
	var s Subscription
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// MatchesAny checks if an event matches any of the subscription patterns.
func (s *Subscription) MatchesAny(namespace string) bool {
	for _, pattern := range s.Patterns {
		if matchPattern(pattern, namespace) {
			return true
		}
	}
	return false
}

// Common namespace constants for type safety.
const (
	// System events
	NSSystemConnect    = "system.connect"
	NSSystemDisconnect = "system.disconnect"
	NSSystemSnapshot   = "system.snapshot"
	NSSystemError      = "system.error"

	// Health events
	NSHealthPing    = "health.ping"
	NSHealthLatency = "health.latency"
	NSHealthUptime  = "health.uptime"

	// Version events
	NSVersionsClient = "versions.client"
	NSVersionsServer = "versions.server"
	NSVersionsSync   = "versions.sync"

	// Update events
	NSUpdatesAvailable = "updates.available"
	NSUpdatesApplied   = "updates.applied"
	NSUpdatesRejected  = "updates.rejected"

	// Peer events
	NSPeersJoined  = "peers.joined"
	NSPeersLeft    = "peers.left"
	NSPeersUpdated = "peers.updated"

	// Dashboard events
	NSDashboardRefresh = "dashboard.refresh"
)

// Predefined subscription patterns for common use cases.
var (
	SubAll      = Subscription{Patterns: []string{"*"}}
	SubVersions = Subscription{Patterns: []string{"versions.*"}}
	SubHealth   = Subscription{Patterns: []string{"health.*"}}
	SubPeers    = Subscription{Patterns: []string{"peers.*"}}
	SubUpdates  = Subscription{Patterns: []string{"updates.*"}}
)
