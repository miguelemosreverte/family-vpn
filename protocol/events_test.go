package protocol

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestEventBusPublishSubscribe(t *testing.T) {
	eb := NewEventBus(0) // No automatic snapshots for testing
	defer eb.Stop()

	var received []Event
	var mu sync.Mutex

	// Subscribe to versions.*
	eb.Subscribe("versions.*", func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	// Publish events
	eb.Publish(NSVersionsClient, map[string]string{"hostname": "test-host", "version": "abc123"})
	eb.Publish(NSVersionsServer, map[string]string{"version": "def456"})
	eb.Publish(NSHealthPing, map[string]int{"latency": 50}) // Should NOT match

	// Wait for async dispatch
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 2 {
		t.Errorf("Expected 2 events, got %d", len(received))
	}

	// Check that both expected namespaces are present (order may vary due to async)
	found := make(map[string]bool)
	for _, e := range received {
		found[e.Namespace] = true
	}

	if !found[NSVersionsClient] {
		t.Errorf("Expected to receive %s event", NSVersionsClient)
	}

	if !found[NSVersionsServer] {
		t.Errorf("Expected to receive %s event", NSVersionsServer)
	}
}

func TestPatternMatching(t *testing.T) {
	tests := []struct {
		pattern   string
		namespace string
		expected  bool
	}{
		// Exact matches
		{"versions.client", "versions.client", true},
		{"versions.client", "versions.server", false},

		// Wildcard matches
		{"versions.*", "versions.client", true},
		{"versions.*", "versions.server", true},
		{"versions.*", "health.ping", false},

		// Global wildcard
		{"*", "versions.client", true},
		{"*", "health.ping", true},
		{"**", "anything.here", true},

		// Edge cases
		{"versions.*", "versions", false},   // No second part
		{"health.ping", "health.pong", false}, // Different second part
	}

	for _, tc := range tests {
		result := matchPattern(tc.pattern, tc.namespace)
		if result != tc.expected {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.namespace, result, tc.expected)
		}
	}
}

func TestEventSequencing(t *testing.T) {
	eb := NewEventBus(0)
	defer eb.Stop()

	e1 := eb.Publish("test.event1", nil)
	e2 := eb.Publish("test.event2", nil)
	e3 := eb.Publish("test.event3", nil)

	if e1.Sequence != 1 || e2.Sequence != 2 || e3.Sequence != 3 {
		t.Errorf("Sequence numbers not monotonically increasing: %d, %d, %d", e1.Sequence, e2.Sequence, e3.Sequence)
	}
}

func TestEventHistory(t *testing.T) {
	eb := NewEventBus(0)
	defer eb.Stop()

	// Publish some events
	for i := 0; i < 10; i++ {
		eb.Publish("test.event", map[string]int{"n": i})
	}

	// Get events since seq 5
	events := eb.GetEventsSince(5)

	if len(events) != 5 {
		t.Errorf("Expected 5 events since seq 5, got %d", len(events))
	}

	for i, e := range events {
		if e.Sequence != uint64(6+i) {
			t.Errorf("Event %d has seq %d, expected %d", i, e.Sequence, 6+i)
		}
	}
}

func TestSnapshot(t *testing.T) {
	eb := NewEventBus(0)
	defer eb.Stop()

	// Register state collectors
	eb.RegisterStateCollector("versions", func() interface{} {
		return map[string]string{
			"client1": "abc123",
			"client2": "def456",
		}
	})

	eb.RegisterStateCollector("health", func() interface{} {
		return map[string]interface{}{
			"uptime": 99.9,
			"lastPing": time.Now().Unix(),
		}
	})

	// Generate snapshot
	snapshot := eb.GenerateSnapshot()

	if snapshot == nil {
		t.Fatal("Snapshot is nil")
	}

	if snapshot.ID == "" {
		t.Error("Snapshot ID is empty")
	}

	if len(snapshot.State) != 2 {
		t.Errorf("Expected 2 state entries, got %d", len(snapshot.State))
	}

	versions, ok := snapshot.State["versions"].(map[string]string)
	if !ok {
		t.Fatal("versions state not found or wrong type")
	}

	if versions["client1"] != "abc123" {
		t.Errorf("Expected client1=abc123, got %s", versions["client1"])
	}
}

