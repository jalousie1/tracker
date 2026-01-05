package discord

import (
	"net"
	"net/http"
	"time"
)

// DiscordHTTPClient is a shared HTTP client configured for optimal Discord API usage.
// It provides connection pooling, proper timeouts, and keepalive for efficiency.
var DiscordHTTPClient = NewDiscordHTTPClient()

// NewDiscordHTTPClient creates a new HTTP client optimized for Discord API calls.
// Features:
// - Connection pooling (max 100 idle connections)
// - Keep-alive enabled
// - Proper timeouts to prevent hanging requests
// - TLS handshake timeout
func NewDiscordHTTPClient() *http.Client {
	transport := &http.Transport{
		// Connection pooling settings
		MaxIdleConns:        100,              // Total max idle connections
		MaxIdleConnsPerHost: 20,               // Max idle connections per host (Discord)
		MaxConnsPerHost:     50,               // Max total connections per host
		IdleConnTimeout:     90 * time.Second, // How long idle connections stay in pool

		// Dial settings
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second, // Connection timeout
			KeepAlive: 30 * time.Second, // TCP keep-alive interval
		}).DialContext,

		// TLS settings
		TLSHandshakeTimeout: 10 * time.Second,

		// Response settings
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// Forces HTTP/2 when available
		ForceAttemptHTTP2: true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Overall request timeout
	}
}

// RetryConfig holds configuration for exponential backoff retries.
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         bool // Add random jitter to prevent thundering herd
}

// DefaultRetryConfig returns sensible defaults for Discord API retries.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         true,
	}
}

// CalculateBackoff calculates the next backoff duration for a given attempt.
// Uses exponential backoff: initialBackoff * (multiplier ^ attempt)
// Respects maxBackoff limit and optionally adds jitter.
func CalculateBackoff(cfg RetryConfig, attempt int, retryAfter time.Duration) time.Duration {
	// If Discord sent Retry-After, use it (slightly padded)
	if retryAfter > 0 {
		return retryAfter + 500*time.Millisecond
	}

	// Calculate exponential backoff
	backoff := cfg.InitialBackoff
	for i := 0; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * cfg.Multiplier)
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
			break
		}
	}

	// Add jitter (up to 25% of backoff) to prevent thundering herd
	if cfg.Jitter && backoff > 0 {
		jitterRange := int64(backoff) / 4
		if jitterRange > 0 {
			// Simple deterministic jitter based on attempt number
			jitter := time.Duration((int64(attempt) * 137) % jitterRange)
			backoff += jitter
		}
	}

	return backoff
}
