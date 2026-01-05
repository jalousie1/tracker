package discord

import (
	"sync"
	"time"
)

// CircuitBreaker implements a simple circuit breaker pattern for Discord API calls.
// It prevents cascading failures by temporarily disabling requests when errors exceed threshold.
type CircuitBreaker struct {
	mu sync.RWMutex

	// Configuration
	failureThreshold int           // Number of failures before opening circuit
	resetTimeout     time.Duration // Time to wait before trying again
	halfOpenMax      int           // Max requests to allow in half-open state

	// State
	failures      int       // Current consecutive failures
	lastFailure   time.Time // Time of last failure
	state         CBState   // Current state
	halfOpenCount int       // Current requests in half-open
}

// CBState represents the state of the circuit breaker
type CBState int

const (
	CBClosed   CBState = iota // Normal operation
	CBOpen                    // Circuit is open, rejecting requests
	CBHalfOpen                // Testing if service recovered
)

// NewCircuitBreaker creates a new circuit breaker with sensible defaults for Discord API.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: 5,                // Open after 5 consecutive failures
		resetTimeout:     30 * time.Second, // Wait 30s before trying again
		halfOpenMax:      2,                // Allow 2 test requests in half-open
		state:            CBClosed,
	}
}

// NewCircuitBreakerWithConfig creates a circuit breaker with custom configuration.
func NewCircuitBreakerWithConfig(failureThreshold int, resetTimeout time.Duration, halfOpenMax int) *CircuitBreaker {
	if failureThreshold < 1 {
		failureThreshold = 5
	}
	if resetTimeout < time.Second {
		resetTimeout = 30 * time.Second
	}
	if halfOpenMax < 1 {
		halfOpenMax = 2
	}
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		halfOpenMax:      halfOpenMax,
		state:            CBClosed,
	}
}

// Allow returns true if the request should be allowed to proceed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CBClosed:
		return true

	case CBOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = CBHalfOpen
			cb.halfOpenCount = 0
			return true
		}
		return false

	case CBHalfOpen:
		// Allow limited requests to test recovery
		if cb.halfOpenCount < cb.halfOpenMax {
			cb.halfOpenCount++
			return true
		}
		return false
	}

	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	if cb.state == CBHalfOpen {
		cb.state = CBClosed
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.failureThreshold {
		cb.state = CBOpen
	}

	// If we were half-open and got a failure, go back to open
	if cb.state == CBHalfOpen {
		cb.state = CBOpen
		cb.halfOpenCount = 0
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CBState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// StateString returns the current state as a string (for logging/debugging).
func (cb *CircuitBreaker) StateString() string {
	switch cb.State() {
	case CBClosed:
		return "closed"
	case CBOpen:
		return "open"
	case CBHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Reset forces the circuit breaker back to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CBClosed
	cb.failures = 0
	cb.halfOpenCount = 0
}
