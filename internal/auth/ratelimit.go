package auth

import (
	"net"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptEntry
	limit    int
	window   time.Duration
	lockout  time.Duration
}

type attemptEntry struct {
	count    int
	firstAt  time.Time
	blockEnd time.Time
}

func NewRateLimiter(limit int, window, lockout time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts: make(map[string]*attemptEntry),
		limit:    limit,
		window:   window,
		lockout:  lockout,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	return rl.RetryAfter(key) == 0
}

// Attempt atomically reserves an authentication attempt before verification.
// Zero means admitted; a positive duration is the remaining lockout.
func (rl *RateLimiter) Attempt(key string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	if retry := rl.retryAfterLocked(key, now); retry > 0 {
		return retry
	}
	rl.recordLocked(key, now)
	return 0
}

func (rl *RateLimiter) RetryAfter(key string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.retryAfterLocked(key, time.Now())
}

func (rl *RateLimiter) retryAfterLocked(key string, now time.Time) time.Duration {
	e, ok := rl.attempts[key]
	if !ok {
		return 0
	}
	if now.Before(e.blockEnd) {
		return e.blockEnd.Sub(now)
	}
	if !e.blockEnd.IsZero() || now.Sub(e.firstAt) >= rl.window {
		delete(rl.attempts, key)
	}
	return 0
}

func (rl *RateLimiter) RecordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if rl.retryAfterLocked(key, now) > 0 {
		return
	}
	rl.recordLocked(key, now)
}

func (rl *RateLimiter) recordLocked(key string, now time.Time) {
	e := rl.attempts[key]
	if e == nil {
		e = &attemptEntry{firstAt: now}
		rl.attempts[key] = e
	}
	e.count++
	if e.count >= rl.limit {
		e.blockEnd = now.Add(rl.lockout)
	}
}

func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

func (rl *RateLimiter) KeyFromRequest(remoteAddr string) string {
	host, _, _ := net.SplitHostPort(remoteAddr)
	if host == "" {
		host = remoteAddr
	}
	return host
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key := range rl.attempts {
			rl.retryAfterLocked(key, now)
		}
		rl.mu.Unlock()
	}
}
