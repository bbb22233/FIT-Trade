package security

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func testUUID(n int) UUID {
	v, err := NewUUID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
	if err != nil {
		panic(err)
	}
	return v
}
func testDigest(s string) Digest32 { return DigestBytes([]byte(s)) }
func testOwner(full bool) AuthorityScope {
	u, _ := NewUserID(testUUID(1))
	a, _ := NewAccountID(testUUID(2))
	o, _ := NewOwnershipID(testUUID(3))
	if !full {
		value, _ := NewOwnerAuthority(u, a, o)
		return value
	}
	s, _ := NewSessionID(testUUID(4))
	d, _ := NewDeviceID(testUUID(5))
	value, _ := NewFullOwnerAuthority(u, a, o, s, d)
	return value
}
func testRequest(t *testing.T, authority AuthorityScope, digest string, claim int) Request {
	t.Helper()
	route, err := NewRouteTemplate("/v1/confirmations/{confirmation_id}/consume")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := NewIdempotencyScope(authority, route, StableKeyIdempotency, NewStableKeyDigest(testDigest("stable")))
	if err != nil {
		t.Fatal(err)
	}
	requestID, _ := NewRequestID(testUUID(100 + claim))
	claimID, _ := NewClaimID(testUUID(200 + claim))
	body, err := NewCanonicalJSON([]byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	headers, _ := NewResponseHeaders(nil, "")
	response, err := NewExactResponse(200, body, headers, NonCredentialSuccess)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewRequest(claimID, scope, requestID, NewRequestDigest(testDigest(digest)), testDigest("context"), response)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func testRequestWithStableKey(t *testing.T, authority AuthorityScope, digest, stableKey string, claim int) Request {
	t.Helper()
	request := testRequest(t, authority, digest, claim)
	request.scope.keyDigest = NewStableKeyDigest(testDigest(stableKey))
	return request
}

type fakeClock struct {
	mu       sync.Mutex
	snapshot ClockSnapshot
	calls    int
}

func (v *fakeClock) TakeTransactionSnapshot(context.Context) (ClockSnapshot, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	return v.snapshot, nil
}

type fakeStore struct {
	mu              sync.Mutex
	claims          map[string]ResultClaim
	caches          map[string]SealedResponseCache
	nonces          map[string]bool
	marked          map[string]bool
	effects         int
	confirmation    ConfirmationFact
	hasConfirmation bool
	operation       ConfirmationOperation
	hasOperation    bool
	linked          bool
	audits          int
}

func newFakeStore() *fakeStore {
	return &fakeStore{claims: map[string]ResultClaim{}, caches: map[string]SealedResponseCache{}, nonces: map[string]bool{}, marked: map[string]bool{}}
}
func (s *fakeStore) WithinTransaction(_ context.Context, fn func(Transaction) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(s)
}
func (s *fakeStore) ValidateAuthority(_ context.Context, authority AuthorityScope) error {
	if !authority.valid() {
		return ErrInvalidDTO
	}
	return nil
}
func (s *fakeStore) FindClaim(_ context.Context, scope IdempotencyScope) (ResultClaim, bool, error) {
	claim, ok := s.claims[scope.key()]
	return claim, ok, nil
}
func (s *fakeStore) PutClaim(_ context.Context, claim ResultClaim) error {
	if _, ok := s.claims[claim.scope.key()]; ok {
		return errors.New("duplicate claim")
	}
	s.claims[claim.scope.key()] = claim
	return nil
}
func (s *fakeStore) ReserveNonce(_ context.Context, reservation NonceReservation) error {
	key := reservation.keyID + string(reservation.nonce.bytes[:])
	if s.nonces[key] {
		return ErrNonceReservation
	}
	s.nonces[key] = true
	return nil
}
func (s *fakeStore) PutResponseCache(_ context.Context, cache SealedResponseCache) error {
	s.caches[cache.claimID.String()] = cache
	return nil
}
func (s *fakeStore) EraseResponseCache(_ context.Context, id ClaimID, _ CacheState) error {
	delete(s.caches, id.String())
	return nil
}
func (s *fakeStore) MarkClaimReconciliationRequired(_ context.Context, id ClaimID) error {
	s.marked[id.String()] = true
	claim := s.claimsByID(id)
	claim.state = CommittedReconciliationRequired
	s.claims[claim.scope.key()] = claim
	return nil
}
func (s *fakeStore) FindResponseCache(_ context.Context, id ClaimID) (SealedResponseCache, bool, error) {
	cache, ok := s.caches[id.String()]
	return cache, ok, nil
}
func (s *fakeStore) claimsByID(id ClaimID) ResultClaim {
	for _, claim := range s.claims {
		if claim.id == id {
			return claim
		}
	}
	return ResultClaim{}
}

func (s *fakeStore) ValidateActiveSessionDevice(_ context.Context, authority AuthorityScope) error {
	if !ActiveSessionDevice(authority, false, false) {
		return ErrInvalidDTO
	}
	return nil
}
func (s *fakeStore) LoadConfirmation(_ context.Context, id ConfirmationID) (ConfirmationFact, bool, error) {
	return s.confirmation, s.hasConfirmation && s.confirmation.id == id, nil
}
func (s *fakeStore) LoadConfirmationOperation(_ context.Context, id ConfirmationID) (ConfirmationOperation, bool, error) {
	return s.operation, s.hasOperation && s.operation.confirmationID == id, nil
}
func (s *fakeStore) CASConfirmation(_ context.Context, id ConfirmationID, from, to ConfirmationState, claim ClaimID, _ ClockSnapshot) error {
	if !s.hasConfirmation || s.confirmation.id != id || s.confirmation.state != from {
		return errors.New("confirmation CAS")
	}
	s.confirmation.state, s.confirmation.consumedClaim, s.confirmation.hasConsumedClaim = to, claim, true
	return nil
}
func (s *fakeStore) CASOperation(_ context.Context, id OperationID, from OperationState, version uint64, to OperationState) error {
	if !s.hasOperation || s.operation.id != id || s.operation.state != from || s.operation.version != version {
		return errors.New("operation CAS")
	}
	s.operation.state, s.operation.version = to, version+1
	return nil
}
func (s *fakeStore) LinkConfirmationConsumption(_ context.Context, confirmation ConfirmationFact, operation ConfirmationOperation, claim ResultClaim, _ ClockSnapshot) error {
	if confirmation.id != s.confirmation.id || operation.id != s.operation.id || claim.id != s.confirmation.consumedClaim {
		return errors.New("invalid consumption link")
	}
	s.linked = true
	return nil
}
func (s *fakeStore) RecordConfirmationAudit(_ context.Context, _ ConfirmationOperation, _ ClockSnapshot) error {
	s.audits++
	return nil
}
func (s *fakeStore) ReconcileConfirmationOperation(_ context.Context, id ConfirmationID, authority AuthorityScope) (ConfirmationOperation, bool, error) {
	return s.operation, s.hasOperation && s.operation.confirmationID == id && sameFullAuthority(s.operation.authority, authority), nil
}

type fakeKeyring struct{ key *fakeKey }

func newFakeKeyring(t *testing.T) *fakeKeyring {
	t.Helper()
	block, err := aes.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeKeyring{key: &fakeKey{id: "test-key", gcm: gcm}}
}
func (v *fakeKeyring) ActiveEncryptKey(context.Context, ClockSnapshot) (AEADKeyCapability, error) {
	return v.key, nil
}
func (v *fakeKeyring) DecryptKey(_ context.Context, id string, _ ClockSnapshot) (AEADKeyCapability, error) {
	if id != v.key.id {
		return nil, ErrDependencyUnavailable
	}
	return v.key, nil
}

type fakeKey struct {
	id  string
	gcm cipher.AEAD
}

func (v *fakeKey) KeyID() string { return v.id }
func (v *fakeKey) Seal(n Nonce96, plaintext, aad []byte) ([]byte, error) {
	return v.gcm.Seal(nil, n.bytes[:], plaintext, aad), nil
}
func (v *fakeKey) Open(n Nonce96, ciphertext, aad []byte) ([]byte, error) {
	return v.gcm.Open(nil, n.bytes[:], ciphertext, aad)
}

type fakeNonceSource struct {
	mu     sync.Mutex
	values []Nonce96
	index  int
}

func (v *fakeNonceSource) NextNonce(context.Context) (Nonce96, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.index >= len(v.values) {
		return Nonce96{}, ErrNonceReservation
	}
	n := v.values[v.index]
	v.index++
	return n, nil
}
func testNonces(t *testing.T, n int) *fakeNonceSource {
	t.Helper()
	source := &fakeNonceSource{}
	for i := 0; i < n; i++ {
		nonce, err := NewNonce96([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		source.values = append(source.values, nonce)
	}
	return source
}

func TestExecuteReplaysConflictAndExpiry(t *testing.T) {
	clockSnapshot, _ := NewClockSnapshot(1000, 1, 1)
	clock := &fakeClock{snapshot: clockSnapshot}
	store := newFakeStore()
	keys := newFakeKeyring(t)
	request := testRequest(t, testOwner(false), "one", 1)
	effect := func(_ context.Context, s Transaction, _ ResultClaim, _ ClockSnapshot) error {
		s.(*fakeStore).effects++
		return nil
	}
	result, err := Execute(context.Background(), store, clock, keys, testNonces(t, 3), request, effect)
	if err != nil || result.Decision() != ClaimNew || store.effects != 1 {
		t.Fatalf("new result=%v err=%v effects=%d", result.Decision(), err, store.effects)
	}
	replayRequest := testRequest(t, testOwner(false), "one", 2)
	result, err = Execute(context.Background(), store, clock, keys, testNonces(t, 3), replayRequest, effect)
	if err != nil || result.Decision() != ReplayExact || store.effects != 1 {
		t.Fatalf("replay result=%v err=%v effects=%d", result.Decision(), err, store.effects)
	}
	conflict := testRequest(t, testOwner(false), "different", 3)
	result, err = Execute(context.Background(), store, clock, keys, testNonces(t, 3), conflict, effect)
	if err != nil || result.Decision() != IdempotencyConflict || store.effects != 1 {
		t.Fatalf("conflict result=%v err=%v effects=%d", result.Decision(), err, store.effects)
	}
	clock.snapshot.utcNowMS += responseCacheTTLMS
	result, err = Execute(context.Background(), store, clock, keys, testNonces(t, 3), replayRequest, effect)
	if err != nil || result.Decision() != ReconciliationRequiredNoReexecution || store.effects != 1 || !store.marked[request.claimID.String()] {
		t.Fatalf("expiry result=%v err=%v effects=%d marked=%v", result.Decision(), err, store.effects, store.marked)
	}
}

func TestExecuteNonceCollisionFailsBeforeEffect(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	store := newFakeStore()
	keys := newFakeKeyring(t)
	request := testRequest(t, testOwner(false), "one", 1)
	source := testNonces(t, 1)
	first, _ := source.NextNonce(context.Background())
	store.nonces["test-key"+string(first.bytes[:])] = true
	source = &fakeNonceSource{values: []Nonce96{first, first, first}}
	_, err := Execute(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, source, request, func(_ context.Context, s Transaction, _ ResultClaim, _ ClockSnapshot) error {
		s.(*fakeStore).effects++
		return nil
	})
	if !errors.Is(err, ErrNonceReservation) || store.effects != 0 {
		t.Fatalf("err=%v effects=%d", err, store.effects)
	}
}

func TestExecuteConcurrentSameScopeHasOneEffect(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	store := newFakeStore()
	keys := newFakeKeyring(t)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := testRequest(t, testOwner(false), "one", i+1)
			_, err := Execute(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), request, func(_ context.Context, s Transaction, _ ResultClaim, _ ClockSnapshot) error {
				s.(*fakeStore).effects++
				return nil
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if store.effects != 1 {
		t.Fatalf("effects=%d", store.effects)
	}
}

func TestConfirmationFullAuthorityAndExpiry(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	store := newFakeStore()
	owner := testOwner(true)
	confirmationID, _ := NewConfirmationID(testUUID(10))
	operationID, _ := NewOperationID(testUUID(11))
	confirmation, _ := NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
	operation, _ := NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 3)
	store.confirmation, store.hasConfirmation, store.operation, store.hasOperation = confirmation, true, operation, true
	request := testRequest(t, owner, "consume", 1)
	input, err := NewConfirmationConsumeRequest(request, confirmationID, testDigest("hash"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, newFakeKeyring(t), testNonces(t, 3), input)
	if err != nil || result.Decision() != ConsumeAndConfirm || !store.linked || store.operation.state != OperationConfirmed || store.audits != 1 {
		t.Fatalf("result=%v err=%v linked=%v state=%v audits=%d", result.Decision(), err, store.linked, store.operation.state, store.audits)
	}

	store = newFakeStore()
	wrongOwner := testOwner(true)
	wrongOwner.value.ownership, _ = NewOwnershipID(testUUID(99))
	confirmation, _ = NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
	operation, _ = NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 3)
	store.confirmation, store.hasConfirmation, store.operation, store.hasOperation = confirmation, true, operation, true
	request = testRequest(t, wrongOwner, "consume", 2)
	input, _ = NewConfirmationConsumeRequest(request, confirmationID, testDigest("hash"))
	result, err = ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, newFakeKeyring(t), testNonces(t, 3), input)
	if err != nil || result.Decision() != FailClosedGenericRejection || store.operation.state != OperationAwaitingConfirmation || store.audits != 0 {
		t.Fatalf("mismatch result=%v err=%v state=%v audits=%d", result.Decision(), err, store.operation.state, store.audits)
	}

	store = newFakeStore()
	confirmation, _ = NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 1000, ConfirmationIssued)
	operation, _ = NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 3)
	store.confirmation, store.hasConfirmation, store.operation, store.hasOperation = confirmation, true, operation, true
	request = testRequest(t, owner, "expiry", 3)
	input, _ = NewConfirmationConsumeRequest(request, confirmationID, testDigest("hash"))
	result, err = ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, newFakeKeyring(t), testNonces(t, 3), input)
	if err != nil || result.Decision() != ExpireAndReject || store.confirmation.state != ConfirmationExpired || store.operation.state != OperationExpired {
		t.Fatalf("expiry result=%v err=%v confirmation=%v operation=%v", result.Decision(), err, store.confirmation.state, store.operation.state)
	}
}

