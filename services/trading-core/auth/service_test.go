package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"fit.trade/trading-core/session"
)

var testReducedArgon2Profile = session.Argon2Profile{
	Version: "argon2id-test-only-v1", MemoryKiB: 64, Iterations: 1,
	Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
}

const revocationSchedulerTolerance = 200 * time.Millisecond

// newTestService is package-private and is compiled only into auth package
// tests. Production callers cannot select this reduced verifier profile.
func newTestService(config Config) *Service {
	service := NewService(config)
	service.verifierProfile = testReducedArgon2Profile
	return service
}

func TestRefreshReuseRevokesAllDescendantsAndReplacementRevokesPriorDevices(t *testing.T) {
	clock := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	service := newTestService(Config{Now: func() time.Time { return clock }})
	first, err := service.addSyntheticUser("one@example.test", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "one@example.test", "test-password", "source-a")
	if err != nil {
		t.Fatal(err)
	}
	originalRefresh := credentials.RefreshToken
	for i := 0; i < 4; i++ {
		clock = clock.Add(time.Second)
		credentials, err = service.Refresh(context.Background(), credentials.RefreshToken)
		if err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
	}
	if _, err := service.Refresh(context.Background(), originalRefresh); err != ErrRefreshReuse {
		t.Fatalf("reuse error = %v", err)
	}
	if _, err := service.Refresh(context.Background(), credentials.RefreshToken); err == nil {
		t.Fatal("descendant survived refresh reuse")
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := fingerprint(public)
	challenge, err := service.StartEnrollment(context.Background(), "one@example.test", "test-password", "REPLACEMENT_DEVICE", fingerprint, "source-b")
	if err != nil {
		t.Fatal(err)
	}
	message, err := signedMessage(enrollmentDomain, challenge)
	if err != nil {
		t.Fatal(err)
	}
	signature := "ed25519-signature:" + hex.EncodeToString(ed25519.Sign(private, message))
	current, err := service.CompleteEnrollment(context.Background(), challenge.SubjectHandle, "ed25519-public:"+hex.EncodeToString(public), signature)
	if err != nil {
		t.Fatal(err)
	}
	if current.DeviceID == first.DeviceID {
		t.Fatal("client-selected or previous device reused")
	}
	if _, err := service.Authenticate(credentials.AccessToken); err == nil {
		t.Fatal("replacement did not revoke prior session")
	}
	if _, err := service.CompleteEnrollment(context.Background(), challenge.SubjectHandle, "ed25519-public:"+hex.EncodeToString(public), signature); err != ErrReplay {
		t.Fatalf("challenge replay = %v", err)
	}

	second, err := service.addSyntheticUser("two@example.test", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Login(context.Background(), "two@example.test", "test-password", "source-c")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeDevice(other.Identity, current.DeviceID); err == nil {
		t.Fatal("cross-user device revoke accepted")
	}
	_ = second
}

func TestSessionInvalidationObserverCompletesBeforeRevocationReturns(t *testing.T) {
	service := newTestService(Config{})
	if _, err := service.addSyntheticUser("fence@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "fence@example.test", "test-password", "test-source")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan string, 1)
	release := make(chan struct{})
	remove := service.ObserveSessionInvalidation(func(_ context.Context, sessionID string) error {
		entered <- sessionID
		<-release
		return nil
	})
	defer remove()
	revoked := make(chan error, 1)
	go func() { revoked <- service.RevokeSession(credentials.Identity, credentials.Identity.SessionID) }()
	select {
	case sessionID := <-entered:
		if sessionID != credentials.Identity.SessionID {
			t.Fatalf("fenced session = %q, want %q", sessionID, credentials.Identity.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("session invalidation observer was not called")
	}
	select {
	case err := <-revoked:
		t.Fatalf("revocation returned before its websocket fence completed: %v", err)
	default:
	}
	if service.Active(credentials.Identity) {
		t.Fatal("blocking observer kept revoked identity active")
	}
	if _, err := service.Authenticate(credentials.AccessToken); !errors.Is(err, ErrDenied) {
		t.Fatalf("blocking observer delayed authentication revocation: %v", err)
	}
	close(release)
	select {
	case err := <-revoked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not complete after fencing")
	}
	if service.Active(credentials.Identity) {
		t.Fatal("revoked identity remained active")
	}
}

func TestSessionInvalidationObserverFailureAndDeadlineFailClosed(t *testing.T) {
	t.Run("early error waits for blocking peer", func(t *testing.T) {
		service := newTestService(Config{})
		if _, err := service.addSyntheticUser("observer-peer@example.test", "test-password"); err != nil {
			t.Fatal(err)
		}
		credentials, err := service.Login(context.Background(), "observer-peer@example.test", "test-password", "test-source")
		if err != nil {
			t.Fatal(err)
		}
		started := make(chan struct{}, 1)
		release := make(chan struct{})
		service.ObserveSessionInvalidation(func(context.Context, string) error { return errors.New("early fence error") })
		service.ObserveSessionInvalidation(func(ctx context.Context, _ string) error {
			started <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		result := make(chan error, 1)
		go func() { result <- service.RevokeSession(credentials.Identity, credentials.Identity.SessionID) }()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("blocking observer did not start")
		}
		select {
		case err := <-result:
			t.Fatalf("revocation returned before required peer completed: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		if err := <-result; !errors.Is(err, ErrRevocationFence) {
			t.Fatalf("revocation error = %v, want ErrRevocationFence", err)
		}
	})

	t.Run("callback error", func(t *testing.T) {
		service := newTestService(Config{})
		if _, err := service.addSyntheticUser("observer-error@example.test", "test-password"); err != nil {
			t.Fatal(err)
		}
		credentials, err := service.Login(context.Background(), "observer-error@example.test", "test-password", "test-source")
		if err != nil {
			t.Fatal(err)
		}
		service.ObserveSessionInvalidation(func(context.Context, string) error { return errors.New("fence unavailable") })
		if err := service.RevokeSession(credentials.Identity, credentials.Identity.SessionID); !errors.Is(err, ErrRevocationFence) {
			t.Fatalf("revocation error = %v, want ErrRevocationFence", err)
		}
		if service.Active(credentials.Identity) {
			t.Fatal("fence error rolled back revoked state")
		}
	})

	t.Run("deadline", func(t *testing.T) {
		service := newTestService(Config{})
		if _, err := service.addSyntheticUser("observer-deadline@example.test", "test-password"); err != nil {
			t.Fatal(err)
		}
		credentials, err := service.Login(context.Background(), "observer-deadline@example.test", "test-password", "test-source")
		if err != nil {
			t.Fatal(err)
		}
		service.ObserveSessionInvalidation(func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		})
		started := time.Now()
		if err := service.RevokeSession(credentials.Identity, credentials.Identity.SessionID); !errors.Is(err, ErrRevocationFence) {
			t.Fatalf("revocation error = %v, want ErrRevocationFence", err)
		}
		elapsed := time.Since(started)
		if elapsed < session.RevocationBound-revocationSchedulerTolerance || elapsed > session.RevocationBound+revocationSchedulerTolerance {
			t.Fatalf("observer deadline = %s, want bounded at %s", elapsed, session.RevocationBound)
		}
	})
}

func TestDeviceRevocationAttemptsEveryCapturedSessionAfterObserverFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(context.Context, string, string) error
	}{
		{
			name: "first callback error",
			call: func(_ context.Context, sessionID, firstSessionID string) error {
				if sessionID == firstSessionID {
					return errors.New("first fence failure")
				}
				return nil
			},
		},
		{
			name: "per-session panic",
			call: func(_ context.Context, sessionID, firstSessionID string) error {
				if sessionID == firstSessionID {
					panic("first fence panic")
				}
				return nil
			},
		},
		{
			name: "callback waits for cancellation",
			call: func(ctx context.Context, _, _ string) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newTestService(Config{})
			if _, err := service.addSyntheticUser("device-fanout@example.test", "test-password"); err != nil {
				t.Fatal(err)
			}
			first, err := service.Login(context.Background(), "device-fanout@example.test", "test-password", "source-a")
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.Login(context.Background(), "device-fanout@example.test", "test-password", "source-b")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]int{first.Identity.SessionID: 1, second.Identity.SessionID: 1}
			var callsMu sync.Mutex
			calls := make(map[string]int, len(want))
			service.ObserveSessionInvalidation(func(ctx context.Context, sessionID string) error {
				callsMu.Lock()
				calls[sessionID]++
				callsMu.Unlock()
				return test.call(ctx, sessionID, first.Identity.SessionID)
			})
			started := time.Now()
			err = service.RevokeDevice(first.Identity, first.Identity.DeviceID)
			if !errors.Is(err, ErrRevocationFence) {
				t.Fatalf("revocation error = %v, want ErrRevocationFence", err)
			}
			if test.name == "callback waits for cancellation" && time.Since(started) > session.RevocationBound+revocationSchedulerTolerance {
				t.Fatalf("deadline fan-out exceeded bound: %s", time.Since(started))
			}
			callsMu.Lock()
			got := make(map[string]int, len(calls))
			for sessionID, count := range calls {
				got[sessionID] = count
			}
			callsMu.Unlock()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("observer calls = %v, want every frozen session exactly once %v", got, want)
			}
		})
	}
}

func TestConcurrentInvalidationStartsDistinctObserverSessions(t *testing.T) {
	service := newTestService(Config{})
	if _, err := service.addSyntheticUser("coalesce-one@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.addSyntheticUser("coalesce-two@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	first, err := service.Login(context.Background(), "coalesce-one@example.test", "test-password", "source-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(context.Background(), "coalesce-two@example.test", "test-password", "source-b")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan string, 2)
	release := make(chan struct{})
	var callsMu sync.Mutex
	calls := make(map[string]int)
	service.ObserveSessionInvalidation(func(ctx context.Context, sessionID string) error {
		callsMu.Lock()
		calls[sessionID]++
		callsMu.Unlock()
		entered <- sessionID
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	results := make(chan error, 2)
	go func() { results <- service.RevokeSession(first.Identity, first.Identity.SessionID) }()
	select {
	case got := <-entered:
		if got != first.Identity.SessionID {
			t.Fatalf("first callback = %q, want %q", got, first.Identity.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("first invalidation observer did not start")
	}
	go func() { results <- service.RevokeSession(second.Identity, second.Identity.SessionID) }()
	select {
	case got := <-entered:
		if got != second.Identity.SessionID {
			t.Fatalf("second callback = %q, want %q", got, second.Identity.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked first callback prevented second session from entering")
	}
	close(release)
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("concurrent revocation error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent revocations did not complete")
		}
	}
	callsMu.Lock()
	got := make(map[string]int, len(calls))
	for sessionID, count := range calls {
		got[sessionID] = count
	}
	callsMu.Unlock()
	want := map[string]int{first.Identity.SessionID: 1, second.Identity.SessionID: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observer calls = %v, want exactly once per session %v", got, want)
	}
}

func TestInvalidationObserverPerSessionAttempts(t *testing.T) {
	t.Run("same session attaches and cleans up", func(t *testing.T) {
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		var callsMu sync.Mutex
		calls := 0
		observer := &invalidationObserver{callback: func(context.Context, string) error {
			callsMu.Lock()
			calls++
			callsMu.Unlock()
			entered <- struct{}{}
			<-release
			return nil
		}}
		results := make(chan error, 2)
		go func() { results <- observer.run(context.Background(), []string{"session-a", "session-a"}) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("first callback did not enter")
		}
		go func() { results <- observer.run(context.Background(), []string{"session-a"}) }()

		callsMu.Lock()
		gotCalls := calls
		callsMu.Unlock()
		if gotCalls != 1 {
			t.Fatalf("same-session callbacks = %d, want 1", gotCalls)
		}
		observer.mu.Lock()
		inFlight := len(observer.inFlight)
		observer.mu.Unlock()
		if inFlight != 1 {
			t.Fatalf("in-flight attempts = %d, want 1", inFlight)
		}

		close(release)
		for range 2 {
			select {
			case err := <-results:
				if err != nil {
					t.Fatalf("attached run = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("attached runs did not complete")
			}
		}
		observer.mu.Lock()
		inFlight = len(observer.inFlight)
		observer.mu.Unlock()
		if inFlight != 0 {
			t.Fatalf("completed attempt remained in flight: %d", inFlight)
		}
	})

	t.Run("blocked session does not delay another", func(t *testing.T) {
		blockedEntered := make(chan struct{}, 1)
		otherEntered := make(chan struct{}, 1)
		release := make(chan struct{})
		observer := &invalidationObserver{callback: func(_ context.Context, sessionID string) error {
			if sessionID == "blocked" {
				blockedEntered <- struct{}{}
				<-release
				return nil
			}
			otherEntered <- struct{}{}
			return nil
		}}
		blockedResult := make(chan error, 1)
		go func() { blockedResult <- observer.run(context.Background(), []string{"blocked"}) }()
		select {
		case <-blockedEntered:
		case <-time.After(time.Second):
			t.Fatal("blocked callback did not enter")
		}
		if err := observer.run(context.Background(), []string{"other"}); err != nil {
			t.Fatalf("independent session run = %v", err)
		}
		select {
		case <-otherEntered:
		case <-time.After(time.Second):
			t.Fatal("different session did not enter while blocked callback remained in flight")
		}
		close(release)
		select {
		case err := <-blockedResult:
			if err != nil {
				t.Fatalf("blocked session run = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("blocked session run did not complete")
		}
		observer.mu.Lock()
		inFlight := len(observer.inFlight)
		observer.mu.Unlock()
		if inFlight != 0 {
			t.Fatalf("completed attempts remained in flight: %d", inFlight)
		}
	})
}

func TestConcurrentRevocationFencesFailClosedWithoutDeadlock(t *testing.T) {
	service := newTestService(Config{})
	if _, err := service.addSyntheticUser("concurrent-revoke@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	first, err := service.Login(context.Background(), "concurrent-revoke@example.test", "test-password", "source-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(context.Background(), "concurrent-revoke@example.test", "test-password", "source-b")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	service.ObserveSessionInvalidation(func(ctx context.Context, _ string) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	results := make(chan error, 2)
	go func() { results <- service.RevokeSession(first.Identity, first.Identity.SessionID) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first observer did not start")
	}
	go func() { results <- service.RevokeSession(second.Identity, second.Identity.SessionID) }()
	close(release)
	for range 2 {
		select {
		case err := <-results:
			if err != nil && !errors.Is(err, ErrRevocationFence) {
				t.Fatalf("concurrent revocation error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent revocation deadlocked")
		}
	}
	if service.Active(first.Identity) || service.Active(second.Identity) {
		t.Fatal("concurrent revoke left an active identity")
	}
}

func TestSessionInvalidationSnapshotsConcurrentObservers(t *testing.T) {
	service := newTestService(Config{})
	if _, err := service.addSyntheticUser("observers@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "observers@example.test", "test-password", "test-source")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	seen := make(map[string]int)
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	for range 8 {
		service.ObserveSessionInvalidation(func(ctx context.Context, sessionID string) error {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			mu.Lock()
			seen[sessionID]++
			mu.Unlock()
			return nil
		})
	}
	revoked := make(chan error, 1)
	go func() { revoked <- service.RevokeSession(credentials.Identity, credentials.Identity.SessionID) }()
	for range 8 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("observers were not invoked concurrently")
		}
	}
	if service.Active(credentials.Identity) {
		t.Fatal("concurrent observers delayed the state transition")
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := seen[credentials.Identity.SessionID]
	mu.Unlock()
	if got != 8 {
		t.Fatalf("observer callbacks = %d, want 8", got)
	}
}

func TestEnrollmentSignatureUsesFrozenWireObject(t *testing.T) {
	clock := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	service := newTestService(Config{Now: func() time.Time { return clock }})
	if _, err := service.addSyntheticUser("wire@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := service.StartEnrollment(context.Background(), "wire@example.test", "test-password", "ADDITIONAL_DEVICE", fingerprint(public), "source")
	if err != nil {
		t.Fatal(err)
	}
	legacyWire, err := json.Marshal(challenge)
	if err != nil {
		t.Fatal(err)
	}
	legacy := ed25519.Sign(private, append(append([]byte(enrollmentDomain), 0), legacyWire...))
	if _, err := service.CompleteEnrollment(context.Background(), challenge.SubjectHandle, "ed25519-public:"+hex.EncodeToString(public), "ed25519-signature:"+hex.EncodeToString(legacy)); err != ErrInvalidProof {
		t.Fatalf("non-JCS device proof = %v", err)
	}
	challenge, err = service.StartEnrollment(context.Background(), "wire@example.test", "test-password", "ADDITIONAL_DEVICE", fingerprint(public), "source")
	if err != nil {
		t.Fatal(err)
	}
	wire, err := signedMessage(enrollmentDomain, challenge)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) == 0 || string(wire[:len(enrollmentDomain)+1]) != enrollmentDomain+"\x00" {
		t.Fatal("device proof did not preserve domain separation")
	}
	signature := ed25519.Sign(private, wire)
	if _, err := service.CompleteEnrollment(context.Background(), challenge.SubjectHandle, "ed25519-public:"+hex.EncodeToString(public), "ed25519-signature:"+hex.EncodeToString(signature)); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceActionUsesFrozenEnumAndDigest(t *testing.T) {
	service := newTestService(Config{})
	if _, err := service.addSyntheticUser("action-schema@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "action-schema@example.test", "test-password", "source")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ action, digest string }{
		{"", strings.Repeat("a", 64)},
		{"CLOSE_POSITION", strings.Repeat("a", 64)},
		{"OPEN_POSITION", strings.Repeat("A", 64)},
		{"OPEN_POSITION", strings.Repeat("z", 64)},
		{"OPEN_POSITION", strings.Repeat("a", 63)},
	} {
		if _, err := service.IssueDeviceAction(credentials.AccessToken, input.action, input.digest); !errors.Is(err, ErrDenied) {
			t.Fatalf("IssueDeviceAction(%q, %q) = %v, want ErrDenied", input.action, input.digest, err)
		}
	}
	challenge, err := service.IssueDeviceAction(credentials.AccessToken, "ADD_POSITION", strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if challenge.SchemaVersion != "fit.platform.device-action-challenge.v1" || challenge.Domain != actionDomain || challenge.Action != "ADD_POSITION" || challenge.PayloadHash != strings.Repeat("b", 64) {
		t.Fatalf("frozen device-action challenge drift: %#v", challenge)
	}
}

func TestArgon2ProfilesAndLockIntents(t *testing.T) {
	production := session.ProductionArgon2Profile()
	if production.Version != "argon2id-v1" || production.MemoryKiB < 64*1024 || production.Iterations < 3 || production.Parallelism != 1 || production.SaltBytes != 16 || production.KeyBytes != 32 {
		t.Fatalf("production Argon2id profile drift: %#v", production)
	}
	clock := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	sink := NewMemoryIntentSink()
	if profile := NewService(Config{}).Argon2Profile(); profile != production {
		t.Fatalf("production constructor selected non-frozen profile: %#v", profile)
	}
	service := newTestService(Config{Now: func() time.Time { return clock }, IntentSink: sink})
	if profile := service.Argon2Profile(); profile.Version != "argon2id-test-only-v1" || profile.MemoryKiB >= production.MemoryKiB {
		t.Fatalf("test Argon2id profile not visibly reduced: %#v", profile)
	}
	if _, err := service.addSyntheticUser("locked@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := service.Login(context.Background(), "locked@example.test", "wrong-password", "pseudonymous-source"); err != ErrDenied {
			t.Fatalf("failure %d = %v", attempt+1, err)
		}
		clock = clock.Add(time.Duration(1<<attempt) * time.Second)
	}
	audits, notifications := sink.Audits(), sink.Notifications()
	if len(audits) != 2 || len(notifications) != 2 {
		t.Fatalf("lock intents audits=%d notifications=%d", len(audits), len(notifications))
	}
	if audits[0].Kind() != "LOGIN_SOURCE_LOCKED" || audits[0].Scope().UserID() != "" || notifications[0].Severity() != "WARNING" {
		t.Fatalf("source lock intent drift: %#v %#v", audits[0], notifications[0])
	}
	if audits[1].Kind() != "LOGIN_ACCOUNT_LOCKED" || audits[1].Scope().UserID() == "" || notifications[1].Severity() != "WARNING" {
		t.Fatalf("resolved-user lock intent drift: %#v %#v", audits[1], notifications[1])
	}
}
