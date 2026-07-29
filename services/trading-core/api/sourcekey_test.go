package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"fit.trade/trading-core/auth"
)

func TestSourceKeyRotationOverlapAndVerification(t *testing.T) {
	rotation := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server := mustServerAt(t, &rotation)
	peer := netip.MustParseAddr("192.0.2.44")
	oldRecord := server.sourceRecord(peer, false)
	if oldRecord.KeyID != "source-key-v1" || !server.VerifySourceKeyRecord(oldRecord, "192.0.2.44:1") {
		t.Fatal("initial source-key record did not verify")
	}
	if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{2}, 32)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://loopback.test/health/live", nil)
	request.RemoteAddr = "127.0.0.1:43210"
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rotated-key HTTP request status = %d, want %d", response.Code, http.StatusOK)
	}
	newRecord := server.sourceRecord(peer, false)
	if newRecord.KeyID != "source-key-v2" || newRecord.SourceKeyDigest == oldRecord.SourceKeyDigest {
		t.Fatalf("rotation record = %#v old = %#v", newRecord, oldRecord)
	}
	if !server.VerifySourceKeyRecord(oldRecord, "192.0.2.44:1") || !server.VerifySourceKeyRecord(newRecord, "192.0.2.44:1") {
		t.Fatal("current or overlap source-key record did not verify")
	}
	if server.VerifySourceKeyRecord(SourceKeyRecord{KeyID: "source-key-v99", SourceKeyDigest: newRecord.SourceKeyDigest}, "192.0.2.44:1") {
		t.Fatal("unknown source-key ID verified")
	}
	if server.VerifySourceKeyRecord(SourceKeyRecord{KeyID: newRecord.KeyID, SourceKeyDigest: "src_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, "192.0.2.44:1") {
		t.Fatal("noncanonical source-key digest verified")
	}
	for _, offset := range []time.Duration{1799 * time.Second, 1800 * time.Second, 1801 * time.Second} {
		rotation = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Add(offset)
		verified := server.VerifySourceKeyRecord(oldRecord, "192.0.2.44:1")
		if verified != (offset == 1799*time.Second) {
			t.Fatalf("prior record verification at +%s = %t", offset, verified)
		}
	}
}

func TestSourceKeyRotationKeepsEverySourceBudgetAndCorrelationDuringOverlap(t *testing.T) {
	rotation := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server := mustServerAt(t, &rotation)
	server.limits = newRateBookWithClock(func() time.Time { return rotation })

	request := httptest.NewRequest(http.MethodGet, "http://loopback.test/health/live", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	before, ok := server.source(request)
	if !ok {
		t.Fatal("initial source was unavailable")
	}
	oldRecord := server.sourceRecord(netip.MustParseAddr("127.0.0.1"), false)
	if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{2}, 32)); err != nil {
		t.Fatal(err)
	}
	rotation = rotation.Add(1799 * time.Second)
	after, ok := server.source(request)
	if !ok || after != before {
		t.Fatalf("overlap correlation = %q, want %q", after, before)
	}
	if !server.VerifySourceKeyRecord(oldRecord, "127.0.0.1:1234") {
		t.Fatal("prior correlation record was rejected at +1799s")
	}

	for _, category := range []string{
		passwordRatePrefix, "health-live:", "health-ready:", "enrollment-completion:",
		"refresh:", "authenticated-source:", "websocket-source-rate:",
	} {
		if !server.limits.allow(category+before, 1, time.Hour, 1) || server.limits.allow(category+after, 1, time.Hour, 1) {
			t.Fatalf("%s budget was reset during source-key overlap", category)
		}
	}
	for _, category := range []string{"websocket-source-concurrent:"} {
		if !server.limits.acquire(category+before, 1) || server.limits.acquire(category+after, 1) {
			t.Fatalf("%s concurrency was reset during source-key overlap", category)
		}
	}

	rotation = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Add(sourceKeyOverlap)
	current, ok := server.source(request)
	if !ok || current == before {
		t.Fatalf("correlation at overlap expiry = %q, want new current source", current)
	}
	if server.VerifySourceKeyRecord(oldRecord, "127.0.0.1:1234") {
		t.Fatal("prior correlation record verified at +1800s")
	}
	rotation = rotation.Add(time.Second)
	if currentAgain, ok := server.source(request); !ok || currentAgain != current {
		t.Fatalf("correlation at +1801s = %q, want %q", currentAgain, current)
	}
}