func TestConsumedConfirmationWithFreshKeyDoesNotAllocate(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	store := newFakeStore()
	owner := testOwner(true)
	confirmationID, _ := NewConfirmationID(testUUID(510))
	operationID, _ := NewOperationID(testUUID(511))
	confirmation, _ := NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
	operation, _ := NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 1)
	store.confirmation, store.hasConfirmation = confirmation, true
	store.operation, store.hasOperation = operation, true
	keys := newFakeKeyring(t)
	nonces := testNonces(t, 3)

	firstRequest := testRequestWithStableKey(t, owner, "consume", "first-key", 510)
	first, _ := NewConfirmationConsumeRequest(firstRequest, confirmationID, testDigest("hash"))
	result, err := ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, nonces, first)
	if err != nil || result.Decision() != ConsumeAndConfirm {
		t.Fatalf("first result=%v err=%v", result.Decision(), err)
	}

	freshRequest := testRequestWithStableKey(t, owner, "consume", "fresh-key", 511)
	fresh, _ := NewConfirmationConsumeRequest(freshRequest, confirmationID, testDigest("hash"))
	result, err = ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, nonces, fresh)
	if err != nil || result.Decision() != DurableOperationReconciliation {
		t.Fatalf("fresh result=%v err=%v", result.Decision(), err)
	}
	if got := len(store.claims); got != 1 {
		t.Fatalf("claims=%d, want 1", got)
	}
	if got := len(store.nonces); got != 1 {
		t.Fatalf("nonces=%d, want 1", got)
	}
	if got := len(store.caches); got != 1 {
		t.Fatalf("caches=%d, want 1", got)
	}
}

