package security

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const responseCacheTTLMS int64 = 120000

// Clock is the sole source of transaction time. Implementations must reject a
// durable watermark rollback rather than clamping it.
type Clock interface {
	TakeTransactionSnapshot(context.Context) (ClockSnapshot, error)
}

// TransactionalStore is a future persistence boundary. This package provides
// no production persistence implementation.
type TransactionalStore interface {
	WithinTransaction(context.Context, func(Transaction) error) error
}

// Transaction has the minimum operations needed for a response-result claim.
// It is deliberately not a generic query interface: source-only authority can
// never acquire an owner-table access method.
type Transaction interface {
	ValidateAuthority(context.Context, AuthorityScope) error
	FindClaim(context.Context, IdempotencyScope) (ResultClaim, bool, error)
	PutClaim(context.Context, ResultClaim) error
	ReserveNonce(context.Context, NonceReservation) error
	PutResponseCache(context.Context, SealedResponseCache) error
	EraseResponseCache(context.Context, ClaimID, CacheState) error
	MarkClaimReconciliationRequired(context.Context, ClaimID) error
}

// PasswordVerifier accepts only a sealed assertion and returns server-derived
// scope facts. It has no password hash or credential material in this API.
type PasswordVerifier interface {
	VerifyPassword(context.Context, PasswordAssertion) (PasswordVerification, error)
}

type PasswordAssertion struct{ opaque []byte }

func NewPasswordAssertion(opaque []byte) (PasswordAssertion, error) {
	if len(opaque) == 0 || len(opaque) > 32768 {
		return PasswordAssertion{}, ErrInvalidDTO
	}
	return PasswordAssertion{opaque: append([]byte(nil), opaque...)}, nil
}
func (v PasswordAssertion) bytes() []byte { return append([]byte(nil), v.opaque...) }

type PasswordVerification struct {
	authority AuthorityScope
	accepted  bool
}

func NewPasswordVerification(authority AuthorityScope, accepted bool) (PasswordVerification, error) {
	if !authority.valid() || (authority.Kind() != AuthoritySecuritySourceOnly && authority.Kind() != AuthoritySecurityResolvedUser) {
		return PasswordVerification{}, ErrInvalidDTO
	}
	return PasswordVerification{authority: authority, accepted: accepted}, nil
}
func (v PasswordVerification) Authority() AuthorityScope { return v.authority }
func (v PasswordVerification) Accepted() bool            { return v.accepted }

// AEADKeyring exposes opaque AES-256-GCM capabilities. Key bytes never cross
// this boundary or enter result/cache DTOs.
type AEADKeyring interface {
	ActiveEncryptKey(context.Context, ClockSnapshot) (AEADKeyCapability, error)
	DecryptKey(context.Context, string, ClockSnapshot) (AEADKeyCapability, error)
}
type AEADKeyCapability interface {
	KeyID() string
	Seal(Nonce96, []byte, []byte) ([]byte, error)
	Open(Nonce96, []byte, []byte) ([]byte, error)
}
type NonceSource interface {
	NextNonce(context.Context) (Nonce96, error)
}
type Nonce96 struct{ bytes [12]byte }

func NewNonce96(raw []byte) (Nonce96, error) {
	if len(raw) != 12 {
		return Nonce96{}, ErrInvalidDTO
	}
	var n Nonce96
	copy(n.bytes[:], raw)
	return n, nil
}
func (v Nonce96) Bytes() []byte { return append([]byte(nil), v.bytes[:]...) }

type NonceReservation struct {
	keyID      string
	nonce      Nonce96
	claimID    ClaimID
	reservedAt ClockSnapshot
}

func (v NonceReservation) KeyID() string    { return v.keyID }
func (v NonceReservation) ClaimID() ClaimID { return v.claimID }

type ResultClaim struct {
	id           ClaimID
	scope        IdempotencyScope
	requestID    RequestID
	firstDigest  RequestDigest
	class        ResultClass
	operation    OperationID
	hasOperation bool
	claimedAt    ClockSnapshot
	state        ClaimState
}

func NewResultClaim(id ClaimID, scope IdempotencyScope, requestID RequestID, digest RequestDigest, class ResultClass, snapshot ClockSnapshot) (ResultClaim, error) {
	if !id.value.valid() || !scope.authority.valid() || !requestID.value.valid() || class < CredentialSuccess || class > NonCredentialRejection {
		return ResultClaim{}, ErrInvalidDTO
	}
	return ResultClaim{id: id, scope: scope, requestID: requestID, firstDigest: digest, class: class, claimedAt: snapshot, state: ClaimedInTransaction}, nil
}
func (v ResultClaim) ID() ClaimID                       { return v.id }
func (v ResultClaim) Scope() IdempotencyScope           { return v.scope }
func (v ResultClaim) FirstRequestDigest() RequestDigest { return v.firstDigest }
func (v ResultClaim) State() ClaimState                 { return v.state }

