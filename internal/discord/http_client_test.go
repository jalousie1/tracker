package discord

import (
	"testing"
	"time"
)

func TestCalculateBackoff_RespectsRetryAfter(t *testing.T) {
	cfg := DefaultRetryConfig()

	retryAfter := 5 * time.Second
	backoff := CalculateBackoff(cfg, 0, retryAfter)

	// Should use Retry-After + 500ms padding
	expected := 5*time.Second + 500*time.Millisecond
	if backoff != expected {
		t.Errorf("expected backoff %v, got %v", expected, backoff)
	}
}

func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         false,
	}

	// Attempt 0: 1s
	b0 := CalculateBackoff(cfg, 0, 0)
	if b0 != 1*time.Second {
		t.Errorf("attempt 0: expected 1s, got %v", b0)
	}

	// Attempt 1: 2s
	b1 := CalculateBackoff(cfg, 1, 0)
	if b1 != 2*time.Second {
		t.Errorf("attempt 1: expected 2s, got %v", b1)
	}

	// Attempt 2: 4s
	b2 := CalculateBackoff(cfg, 2, 0)
	if b2 != 4*time.Second {
		t.Errorf("attempt 2: expected 4s, got %v", b2)
	}
}

func TestCalculateBackoff_RespectsMaxBackoff(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     10,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     5 * time.Second,
		Multiplier:     2.0,
		Jitter:         false,
	}

	// Attempt 10: would be 1024s without cap
	b := CalculateBackoff(cfg, 10, 0)
	if b > 5*time.Second {
		t.Errorf("expected backoff to be capped at 5s, got %v", b)
	}
}

func TestCalculateBackoff_WithJitter(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         true,
	}

	// With jitter, backoff should be >= base but < base + 25%
	base := 1 * time.Second
	b := CalculateBackoff(cfg, 0, 0)

	if b < base {
		t.Errorf("expected backoff >= %v, got %v", base, b)
	}

	maxWithJitter := base + base/4
	if b > maxWithJitter {
		t.Errorf("expected backoff <= %v with jitter, got %v", maxWithJitter, b)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}

	if cfg.InitialBackoff != 1*time.Second {
		t.Errorf("expected InitialBackoff 1s, got %v", cfg.InitialBackoff)
	}

	if cfg.MaxBackoff != 30*time.Second {
		t.Errorf("expected MaxBackoff 30s, got %v", cfg.MaxBackoff)
	}

	if cfg.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %f", cfg.Multiplier)
	}

	if !cfg.Jitter {
		t.Error("expected Jitter to be true by default")
	}
}