func TestConfirmationFailureUsesServerCachedRejection(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	store := newFakeStore()
	owner := testOwner(true)
	confirmationID, _ := NewConfirmationID(testUUID(520))
	operationID, _ := NewOperationID(testUUID(521))
	confirmation, _ := NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
	operation, _ := NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 1)
	store.confirmation, store.hasConfirmation = confirmation, true
	store.operation, store.hasOperation = operation, true
	keys := newFakeKeyring(t)
	request := testRequestWithStableKey(t, owner, "mismatch", "failure-key", 520)
	input, _ := NewConfirmationConsumeRequest(request, confirmationID, testDigest("wrong-hash"))

	result, err := ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), input)
	if err != nil || result.Decision() != FailClosedGenericRejection {
		t.Fatalf("result=%v err=%v", result.Decision(), err)
	}
	claim := store.claimsByID(request.claimID)
	cache := store.caches[request.claimID.String()]
	response, err := openResponse(keys.key, cache, claim, bindConfirmationResponse(request, confirmation))
	if err != nil || response.class != NonCredentialRejection || response.status != 400 || string(response.body.bytes) == string(request.response.body.bytes) {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	result, err = ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), input)
	if err != nil || result.Decision() != ConfirmationReplayExact || len(store.claims) != 1 || len(store.nonces) != 1 || len(store.caches) != 1 {
		t.Fatalf("replay result=%v err=%v claims=%d nonces=%d caches=%d", result.Decision(), err, len(store.claims), len(store.nonces), len(store.caches))
	}
}

