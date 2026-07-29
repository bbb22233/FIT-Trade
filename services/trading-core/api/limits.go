package api

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	at     time.Time
}
type rateLimit struct {
	key    string
	count  int
	period time.Duration
	burst  int
}
type rateBook struct {
	mu      sync.Mutex
	buckets map[string]bucket
	active  map[string]int
	now     func() time.Time
}

func newRateBook() *rateBook {
	return newRateBookWithClock(time.Now)
}

// newRateBookWithClock keeps limit tests deterministic without changing the
// production clock source.
func newRateBookWithClock(now func() time.Time) *rateBook {
	return &rateBook{buckets: make(map[string]bucket), active: make(map[string]int), now: now}
}
func (b *rateBook) allow(key string, count int, period time.Duration, burst int) bool {
	return b.allowAll(rateLimit{key: key, count: count, period: period, burst: burst})
}

// allowAll admits a request only when every bucket can debit one token. It
// calculates and commits all candidate bucket states while holding one mutex,
// so a rejected source bucket cannot consume a session token (or vice versa).
func (b *rateBook) allowAll(limits ...rateLimit) bool {
	if len(limits) == 0 {
		return false
	}
	for _, limit := range limits {
		if limit.key == "" || limit.count < 1 || limit.period <= 0 || limit.burst < 1 {
			return false
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	candidates := make(map[string]bucket, len(limits))
	for _, limit := range limits {
		state, ok := candidates[limit.key]
		if !ok {
			state, ok = b.buckets[limit.key]
			if !ok {
				state = bucket{tokens: float64(limit.burst), at: now}
			} else {
				state.tokens += now.Sub(state.at).Seconds() * float64(limit.count) / limit.period.Seconds()
				if state.tokens > float64(limit.burst) {
					state.tokens = float64(limit.burst)
				}
				state.at = now
			}
		}
		if state.tokens < 1 {
			return false
		}
		state.tokens--
		candidates[limit.key] = state
	}
	for key, state := range candidates {
		b.buckets[key] = state
	}
	return true
}

func (b *rateBook) acquire(key string, maximum int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if maximum < 1 || b.active[key] >= maximum {
		return false
	}
	b.active[key]++
	return true
}

func (b *rateBook) release(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active[key] <= 1 {
		delete(b.active, key)
		return
	}
	b.active[key]--
}
