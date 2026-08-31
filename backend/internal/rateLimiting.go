package internal

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mut      sync.Mutex
	r        rate.Limit
	burst    int
}

// NewRateLimiter Create new rate limiter
func NewRateLimiter(reqsPerMin float64, burst int, resetInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		burst:    burst,
		r:        rate.Limit(reqsPerMin / 60),
	}

	go rl.resetLoop(resetInterval)

	return rl
}

// NewDailyRateLimiter Creates a new daily Limiter
func NewDailyRateLimiter(reqsPerDay float64) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		burst:    10,
		r:        rate.Limit(reqsPerDay / 86400),
	}

	go rl.resetLoop(24 * time.Hour)

	return rl
}

// Limit Check if request should be allowed or not
func (rl *RateLimiter) Limit(ident string) bool {
	rl.mut.Lock()
	limiter, ok := rl.limiters[ident]
	if !ok {
		limiter = rate.NewLimiter(rl.r, rl.burst)
		rl.limiters[ident] = limiter
	}
	rl.mut.Unlock()

	return limiter.Allow()
}

// Reset limiter to avoid extra memory usage
func (rl *RateLimiter) resetLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mut.Lock()
		rl.limiters = make(map[string]*rate.Limiter)
		rl.mut.Unlock()
	}
}