type SealedResponseCache struct {
	claimID             ClaimID
	keyID               string
	nonce               Nonce96
	ciphertext          []byte
	aadDigest           BindingDigest
	exactResponseDigest Digest32
	createdAtMS         int64
	expiresAtMS         int64
	state               CacheState
}

func (v SealedResponseCache) ClaimID() ClaimID   { return v.claimID }
func (v SealedResponseCache) ExpiresAtMS() int64 { return v.expiresAtMS }
func (v SealedResponseCache) State() CacheState  { return v.state }

// Request is a sealed request-result DTO. Its digest and scope are supplied by
// the authenticated transport adapter, never derived from URL/body authority.
type Request struct {
	claimID              ClaimID
	scope                IdempotencyScope
	requestID            RequestID
	firstDigest          RequestDigest
	authenticatedContext Digest32
	response             ExactResponse
	confirmation         confirmationResponseBinding
	hasConfirmation      bool
}

func NewRequest(claimID ClaimID, scope IdempotencyScope, requestID RequestID, digest RequestDigest, authenticatedContext Digest32, response ExactResponse) (Request, error) {
	if !claimID.value.valid() || !scope.authority.valid() || !requestID.value.valid() || response.class < CredentialSuccess || response.class > NonCredentialRejection {
		return Request{}, ErrInvalidDTO
	}
	return Request{claimID: claimID, scope: scope, requestID: requestID, firstDigest: digest, authenticatedContext: authenticatedContext, response: response}, nil
}
func (v Request) Scope() IdempotencyScope    { return v.scope }
func (v Request) RequestID() RequestID       { return v.requestID }
func (v Request) FirstDigest() RequestDigest { return v.firstDigest }

type ExecutionResult struct {
	decision    ReplayDecision
	response    ExactResponse
	hasResponse bool
}

func (v ExecutionResult) Decision() ReplayDecision        { return v.decision }
func (v ExecutionResult) Response() (ExactResponse, bool) { return v.response, v.hasResponse }

type Effect func(context.Context, Transaction, ResultClaim, ClockSnapshot) error

// claimPreparation runs under the same transaction as the idempotency claim.
// It can bind a response to server-loaded facts before a cache is opened or
// sealed, and can suppress a fresh claim when the durable result must be
// reconciled instead.
type claimPreparation func(context.Context, Transaction, ClockSnapshot, bool) (preparedClaim, error)

type preparedClaim struct {
	request Request
	effect  Effect
	skip    bool
}

type preparedExecution struct {
	result  ExecutionResult
	skipped bool
}

// Execute provides the common response cache closed loop. The effect must make
// only the predeclared workflow writes through its typed transaction port.
func Execute(ctx context.Context, store TransactionalStore, clock Clock, keyring AEADKeyring, nonces NonceSource, request Request, effect Effect) (ExecutionResult, error) {
	prepared, err := executePrepared(ctx, store, clock, keyring, nonces, request, effect, nil)
	return prepared.result, err
}

func executePrepared(ctx context.Context, store TransactionalStore, clock Clock, keyring AEADKeyring, nonces NonceSource, request Request, effect Effect, prepare claimPreparation) (preparedExecution, error) {
	if store == nil || clock == nil || keyring == nil || nonces == nil || effect == nil {
		return preparedExecution{}, ErrDependencyUnavailable
	}
	snapshot, err := clock.TakeTransactionSnapshot(ctx)
	if err != nil {
		return preparedExecution{}, fmt.Errorf("%w: %v", ErrClockUnavailable, err)
	}
	var result preparedExecution
	err = store.WithinTransaction(ctx, func(tx Transaction) error {
		if tx == nil {
			return ErrDependencyUnavailable
		}
		if err := tx.ValidateAuthority(ctx, request.scope.authority); err != nil {
			return err
		}
		existing, found, err := tx.FindClaim(ctx, request.scope)
		if err != nil {
			return err
		}
		if found {
			if !existing.firstDigest.value.equal(request.firstDigest.value) {
				result.result = ExecutionResult{decision: IdempotencyConflict}
				return nil
			}
		}
		prepared := preparedClaim{request: request, effect: effect}
		if prepare != nil {
			prepared, err = prepare(ctx, tx, snapshot, found)
			if err != nil {
				return err
			}
		}
		if prepared.skip {
			result.skipped = true
			return nil
		}
		if found {
			response, replay, err := replayOrReconcile(ctx, tx, keyring, snapshot, existing, prepared.request)
			if err != nil {
				return err
			}
			result.result = ExecutionResult{decision: replay, response: response, hasResponse: replay == ReplayExact}
			return nil
		}
		claim, err := NewResultClaim(request.claimID, request.scope, request.requestID, request.firstDigest, prepared.request.response.class, snapshot)
		if err != nil {
			return err
		}
		if err := tx.PutClaim(ctx, claim); err != nil {
			return err
		}
		key, err := keyring.ActiveEncryptKey(ctx, snapshot)
		if err != nil || key == nil || key.KeyID() == "" {
			return ErrDependencyUnavailable
		}
		nonce, err := reserveNonce(ctx, tx, nonces, key.KeyID(), claim, snapshot)
		if err != nil {
			return err
		}
		if err := prepared.effect(ctx, tx, claim, snapshot); err != nil {
			return err
		}
		sealed, err := sealResponse(key, nonce, claim, prepared.request, snapshot)
		if err != nil {
			return err
		}
		if err := tx.PutResponseCache(ctx, sealed); err != nil {
			return err
		}
		result.result = ExecutionResult{decision: ClaimNew, response: prepared.request.response, hasResponse: true}
		return nil
	})
	if err != nil {
		return preparedExecution{}, err
	}
	return result, nil
}