func TestSourceKeyCorrelationUsesDirectIPv4IPv6AndOnlyCurrentPrior(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server := mustServerAt(t, &now)
	ipv4 := sourceForTest(t, server, "127.0.0.1:1000", "198.51.100.9")
	if forwarded := sourceForTest(t, server, "127.0.0.1:1000", "203.0.113.7"); forwarded != ipv4 {
		t.Fatal("forwarding header changed direct IPv4 source correlation")
	}
	ipv6 := sourceForTest(t, server, "[::1]:1000", "2001:db8::9")
	if ipv4 == ipv6 {
		t.Fatal("IPv4 and IPv6 peers shared a source correlation")
	}
	stale := server.sourceRecord(netip.MustParseAddr("127.0.0.1"), false)
	if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{2}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := server.RotateSourceKey("source-key-v3", bytes.Repeat([]byte{3}, 32)); err == nil {
		t.Fatal("rotation displaced an active prior key")
	}
	if !server.VerifySourceKeyRecord(stale, "127.0.0.1:1000") {
		t.Fatal("prior source-key ID was rejected during its overlap")
	}
	now = now.Add(sourceKeyOverlap)
	if err := server.RotateSourceKey("source-key-v3", bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	if server.VerifySourceKeyRecord(stale, "127.0.0.1:1000") {
		t.Fatal("expired source-key ID verified after advancing rotation")
	}
	if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{4}, 32)); err == nil {
		t.Fatal("non-advancing source-key ID accepted")
	}
}

func mustServerAt(t *testing.T, now *time.Time) *Server {
	t.Helper()
	server, err := NewLoopbackServer(auth.NewService(auth.Config{}), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	server.now = func() time.Time { return *now }
	return server
}

func sourceForTest(t *testing.T, server *Server, remoteAddr, forwarded string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://loopback.test/health/live", nil)
	request.RemoteAddr = remoteAddr
	request.Header.Set("X-Forwarded-For", forwarded)
	source, ok := server.source(request)
	if !ok {
		t.Fatalf("source unavailable for %s", remoteAddr)
	}
	return source
}

func TestSourceKeyRotationRejectsInvalidIDsAndConcurrentVerification(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server := mustServerAt(t, &now)
	if err := server.RotateSourceKey("source-key-vx", bytes.Repeat([]byte{4}, 32)); err == nil {
		t.Fatal("invalid source-key ID accepted")
	}
	if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{4}, 15)); err == nil {
		t.Fatal("short source key accepted")
	}
	peer := netip.MustParseAddr("2001:db8::4")
	var rotations sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		rotations.Add(1)
		go func() {
			defer rotations.Done()
			errs <- server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{2}, 32))
		}()
	}
	for range 200 {
		record := server.sourceRecord(peer, false)
		_ = server.VerifySourceKeyRecord(record, "[2001:db8::4]:1")
	}
	rotations.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent v2 rotations succeeded %d times, want 1", successes)
	}
	if record := server.sourceRecord(peer, false); record.KeyID != "source-key-v2" {
		t.Fatalf("concurrent rotation current key = %q, want source-key-v2", record.KeyID)
	}
}

func TestSourceKeyRotationOverlapRejectsRapidRepeatWithoutResettingBudget(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{1799 * time.Second, 1800 * time.Second, 1801 * time.Second} {
		t.Run(offset.String(), func(t *testing.T) {
			now := base
			server := mustServerAt(t, &now)
			server.limits = newRateBookWithClock(func() time.Time { return now })
			request := httptest.NewRequest(http.MethodGet, "http://loopback.test/health/live", nil)
			request.RemoteAddr = "127.0.0.1:43123"
			original, ok := server.source(request)
			if !ok || !server.limits.allow(passwordRatePrefix+original, 1, time.Hour, 1) {
				t.Fatal("initial source budget was not debited")
			}
			if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{2}, 32)); err != nil {
				t.Fatal(err)
			}
			now = base.Add(offset)
			before, ok := server.source(request)
			if !ok {
				t.Fatal("source unavailable before advancing rotation")
			}
			if offset >= sourceKeyOverlap && !server.limits.allow(passwordRatePrefix+before, 1, time.Hour, 1) {
				t.Fatal("current source budget was not debited before advancing rotation")
			}
			err := server.RotateSourceKey("source-key-v3", bytes.Repeat([]byte{3}, 32))
			if offset == 1799*time.Second {
				if err == nil {
					t.Fatal("rapid rotation succeeded during active overlap")
				}
				if got, _ := server.source(request); got != original || server.limits.allow(passwordRatePrefix+got, 1, time.Hour, 1) {
					t.Fatal("rejected rapid rotation reset source correlation or budget")
				}
				return
			}
			if err != nil {
				t.Fatalf("rotation at +%s failed: %v", offset, err)
			}
			after, ok := server.source(request)
			if !ok || after != before || server.limits.allow(passwordRatePrefix+after, 1, time.Hour, 1) {
				t.Fatalf("rotation at +%s changed active correlation or reset budget", offset)
			}
		})
	}
}