func TestConfirmationCacheLossAndTimeoutReconcileWithoutReexecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*fakeStore, *fakeClock, ClaimID)
	}{
		{
			name: "cache loss",
			mutate: func(store *fakeStore, _ *fakeClock, claimID ClaimID) {
				delete(store.caches, claimID.String())
			},
		},
		{
			name: "cache timeout",
			mutate: func(_ *fakeStore, clock *fakeClock, _ ClaimID) {
				clock.snapshot.utcNowMS += responseCacheTTLMS
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, _ := NewClockSnapshot(1000, 1, 1)
			clock := &fakeClock{snapshot: snapshot}
			store := newFakeStore()
			owner := testOwner(true)
			confirmationID, _ := NewConfirmationID(testUUID(530))
			operationID, _ := NewOperationID(testUUID(531))
			confirmation, _ := NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
			operation, _ := NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 1)
			store.confirmation, store.hasConfirmation = confirmation, true
			store.operation, store.hasOperation = operation, true
			request := testRequestWithStableKey(t, owner, "consume", "cache-key", 530)
			input, _ := NewConfirmationConsumeRequest(request, confirmationID, testDigest("hash"))
			keys := newFakeKeyring(t)
			nonces := testNonces(t, 3)
			result, err := ConsumeConfirmation(context.Background(), store, clock, keys, nonces, input)
			if err != nil || result.Decision() != ConsumeAndConfirm {
				t.Fatalf("initial result=%v err=%v", result.Decision(), err)
			}
			tc.mutate(store, clock, request.claimID)
			result, err = ConsumeConfirmation(context.Background(), store, clock, keys, nonces, input)
			if err != nil || result.Decision() != DurableOperationReconciliation || store.audits != 1 || len(store.claims) != 1 || len(store.nonces) != 1 {
				t.Fatalf("reconcile result=%v err=%v audits=%d claims=%d nonces=%d", result.Decision(), err, store.audits, len(store.claims), len(store.nonces))
			}
		})
	}
}

