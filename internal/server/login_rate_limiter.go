package server

import (
	"sync"
	"time"
)

type loginRateLimiter struct {
	mu            sync.Mutex
	entries       map[string]loginRateLimitEntry
	maxFailures   int
	window        time.Duration
	blockedFor    time.Duration
	staleAfter    time.Duration
	opCount       int
	cleanupEveryN int
}

type loginRateLimitEntry struct {
	failures       int
	firstFailureAt time.Time
	blockedUntil   time.Time
	lastSeenAt     time.Time
}

func newLoginRateLimiter(maxFailures int, window, blockedFor time.Duration) *loginRateLimiter {
	if maxFailures <= 0 || window <= 0 || blockedFor <= 0 {
		return nil
	}
	staleAfter := window
	if blockedFor > staleAfter {
		staleAfter = blockedFor
	}
	staleAfter *= 2
	if staleAfter < 10*time.Minute {
		staleAfter = 10 * time.Minute
	}
	return &loginRateLimiter{
		entries:       make(map[string]loginRateLimitEntry),
		maxFailures:   maxFailures,
		window:        window,
		blockedFor:    blockedFor,
		staleAfter:    staleAfter,
		cleanupEveryN: 64,
	}
}

func (l *loginRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil || key == "" {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.entries[key]
	if !entry.blockedUntil.IsZero() && now.Before(entry.blockedUntil) {
		entry.lastSeenAt = now
		l.entries[key] = entry
		l.maybeCleanupLocked(now)
		return false
	}

	if !entry.firstFailureAt.IsZero() && now.Sub(entry.firstFailureAt) > l.window {
		entry.failures = 0
		entry.firstFailureAt = time.Time{}
	}
	if !entry.blockedUntil.IsZero() && !now.Before(entry.blockedUntil) {
		entry.blockedUntil = time.Time{}
	}
	entry.lastSeenAt = now
	l.entries[key] = entry
	l.maybeCleanupLocked(now)

	return true
}

func (l *loginRateLimiter) RegisterFailure(key string, now time.Time) {
	if l == nil || key == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.entries[key]
	if entry.firstFailureAt.IsZero() || now.Sub(entry.firstFailureAt) > l.window {
		entry.failures = 0
		entry.firstFailureAt = now
	}
	entry.failures++
	if entry.failures >= l.maxFailures {
		entry.blockedUntil = now.Add(l.blockedFor)
		entry.failures = 0
		entry.firstFailureAt = time.Time{}
	}
	entry.lastSeenAt = now
	l.entries[key] = entry
	l.maybeCleanupLocked(now)
}

func (l *loginRateLimiter) Reset(key string) {
	if l == nil || key == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *loginRateLimiter) maybeCleanupLocked(now time.Time) {
	l.opCount++
	if l.cleanupEveryN <= 0 {
		l.cleanupEveryN = 64
	}
	if l.opCount%l.cleanupEveryN != 0 {
		return
	}
	for key, entry := range l.entries {
		if entry.lastSeenAt.IsZero() {
			delete(l.entries, key)
			continue
		}
		if now.Sub(entry.lastSeenAt) > l.staleAfter {
			delete(l.entries, key)
		}
	}
}
