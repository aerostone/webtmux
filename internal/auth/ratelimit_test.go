package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute, time.Minute)
	if !rl.Allow("1.2.3.4") {
		t.Error("should allow first attempt")
	}
}

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		rl.RecordFailure("1.2.3.4")
	}
	if rl.Allow("1.2.3.4") {
		t.Error("should block after limit reached")
	}
}

func TestRateLimiterAllowsDifferentKeys(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute, time.Minute)
	rl.RecordFailure("1.2.3.4")
	if !rl.Allow("5.6.7.8") {
		t.Error("different key should be allowed")
	}
}

func TestRateLimiterReset(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute, time.Minute)
	rl.RecordFailure("1.2.3.4")
	rl.Reset("1.2.3.4")
	if !rl.Allow("1.2.3.4") {
		t.Error("should allow after reset")
	}
}

func TestRateLimiterKeyFromRequest(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute, time.Minute)
	key := rl.KeyFromRequest("192.168.1.100:12345")
	if key != "192.168.1.100" {
		t.Errorf("expected IP, got %q", key)
	}
}
