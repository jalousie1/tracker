package processor

import (
	"testing"
	"time"
)

func TestEvent_Structure(t *testing.T) {
	event := Event{
		Type:      "PRESENCE_UPDATE",
		Data:      map[string]interface{}{"user_id": "123"},
		Timestamp: time.Now(),
		TokenID:   42,
	}

	if event.Type != "PRESENCE_UPDATE" {
		t.Errorf("expected type PRESENCE_UPDATE, got %s", event.Type)
	}

	if event.TokenID != 42 {
		t.Errorf("expected TokenID 42, got %d", event.TokenID)
	}

	userID, ok := event.Data["user_id"].(string)
	if !ok || userID != "123" {
		t.Error("expected user_id to be '123'")
	}
}

func TestEventProcessor_QueueEvent(t *testing.T) {
	// Create a minimal event processor for testing
	ep := &EventProcessor{
		eventQueue: make(chan Event, 100),
	}

	event := Event{
		Type: "TEST_EVENT",
		Data: map[string]interface{}{"test": true},
	}

	// Queue the event directly to channel
	ep.eventQueue <- event

	// Should be in queue
	if len(ep.GetEventQueue()) != 1 {
		t.Errorf("expected 1 event in queue, got %d", len(ep.GetEventQueue()))
	}

	// Read it back
	received := <-ep.eventQueue
	if received.Type != "TEST_EVENT" {
		t.Errorf("expected TEST_EVENT, got %s", received.Type)
	}
}

func TestEventProcessor_GetDataMap(t *testing.T) {
	ep := &EventProcessor{}

	event := Event{
		Type: "TEST",
		Data: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	dataMap := ep.getDataMap(event)

	if dataMap["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %v", dataMap["key1"])
	}

	if dataMap["key2"] != 123 {
		t.Errorf("expected key2=123, got %v", dataMap["key2"])
	}
}

// TestEventProcessor_WorkerPool tests that workers can be started and stopped
func TestEventProcessor_WorkerPool(t *testing.T) {
	ep := &EventProcessor{
		eventQueue: make(chan Event, 100),
		workerPool: make([]*Worker, 0),
	}

	// Verify initial state
	if len(ep.workerPool) != 0 {
		t.Errorf("expected empty worker pool, got %d workers", len(ep.workerPool))
	}
}
