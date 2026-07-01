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
	blocked  bool
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
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.attempts[key]
	if !ok {
		return true
	}

	now := time.Now()
	if e.blocked && now.Before(e.blockEnd) {
		return false
	}
	if e.blocked && !now.Before(e.blockEnd) {
		delete(rl.attempts, key)
		return true
	}
	if now.Sub(e.firstAt) > rl.window {
		delete(rl.attempts, key)
		return true
	}

	return e.count < rl.limit
}

func (rl *RateLimiter) RecordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	e, ok := rl.attempts[key]
	if !ok {
		rl.attempts[key] = &attemptEntry{count: 1, firstAt: now}
		return
	}

	if now.Sub(e.firstAt) > rl.window {
		rl.attempts[key] = &attemptEntry{count: 1, firstAt: now}
		return
	}

	e.count++
	if e.count >= rl.limit {
		e.blocked = true
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
		for k, e := range rl.attempts {
			if e.blocked && now.After(e.blockEnd) {
				delete(rl.attempts, k)
			} else if !e.blocked && now.Sub(e.firstAt) > rl.window*2 {
				delete(rl.attempts, k)
			}
		}
		rl.mu.Unlock()
	}
}
