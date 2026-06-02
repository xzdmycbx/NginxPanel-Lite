package auth

import (
	"sync"
	"time"
)

type rlEntry struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

// Limiter is a simple in-memory failed-attempt limiter keyed by an arbitrary
// string (e.g. "login:<user>:<ip>"). It is not distributed and assumes a single
// panel instance (which is already a deployment constraint).
type Limiter struct {
	mu     sync.Mutex
	m      map[string]*rlEntry
	max    int
	window time.Duration
	lock   time.Duration
}

func NewLimiter(max int, window, lock time.Duration) *Limiter {
	return &Limiter{m: make(map[string]*rlEntry), max: max, window: window, lock: lock}
}

// Allowed reports whether an attempt is currently allowed; if locked, it also
// returns the remaining lock duration.
func (l *Limiter) Allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.m[key]
	if e == nil {
		return true, 0
	}
	if now := time.Now(); now.Before(e.lockedUntil) {
		return false, time.Until(e.lockedUntil)
	}
	return true, 0
}

// Fail records a failed attempt, locking the key after max failures in window.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e := l.m[key]
	if e == nil || now.Sub(e.windowStart) > l.window {
		e = &rlEntry{windowStart: now}
		l.m[key] = e
	}
	e.count++
	if e.count >= l.max {
		e.lockedUntil = now.Add(l.lock)
		e.count = 0
		e.windowStart = now
	}
}

// Reset clears the counter for a key (call after a successful attempt).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, key)
}