func TestResponseAADRejectsCrossUserAndDeviceCacheSubstitution(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	keys := newFakeKeyring(t)
	store := newFakeStore()
	ownerA := testOwner(true)
	requestA := testRequestWithStableKey(t, ownerA, "same", "shared-key", 540)
	effect := func(_ context.Context, s Transaction, _ ResultClaim, _ ClockSnapshot) error {
		s.(*fakeStore).effects++
		return nil
	}
	if result, err := Execute(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), requestA, effect); err != nil || result.Decision() != ClaimNew {
		t.Fatalf("seed result=%v err=%v", result.Decision(), err)
	}

	ownerB := ownerA
	ownerB.value.user, _ = NewUserID(testUUID(541))
	requestB := testRequestWithStableKey(t, ownerB, "same", "shared-key", 541)
	claimB, _ := NewResultClaim(requestB.claimID, requestB.scope, requestB.requestID, requestB.firstDigest, requestB.response.class, snapshot)
	store.claims[requestB.scope.key()] = claimB
	cacheB := store.caches[requestA.claimID.String()]
	cacheB.claimID = requestB.claimID
	store.caches[requestB.claimID.String()] = cacheB
	result, err := Execute(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), requestB, effect)
	if err != nil || result.Decision() != ReconciliationRequiredNoReexecution || store.effects != 1 || !store.marked[requestB.claimID.String()] {
		t.Fatalf("cross-user result=%v err=%v effects=%d marked=%v", result.Decision(), err, store.effects, store.marked[requestB.claimID.String()])
	}

	store = newFakeStore()
	if result, err = Execute(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), requestA, effect); err != nil || result.Decision() != ClaimNew {
		t.Fatalf("device seed result=%v err=%v", result.Decision(), err)
	}
	ownerDeviceB := ownerA
	ownerDeviceB.value.device, _ = NewDeviceID(testUUID(542))
	requestDeviceB := testRequestWithStableKey(t, ownerDeviceB, "same", "shared-key", 542)
	result, err = Execute(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), requestDeviceB, effect)
	if err != nil || result.Decision() != ReconciliationRequiredNoReexecution || store.effects != 1 || !store.marked[requestA.claimID.String()] {
		t.Fatalf("cross-device result=%v err=%v effects=%d marked=%v", result.Decision(), err, store.effects, store.marked[requestA.claimID.String()])
	}
}

