package ratelimit

import (
	"sync"
	"time"
)

// Limiter tracks failed attempts per key (typically IP address).
type Limiter struct {
	mu       sync.Mutex
	attempts map[string]*entry
	max      int
	window   time.Duration
}

type entry struct {
	count    int
	firstAt  time.Time
}

// New creates a rate limiter that allows max attempts per window.
func New(max int, window time.Duration) *Limiter {
	l := &Limiter{
		attempts: make(map[string]*entry),
		max:      max,
		window:   window,
	}
	// Cleanup stale entries every window period
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			l.cleanup()
		}
	}()
	return l
}

// Allow returns true if the key has not exceeded the rate limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.attempts[key]
	if !ok {
		return true
	}
	if time.Since(e.firstAt) > l.window {
		delete(l.attempts, key)
		return true
	}
	return e.count < l.max
}

// Record records a failed attempt for the given key.
func (l *Limiter) Record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.attempts[key]
	if !ok || time.Since(e.firstAt) > l.window {
		l.attempts[key] = &entry{count: 1, firstAt: time.Now()}
		return
	}
	e.count++
}

// Reset clears the attempts for the given key (e.g. on successful login).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for key, e := range l.attempts {
		if now.Sub(e.firstAt) > l.window {
			delete(l.attempts, key)
		}
	}
}