func TestSubscription(t *testing.T) {
	sub := Subscription{Patterns: []string{"versions.*", "health.ping"}}

	tests := []struct {
		namespace string
		expected  bool
	}{
		{"versions.client", true},
		{"versions.server", true},
		{"health.ping", true},
		{"health.uptime", false},
		{"updates.available", false},
	}

	for _, tc := range tests {
		result := sub.MatchesAny(tc.namespace)
		if result != tc.expected {
			t.Errorf("MatchesAny(%q) = %v, want %v", tc.namespace, result, tc.expected)
		}
	}
}

func TestEventJSON(t *testing.T) {
	eb := NewEventBus(0)
	defer eb.Stop()

	data := map[string]interface{}{
		"hostname": "test-host",
		"version":  "abc123",
	}
	event := eb.Publish(NSVersionsClient, data)

	// Serialize
	jsonBytes, err := event.JSON()
	if err != nil {
		t.Fatalf("Failed to serialize event: %v", err)
	}

	// Verify structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsed["ns"] != NSVersionsClient {
		t.Errorf("Expected ns=%s, got %v", NSVersionsClient, parsed["ns"])
	}

	if parsed["seq"].(float64) != float64(event.Sequence) {
		t.Errorf("Expected seq=%d, got %v", event.Sequence, parsed["seq"])
	}
}

func TestParseEvent(t *testing.T) {
	jsonStr := `{"ns":"versions.client","ts":1234567890123,"seq":42,"data":{"version":"abc123"}}`

	event, err := ParseEvent([]byte(jsonStr))
	if err != nil {
		t.Fatalf("Failed to parse event: %v", err)
	}

	if event.Namespace != "versions.client" {
		t.Errorf("Expected ns=versions.client, got %s", event.Namespace)
	}

	if event.Sequence != 42 {
		t.Errorf("Expected seq=42, got %d", event.Sequence)
	}

	if event.Timestamp != 1234567890123 {
		t.Errorf("Expected ts=1234567890123, got %d", event.Timestamp)
	}
}

func TestConcurrentPublish(t *testing.T) {
	eb := NewEventBus(0)
	defer eb.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				eb.Publish("test.concurrent", map[string]int{"g": n, "e": j})
			}
		}(i)
	}

	wg.Wait()

	// Verify events were recorded (may be less than total due to buffer limit)
	events := eb.GetEventsSince(0)
	expected := numGoroutines * eventsPerGoroutine
	if len(events) > expected {
		t.Errorf("More events than published: %d > %d", len(events), expected)
	}

	// Verify sequence numbers are unique within buffer
	seqs := make(map[uint64]bool)
	for _, e := range events {
		if seqs[e.Sequence] {
			t.Errorf("Duplicate sequence number: %d", e.Sequence)
		}
		seqs[e.Sequence] = true
	}

	// Verify sequence numbers are monotonically assigned
	if len(events) != expected {
		t.Logf("Buffer limited events from %d to %d (expected)", expected, len(events))
	}
}

func TestAutoSnapshot(t *testing.T) {
	// Create bus with 100ms snapshot interval
	eb := NewEventBus(100 * time.Millisecond)
	defer eb.Stop()

	var snapshotCount int
	var mu sync.Mutex

	eb.RegisterStateCollector("test", func() interface{} {
		return "test-state"
	})

	eb.Subscribe(NSSystemSnapshot, func(e Event) {
		mu.Lock()
		snapshotCount++
		mu.Unlock()
	})

	// Wait for at least 2 snapshots
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if snapshotCount < 2 {
		t.Errorf("Expected at least 2 snapshots, got %d", snapshotCount)
	}
}