func reserveNonce(ctx context.Context, tx Transaction, source NonceSource, keyID string, claim ResultClaim, snapshot ClockSnapshot) (Nonce96, error) {
	for attempt := 0; attempt < 3; attempt++ {
		nonce, err := source.NextNonce(ctx)
		if err != nil {
			return Nonce96{}, ErrNonceReservation
		}
		err = tx.ReserveNonce(ctx, NonceReservation{keyID: keyID, nonce: nonce, claimID: claim.id, reservedAt: snapshot})
		if err == nil {
			return nonce, nil
		}
		if !errors.Is(err, ErrNonceReservation) {
			return Nonce96{}, err
		}
	}
	return Nonce96{}, ErrNonceReservation
}

func replayOrReconcile(ctx context.Context, tx Transaction, keyring AEADKeyring, snapshot ClockSnapshot, claim ResultClaim, request Request) (ExactResponse, ReplayDecision, error) {
	cacheTx, ok := tx.(responseCacheReader)
	if !ok {
		return ExactResponse{}, ReconciliationRequiredNoReexecution, ErrDependencyUnavailable
	}
	cache, found, err := cacheTx.FindResponseCache(ctx, claim.id)
	if err != nil {
		return ExactResponse{}, ReconciliationRequiredNoReexecution, err
	}
	if !found || snapshot.utcNowMS >= cache.expiresAtMS {
		if found {
			if err := tx.EraseResponseCache(ctx, claim.id, CacheErasedAtTTL); err != nil {
				return ExactResponse{}, ReconciliationRequiredNoReexecution, err
			}
		}
		if err := tx.MarkClaimReconciliationRequired(ctx, claim.id); err != nil {
			return ExactResponse{}, ReconciliationRequiredNoReexecution, err
		}
		return ExactResponse{}, ReconciliationRequiredNoReexecution, nil
	}
	key, err := keyring.DecryptKey(ctx, cache.keyID, snapshot)
	if err != nil || key == nil {
		if e := markUndecryptable(ctx, tx, claim.id); e != nil {
			return ExactResponse{}, ReconciliationRequiredNoReexecution, e
		}
		return ExactResponse{}, ReconciliationRequiredNoReexecution, nil
	}
	response, err := openResponse(key, cache, claim, request)
	if err != nil {
		if e := markUndecryptable(ctx, tx, claim.id); e != nil {
			return ExactResponse{}, ReconciliationRequiredNoReexecution, e
		}
		return ExactResponse{}, ReconciliationRequiredNoReexecution, nil
	}
	return response, ReplayExact, nil
}

func markUndecryptable(ctx context.Context, tx Transaction, claimID ClaimID) error {
	if err := tx.EraseResponseCache(ctx, claimID, CacheUndecryptableReconciliationRequired); err != nil {
		return err
	}
	return tx.MarkClaimReconciliationRequired(ctx, claimID)
}

type responseCacheReader interface {
	FindResponseCache(context.Context, ClaimID) (SealedResponseCache, bool, error)
}

type encryptedResponse struct {
	Status     int         `json:"status"`
	Body       string      `json:"body"`
	RetryAfter *uint32     `json:"retry_after,omitempty"`
	SetCookie  string      `json:"set_cookie,omitempty"`
	Class      ResultClass `json:"class"`
}

