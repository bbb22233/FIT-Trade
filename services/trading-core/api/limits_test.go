package api

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateBookBurstExhaustionAndRefill(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	book := newRateBookWithClock(func() time.Time { return now })

	for range 2 {
		if !book.allow("authenticated-session:one", 2, time.Second, 2) {
			t.Fatal("burst token was denied")
		}
	}
	if book.allow("authenticated-session:one", 2, time.Second, 2) {
		t.Fatal("request beyond burst was allowed")
	}

	now = now.Add(500 * time.Millisecond)
	if !book.allow("authenticated-session:one", 2, time.Second, 2) {
		t.Fatal("one refilled token was denied")
	}
	if book.allow("authenticated-session:one", 2, time.Second, 2) {
		t.Fatal("request exceeded the partial refill")
	}

	now = now.Add(time.Second)
	for range 2 {
		if !book.allow("authenticated-session:one", 2, time.Second, 2) {
			t.Fatal("refilled burst token was denied")
		}
	}
}

func TestRateBookInvalidRateConfigurationFailsClosed(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	book := newRateBookWithClock(func() time.Time { return now })
	cases := []struct {
		name   string
		count  int
		period time.Duration
		burst  int
	}{
		{name: "zero count", count: 0, period: time.Second, burst: 1},
		{name: "negative count", count: -1, period: time.Second, burst: 1},
		{name: "zero period", count: 1, period: 0, burst: 1},
		{name: "negative period", count: 1, period: -time.Second, burst: 1},
		{name: "zero burst", count: 1, period: time.Second, burst: 0},
		{name: "negative burst", count: 1, period: time.Second, burst: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if book.allow(tc.name, tc.count, tc.period, tc.burst) {
				t.Fatal("invalid rate configuration was allowed")
			}
		})
	}
	if len(book.buckets) != 0 {
		t.Fatalf("invalid rate configurations created buckets: %#v", book.buckets)
	}
}

func TestRateBookKeysAndConcurrencySlotsAreIndependent(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	book := newRateBookWithClock(func() time.Time { return now })

	if !book.allow("authenticated-source:one", 1, time.Hour, 1) {
		t.Fatal("initial source token was denied")
	}
	if book.allow("authenticated-source:one", 1, time.Hour, 1) {
		t.Fatal("exhausted source key was allowed")
	}
	if !book.allow("authenticated-source:two", 1, time.Hour, 1) {
		t.Fatal("independent source key was denied")
	}
	if !book.allow("authenticated-session:one", 1, time.Hour, 1) {
		t.Fatal("initial session token was denied")
	}
	if book.allow("authenticated-session:one", 1, time.Hour, 1) {
		t.Fatal("exhausted session key was allowed")
	}
	if !book.allow("authenticated-session:two", 1, time.Hour, 1) {
		t.Fatal("independent session key was denied")
	}

	if !book.acquire("websocket-session:one", 2) || !book.acquire("websocket-session:one", 2) {
		t.Fatal("available concurrency slot was denied")
	}
	if book.acquire("websocket-session:one", 2) {
		t.Fatal("saturated concurrency slot was allowed")
	}
	if !book.acquire("websocket-session:two", 1) {
		t.Fatal("separate concurrency key was denied")
	}
	book.release("websocket-session:one")
	if !book.acquire("websocket-session:one", 2) {
		t.Fatal("released concurrency slot was not reusable")
	}
	for _, maximum := range []int{0, -1} {
		if book.acquire("websocket-session:invalid", maximum) {
			t.Fatalf("invalid maximum %d was allowed", maximum)
		}
	}
}

func TestRateBookAllowAllRejectsWithoutPartialDebit(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	book := newRateBookWithClock(func() time.Time { return now })
	source := rateLimit{key: "authenticated-source:source-a", count: 1, period: time.Hour, burst: 1}
	session := rateLimit{key: "authenticated-session:one", count: 1, period: time.Hour, burst: 1}
	if !book.allowAll(source) {
		t.Fatal("initial source admission was denied")
	}
	if book.allowAll(session, source) {
		t.Fatal("source-exhausted pair was admitted")
	}
	if !book.allowAll(session) {
		t.Fatal("rejected pair consumed the session bucket")
	}
}

func TestRateBookAllowAllConcurrentAdmissionIsAtomic(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	book := newRateBookWithClock(func() time.Time { return now })
	source := rateLimit{key: "authenticated-source:shared", count: 1, period: time.Hour, burst: 1}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session := rateLimit{key: fmt.Sprintf("authenticated-session:%d", i), count: 1, period: time.Hour, burst: 1}
			if book.allowAll(session, source) {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("accepted admissions = %d, want 1", got)
	}
	book.mu.Lock()
	defer book.mu.Unlock()
	if len(book.buckets) != 2 {
		t.Fatalf("partial admissions created buckets: %#v", book.buckets)
	}
}