func TestConfirmationResponseAADBindsAuthorityAndTicketFacts(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	owner := testOwner(true)
	request := testRequestWithStableKey(t, owner, "consume", "binding-key", 545)
	claim, _ := NewResultClaim(request.claimID, request.scope, request.requestID, request.firstDigest, request.response.class, snapshot)
	confirmationID, _ := NewConfirmationID(testUUID(545))
	confirmation, _ := NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
	bound := bindConfirmationResponse(request, confirmation)
	want := responseAAD(claim, bound, "test-key", 1000, 121000)

	cases := []struct {
		name   string
		mutate func(*Request)
	}{
		{
			name: "request owner",
			mutate: func(request *Request) {
				request.scope.authority.value.user, _ = NewUserID(testUUID(546))
			},
		},
		{
			name: "request device",
			mutate: func(request *Request) {
				request.scope.authority.value.device, _ = NewDeviceID(testUUID(547))
			},
		},
		{
			name: "confirmation owner",
			mutate: func(request *Request) {
				request.confirmation.authority.value.ownership, _ = NewOwnershipID(testUUID(548))
			},
		},
		{
			name: "confirmation device",
			mutate: func(request *Request) {
				request.confirmation.authority.value.device, _ = NewDeviceID(testUUID(549))
			},
		},
		{
			name: "confirmation id",
			mutate: func(request *Request) {
				request.confirmation.confirmationID, _ = NewConfirmationID(testUUID(550))
			},
		},
		{
			name: "action",
			mutate: func(request *Request) {
				request.confirmation.purpose = IncreasePosition
			},
		},
		{
			name: "intent",
			mutate: func(request *Request) {
				request.confirmation.intentDigest = testDigest("other-intent")
			},
		},
		{
			name: "confirmation signature",
			mutate: func(request *Request) {
				request.confirmation.confirmationHash = testDigest("other-hash")
			},
		},
		{
			name: "expiry",
			mutate: func(request *Request) {
				request.confirmation.expiresAtMS++
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := bound
			tc.mutate(&changed)
			if bytes.Equal(want, responseAAD(claim, changed, "test-key", 1000, 121000)) {
				t.Fatal("AAD accepted a substituted authority or ticket fact")
			}
		})
	}
}

