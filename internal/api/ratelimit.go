package api

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a simple token-bucket rate limiter.
// Designed for local API protection — limits per-client request rate.
type rateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	max      float64 // burst capacity
	rate     float64 // tokens per second (sustained rate)
	lastTime time.Time
}

func newRateLimiter(burstSize int, sustainedPerSec float64) *rateLimiter {
	return &rateLimiter{
		tokens:   float64(burstSize),
		max:      float64(burstSize),
		rate:     sustainedPerSec,
		lastTime: time.Now(),
	}
}

// allow checks if a request is allowed under the rate limit.
func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now

	// Replenish tokens
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.max {
		rl.tokens = rl.max
	}

	if rl.tokens < 1.0 {
		return false
	}

	rl.tokens--
	return true
}

// rateLimitMiddleware wraps an HTTP handler with rate limiting.
func rateLimitMiddleware(limiter *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow() {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
