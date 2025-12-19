package security

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type LimiterStore struct {
	mu       sync.Mutex
	limiters map[string]*clientLimiter
	r        rate.Limit
	b        int
	ttl      time.Duration
}

type clientLimiter struct {
	lim     *rate.Limiter
	lastHit time.Time
}

func NewLimiterStore(r rate.Limit, burst int, ttl time.Duration) *LimiterStore {
	return &LimiterStore{
		limiters: make(map[string]*clientLimiter),
		r:        r,
		b:        burst,
		ttl:      ttl,
	}
}

func (s *LimiterStore) Allow(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// lazy cleanup
	for k, v := range s.limiters {
		if now.Sub(v.lastHit) > s.ttl {
			delete(s.limiters, k)
		}
	}

	cl, ok := s.limiters[ip]
	if !ok {
		cl = &clientLimiter{
			lim:     rate.NewLimiter(s.r, s.b),
			lastHit: now,
		}
		s.limiters[ip] = cl
	}

	cl.lastHit = now
	return cl.lim.Allow()
}

func ClientIPFromRequest(r *http.Request) string {
	// prefer RemoteAddr to avoid trusting spoofable headers by default
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}