func TestConcurrentFreshKeysConsumeOnlyOnce(t *testing.T) {
	snapshot, _ := NewClockSnapshot(1000, 1, 1)
	store := newFakeStore()
	owner := testOwner(true)
	confirmationID, _ := NewConfirmationID(testUUID(550))
	operationID, _ := NewOperationID(testUUID(551))
	confirmation, _ := NewConfirmationFact(confirmationID, owner, OpenPosition, testDigest("intent"), testDigest("hash"), 2000, ConfirmationIssued)
	operation, _ := NewConfirmationOperation(operationID, confirmationID, owner, testDigest("intent"), OperationAwaitingConfirmation, 1)
	store.confirmation, store.hasConfirmation = confirmation, true
	store.operation, store.hasOperation = operation, true
	keys := newFakeKeyring(t)
	inputs := make([]ConfirmationConsumeRequest, 16)
	for i := range inputs {
		request := testRequestWithStableKey(t, owner, "consume", fmt.Sprintf("fresh-%d", i), 550+i)
		inputs[i], _ = NewConfirmationConsumeRequest(request, confirmationID, testDigest("hash"))
	}
	decisions := make(chan ConfirmationConsumeDecision, len(inputs))
	errs := make(chan error, len(inputs))
	var wg sync.WaitGroup
	for _, input := range inputs {
		wg.Add(1)
		go func(input ConfirmationConsumeRequest) {
			defer wg.Done()
			result, err := ConsumeConfirmation(context.Background(), store, &fakeClock{snapshot: snapshot}, keys, testNonces(t, 3), input)
			if err == nil {
				decisions <- result.Decision()
			}
			errs <- err
		}(input)
	}
	wg.Wait()
	close(errs)
	close(decisions)
	consumed, reconciled := 0, 0
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for decision := range decisions {
		switch decision {
		case ConsumeAndConfirm:
			consumed++
		case DurableOperationReconciliation:
			reconciled++
		default:
			t.Fatalf("unexpected decision=%v", decision)
		}
	}
	if consumed != 1 || reconciled != len(inputs)-1 || len(store.claims) != 1 || len(store.nonces) != 1 || len(store.caches) != 1 || store.audits != 1 {
		t.Fatalf("consumed=%d reconciled=%d claims=%d nonces=%d caches=%d audits=%d", consumed, reconciled, len(store.claims), len(store.nonces), len(store.caches), store.audits)
	}
}

func TestTransportAndFailClosedGaps(t *testing.T) {
	limits := DevelopmentTransportLimits()
	if err := limits.AdmitRequest(8192, 16384, 65536, true, false); err != nil {
		t.Fatal(err)
	}
	if err := limits.AdmitRequest(8193, 1, 1, false, false); err == nil {
		t.Fatal("accepted oversized target")
	}
	if err := FailClosedRoute(mustRoute(t, "/v1/logout")); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("logout=%v", err)
	}
	if !errors.Is(RejectTradingSubject("fit.platform.v1.order.created"), ErrTradingSubject) {
		t.Fatal("trading subject accepted")
	}
}

func TestSourceOnlyAuthorityHasNoOwnerTuple(t *testing.T) {
	aggregate, _ := NewAggregateID(testUUID(70))
	authority, err := NewSourceOnlyAuthority("source-1", NewSourceKeyDigest(testDigest("source")), aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Kind() != AuthoritySecuritySourceOnly || authority.UserID().String() != "" || authority.AccountID().String() != "" || authority.OwnershipID().String() != "" {
		t.Fatalf("source-only authority acquired owner facts: %#v", authority)
	}
	route := mustRoute(t, "/v1/password/failure")
	scope, err := NewIdempotencyScope(authority, route, StableKeyRequestID, NewStableKeyDigest(testDigest("request")))
	if err != nil {
		t.Fatal(err)
	}
	if scope.key() == "" {
		t.Fatal("empty source-only scope")
	}
}
func mustRoute(t *testing.T, raw string) RouteTemplate {
	t.Helper()
	value, err := NewRouteTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
