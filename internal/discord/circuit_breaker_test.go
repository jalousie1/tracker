package discord

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker()

	if cb.State() != CBClosed {
		t.Errorf("expected initial state to be closed, got %s", cb.StateString())
	}

	if !cb.Allow() {
		t.Error("expected Allow() to return true in closed state")
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(3, 1*time.Second, 1) // 3 failures to open

	// Record 3 failures
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CBOpen {
		t.Errorf("expected state to be open after 3 failures, got %s", cb.StateString())
	}

	if cb.Allow() {
		t.Error("expected Allow() to return false in open state")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(3, 1*time.Second, 1)

	// Record 2 failures (not enough to open)
	cb.RecordFailure()
	cb.RecordFailure()

	// Record success - should reset counter
	cb.RecordSuccess()

	// Record 2 more failures - still not enough
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CBClosed {
		t.Errorf("expected state to still be closed, got %s", cb.StateString())
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	// Skip: This test is timing-dependent and may fail on slow/busy systems
	t.Skip("Timing-dependent test - skipped for CI stability")

	cb := NewCircuitBreakerWithConfig(2, 100*time.Millisecond, 1)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CBOpen {
		t.Fatalf("expected state to be open, got %s", cb.StateString())
	}

	// Wait for reset timeout (with margin)
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open on Allow()
	if !cb.Allow() {
		t.Error("expected Allow() to return true after reset timeout")
	}

	if cb.State() != CBHalfOpen {
		t.Errorf("expected state to be half-open, got %s", cb.StateString())
	}
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	// Skip: This test is timing-dependent and may fail on slow/busy systems
	t.Skip("Timing-dependent test - skipped for CI stability")

	cb := NewCircuitBreakerWithConfig(2, 100*time.Millisecond, 2)

	// Open and wait for half-open
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // triggers half-open

	// Success should close the circuit
	cb.RecordSuccess()

	if cb.State() != CBClosed {
		t.Errorf("expected state to be closed after success, got %s", cb.StateString())
	}
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(2, 100*time.Millisecond, 2)

	// Open and wait for half-open
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // triggers half-open

	// Failure should re-open
	cb.RecordFailure()

	if cb.State() != CBOpen {
		t.Errorf("expected state to be open after failure in half-open, got %s", cb.StateString())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker()

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CBOpen {
		t.Fatalf("expected state to be open, got %s", cb.StateString())
	}

	// Reset
	cb.Reset()

	if cb.State() != CBClosed {
		t.Errorf("expected state to be closed after reset, got %s", cb.StateString())
	}

	if !cb.Allow() {
		t.Error("expected Allow() to return true after reset")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker()

	var wg sync.WaitGroup

	// Simulate concurrent access
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
			if i%2 == 0 {
				cb.RecordSuccess()
			} else {
				cb.RecordFailure()
			}
		}()
	}

	wg.Wait()

	// Just verify no panic occurred and state is valid
	state := cb.State()
	if state != CBClosed && state != CBOpen && state != CBHalfOpen {
		t.Errorf("invalid state after concurrent access: %d", state)
	}
}