func sealResponse(key AEADKeyCapability, nonce Nonce96, claim ResultClaim, request Request, snapshot ClockSnapshot) (SealedResponseCache, error) {
	if snapshot.utcNowMS > int64(^uint64(0)>>1)-responseCacheTTLMS {
		return SealedResponseCache{}, ErrInvalidDTO
	}
	expires := snapshot.utcNowMS + responseCacheTTLMS
	payload, err := json.Marshal(encryptedResponse{Status: request.response.status, Body: base64.RawURLEncoding.EncodeToString(request.response.body.bytes), RetryAfter: request.response.headers.retryAfter, SetCookie: request.response.headers.setCookie, Class: request.response.class})
	if err != nil {
		return SealedResponseCache{}, err
	}
	aad := responseAAD(claim, request, key.KeyID(), snapshot.utcNowMS, expires)
	ciphertext, err := key.Seal(nonce, payload, aad)
	if err != nil || len(ciphertext) == 0 {
		return SealedResponseCache{}, ErrDependencyUnavailable
	}
	return SealedResponseCache{claimID: claim.id, keyID: key.KeyID(), nonce: nonce, ciphertext: append([]byte(nil), ciphertext...), aadDigest: NewBindingDigest(DigestBytes(aad)), exactResponseDigest: DigestBytes(payload), createdAtMS: snapshot.utcNowMS, expiresAtMS: expires, state: CacheActive}, nil
}
func openResponse(key AEADKeyCapability, cache SealedResponseCache, claim ResultClaim, request Request) (ExactResponse, error) {
	aad := responseAAD(claim, request, cache.keyID, cache.createdAtMS, cache.expiresAtMS)
	if !cache.aadDigest.value.equal(DigestBytes(aad)) {
		return ExactResponse{}, ErrInvalidDTO
	}
	payload, err := key.Open(cache.nonce, cache.ciphertext, aad)
	if err != nil || !cache.exactResponseDigest.equal(DigestBytes(payload)) {
		return ExactResponse{}, ErrInvalidDTO
	}
	var decoded encryptedResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ExactResponse{}, err
	}
	body, err := base64.RawURLEncoding.DecodeString(decoded.Body)
	if err != nil {
		return ExactResponse{}, err
	}
	canonical, err := NewCanonicalJSON(body)
	if err != nil {
		return ExactResponse{}, err
	}
	headers, err := NewResponseHeaders(decoded.RetryAfter, decoded.SetCookie)
	if err != nil {
		return ExactResponse{}, err
	}
	return NewExactResponse(decoded.Status, canonical, headers, decoded.Class)
}
func responseAAD(claim ResultClaim, request Request, keyID string, created, expires int64) []byte {
	// A positional encoding avoids a mutable map and binds both the request
	// authority and every server-loaded confirmation fact used to authorize it.
	parts := []string{
		"fit.interface.response-cache-aad.v2",
		claim.id.String(),
		request.scope.key(),
		claim.requestID.String(),
		request.firstDigest.String(),
		request.authenticatedContext.String(),
		fmt.Sprint(claim.class),
		"request-authority",
	}
	parts = append(parts, authorityAADParts(request.scope.authority)...)
	parts = append(parts, "confirmation-bound", fmt.Sprint(request.hasConfirmation))
	if request.hasConfirmation {
		parts = append(parts,
			request.confirmation.confirmationID.String(),
			"confirmation-authority",
		)
		parts = append(parts, authorityAADParts(request.confirmation.authority)...)
		parts = append(parts,
			fmt.Sprint(request.confirmation.purpose),
			request.confirmation.intentDigest.String(),
			request.confirmation.confirmationHash.String(),
			fmt.Sprint(request.confirmation.expiresAtMS),
		)
	}
	parts = append(parts, keyID, fmt.Sprint(created), fmt.Sprint(expires))
	return []byte(stringsJoin(parts, "\x00"))
}

func authorityAADParts(authority AuthorityScope) []string {
	return []string{
		fmt.Sprint(authority.Kind()),
		authority.UserID().String(),
		authority.AccountID().String(),
		authority.OwnershipID().String(),
		authority.SessionID().String(),
		authority.DeviceID().String(),
		authority.value.sourceKeyID,
		authority.value.sourceKeyDigest.String(),
		authority.value.controlPlane.String(),
		authority.value.systemOperation.String(),
	}
}
func stringsJoin(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

// FailClosedRoute recognizes the two deliberately unresolved capabilities.
func FailClosedRoute(route RouteTemplate) error {
	switch route.String() {
	case "/v1/logout":
		return ErrUnsupportedRoute
	default:
		return nil
	}
}
func RejectTradingSubject(string) error { return ErrTradingSubject }
