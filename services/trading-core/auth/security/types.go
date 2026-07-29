// Package security contains the closed, transport-free authentication policy
// core. It deliberately has no HTTP, database, broker, signer, or exchange
// implementation.
package security

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrInvalidDTO            = errors.New("auth security: invalid typed dto")
	ErrDependencyUnavailable = errors.New("auth security: required dependency unavailable")
	ErrUnsupportedRoute      = errors.New("auth security: unsupported route fails closed")
	ErrTradingSubject        = errors.New("auth security: trading subject is forbidden")
	ErrClockUnavailable      = errors.New("auth security: clock unavailable")
	ErrClockRollback         = errors.New("auth security: clock rollback")
	ErrNonceReservation      = errors.New("auth security: nonce reservation failed")
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// UUID is an immutable, validated identifier. Individual tagged IDs wrap it
// so identifiers cannot be interchanged accidentally.
type UUID struct{ value string }

func NewUUID(raw string) (UUID, error) {
	if !uuidRE.MatchString(raw) {
		return UUID{}, fmt.Errorf("%w: UUID", ErrInvalidDTO)
	}
	return UUID{value: raw}, nil
}
func (v UUID) String() string { return v.value }
func (v UUID) valid() bool    { return uuidRE.MatchString(v.value) }

type RequestID struct{ value UUID }
type OperationID struct{ value UUID }
type ConfirmationID struct{ value UUID }
type SessionID struct{ value UUID }
type DeviceID struct{ value UUID }
type OwnershipID struct{ value UUID }
type AccountID struct{ value UUID }
type UserID struct{ value UUID }
type ClaimID struct{ value UUID }
type AggregateID struct{ value UUID }

func NewRequestID(v UUID) (RequestID, error)           { return taggedRequestID(v) }
func NewOperationID(v UUID) (OperationID, error)       { return taggedOperationID(v) }
func NewConfirmationID(v UUID) (ConfirmationID, error) { return taggedConfirmationID(v) }
func NewSessionID(v UUID) (SessionID, error)           { return taggedSessionID(v) }
func NewDeviceID(v UUID) (DeviceID, error)             { return taggedDeviceID(v) }
func NewOwnershipID(v UUID) (OwnershipID, error)       { return taggedOwnershipID(v) }
func NewAccountID(v UUID) (AccountID, error)           { return taggedAccountID(v) }
func NewUserID(v UUID) (UserID, error)                 { return taggedUserID(v) }
func NewClaimID(v UUID) (ClaimID, error)               { return taggedClaimID(v) }
func NewAggregateID(v UUID) (AggregateID, error)       { return taggedAggregateID(v) }

func taggedRequestID(v UUID) (RequestID, error) {
	if !v.valid() {
		return RequestID{}, ErrInvalidDTO
	}
	return RequestID{v}, nil
}
func taggedOperationID(v UUID) (OperationID, error) {
	if !v.valid() {
		return OperationID{}, ErrInvalidDTO
	}
	return OperationID{v}, nil
}
func taggedConfirmationID(v UUID) (ConfirmationID, error) {
	if !v.valid() {
		return ConfirmationID{}, ErrInvalidDTO
	}
	return ConfirmationID{v}, nil
}
func taggedSessionID(v UUID) (SessionID, error) {
	if !v.valid() {
		return SessionID{}, ErrInvalidDTO
	}
	return SessionID{v}, nil
}
func taggedDeviceID(v UUID) (DeviceID, error) {
	if !v.valid() {
		return DeviceID{}, ErrInvalidDTO
	}
	return DeviceID{v}, nil
}
func taggedOwnershipID(v UUID) (OwnershipID, error) {
	if !v.valid() {
		return OwnershipID{}, ErrInvalidDTO
	}
	return OwnershipID{v}, nil
}
func taggedAccountID(v UUID) (AccountID, error) {
	if !v.valid() {
		return AccountID{}, ErrInvalidDTO
	}
	return AccountID{v}, nil
}
func taggedUserID(v UUID) (UserID, error) {
	if !v.valid() {
		return UserID{}, ErrInvalidDTO
	}
	return UserID{v}, nil
}
func taggedClaimID(v UUID) (ClaimID, error) {
	if !v.valid() {
		return ClaimID{}, ErrInvalidDTO
	}
	return ClaimID{v}, nil
}
func taggedAggregateID(v UUID) (AggregateID, error) {
	if !v.valid() {
		return AggregateID{}, ErrInvalidDTO
	}
	return AggregateID{v}, nil
}

func (v RequestID) String() string      { return v.value.String() }
func (v OperationID) String() string    { return v.value.String() }
func (v ConfirmationID) String() string { return v.value.String() }
func (v SessionID) String() string      { return v.value.String() }
func (v DeviceID) String() string       { return v.value.String() }
func (v OwnershipID) String() string    { return v.value.String() }
func (v AccountID) String() string      { return v.value.String() }
func (v UserID) String() string         { return v.value.String() }
func (v ClaimID) String() string        { return v.value.String() }
func (v AggregateID) String() string    { return v.value.String() }

type Digest32 struct{ bytes [32]byte }

func NewDigest32Hex(raw string) (Digest32, error) {
	if len(raw) != 64 || strings.ToLower(raw) != raw {
		return Digest32{}, fmt.Errorf("%w: digest", ErrInvalidDTO)
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != 32 {
		return Digest32{}, fmt.Errorf("%w: digest", ErrInvalidDTO)
	}
	var out Digest32
	copy(out.bytes[:], b)
	return out, nil
}
func DigestBytes(raw []byte) Digest32        { return Digest32{bytes: sha256.Sum256(raw)} }
func (v Digest32) String() string            { return hex.EncodeToString(v.bytes[:]) }
func (v Digest32) equal(other Digest32) bool { return v == other }

type RequestDigest struct{ value Digest32 }
type StableKeyDigest struct{ value Digest32 }
type SourceKeyDigest struct{ value Digest32 }
type BindingDigest struct{ value Digest32 }

func NewRequestDigest(v Digest32) RequestDigest     { return RequestDigest{v} }
func NewStableKeyDigest(v Digest32) StableKeyDigest { return StableKeyDigest{v} }
func NewSourceKeyDigest(v Digest32) SourceKeyDigest { return SourceKeyDigest{v} }
func NewBindingDigest(v Digest32) BindingDigest     { return BindingDigest{v} }
func (v RequestDigest) String() string              { return v.value.String() }
func (v StableKeyDigest) String() string            { return v.value.String() }
func (v SourceKeyDigest) String() string            { return v.value.String() }
func (v BindingDigest) String() string              { return v.value.String() }

type RouteTemplate struct{ value string }

func NewRouteTemplate(raw string) (RouteTemplate, error) {
	if raw == "" || !strings.HasPrefix(raw, "/v1/") || strings.ContainsAny(raw, "?#") {
		return RouteTemplate{}, fmt.Errorf("%w: route template", ErrInvalidDTO)
	}
	return RouteTemplate{value: raw}, nil
}
func (v RouteTemplate) String() string { return v.value }

type AuthorityScopeKind uint8

const (
	AuthorityOwner AuthorityScopeKind = iota + 1
	AuthoritySecuritySourceOnly
	AuthoritySecurityResolvedUser
	AuthoritySystem
)

type authorityScope struct {
	kind            AuthorityScopeKind
	user            UserID
	account         AccountID
	ownership       OwnershipID
	session         SessionID
	device          DeviceID
	sourceKeyID     string
	sourceKeyDigest SourceKeyDigest
	controlPlane    AggregateID
	systemOperation OperationID
}

// AuthorityScope is sealed: callers can obtain it only through checked
// constructors, and may inspect but not alter the variant fields.
type AuthorityScope struct{ value authorityScope }

func NewOwnerAuthority(user UserID, account AccountID, ownership OwnershipID) (AuthorityScope, error) {
	if !user.value.valid() || !account.value.valid() || !ownership.value.valid() {
		return AuthorityScope{}, ErrInvalidDTO
	}
	return AuthorityScope{value: authorityScope{kind: AuthorityOwner, user: user, account: account, ownership: ownership}}, nil
}
func NewFullOwnerAuthority(user UserID, account AccountID, ownership OwnershipID, session SessionID, device DeviceID) (AuthorityScope, error) {
	a, err := NewOwnerAuthority(user, account, ownership)
	if err != nil || !session.value.valid() || !device.value.valid() {
		return AuthorityScope{}, ErrInvalidDTO
	}
	a.value.session, a.value.device = session, device
	return a, nil
}
func NewSourceOnlyAuthority(sourceKeyID string, source SourceKeyDigest, aggregate AggregateID) (AuthorityScope, error) {
	if sourceKeyID == "" || !aggregate.value.valid() {
		return AuthorityScope{}, ErrInvalidDTO
	}
	return AuthorityScope{value: authorityScope{kind: AuthoritySecuritySourceOnly, sourceKeyID: sourceKeyID, sourceKeyDigest: source, controlPlane: aggregate}}, nil
}
func NewResolvedUserAuthority(sourceKeyID string, source SourceKeyDigest, user UserID, aggregate AggregateID) (AuthorityScope, error) {
	if sourceKeyID == "" || !user.value.valid() || !aggregate.value.valid() {
		return AuthorityScope{}, ErrInvalidDTO
	}
	return AuthorityScope{value: authorityScope{kind: AuthoritySecurityResolvedUser, sourceKeyID: sourceKeyID, sourceKeyDigest: source, user: user, controlPlane: aggregate}}, nil
}
func NewSystemAuthority(operation OperationID, aggregate AggregateID) (AuthorityScope, error) {
	if !operation.value.valid() || !aggregate.value.valid() {
		return AuthorityScope{}, ErrInvalidDTO
	}
	return AuthorityScope{value: authorityScope{kind: AuthoritySystem, systemOperation: operation, controlPlane: aggregate}}, nil
}
func (v AuthorityScope) Kind() AuthorityScopeKind { return v.value.kind }
func (v AuthorityScope) UserID() UserID           { return v.value.user }
func (v AuthorityScope) AccountID() AccountID     { return v.value.account }
func (v AuthorityScope) OwnershipID() OwnershipID { return v.value.ownership }
func (v AuthorityScope) SessionID() SessionID     { return v.value.session }
func (v AuthorityScope) DeviceID() DeviceID       { return v.value.device }
func (v AuthorityScope) valid() bool {
	switch v.value.kind {
	case AuthorityOwner:
		return v.value.user.value.valid() && v.value.account.value.valid() && v.value.ownership.value.valid()
	case AuthoritySecuritySourceOnly:
		return v.value.sourceKeyID != "" && v.value.controlPlane.value.valid()
	case AuthoritySecurityResolvedUser:
		return v.value.sourceKeyID != "" && v.value.user.value.valid() && v.value.controlPlane.value.valid()
	case AuthoritySystem:
		return v.value.systemOperation.value.valid() && v.value.controlPlane.value.valid()
	default:
		return false
	}
}

type StableKeyKind uint8

const (
	StableKeyIdempotency StableKeyKind = iota + 1
	StableKeyRequestID
)

type ResultClass uint8

const (
	CredentialSuccess ResultClass = iota + 1
	ChallengeSuccess
	GenericRejection
	NonCredentialSuccess
	NonCredentialRejection
)

type ReplayDecision uint8

const (
	ClaimNew ReplayDecision = iota + 1
	ReplayExact
	IdempotencyConflict
	ReconciliationRequiredNoReexecution
)

type ClaimState uint8

const (
	ClaimedInTransaction ClaimState = iota + 1
	CommittedReplayable
	CommittedReconciliationRequired
)

type CacheState uint8

const (
	CacheActive CacheState = iota + 1
	CacheErasedAtTTL
	CacheUndecryptableReconciliationRequired
)

type ConfirmationState uint8

const (
	ConfirmationIssued ConfirmationState = iota + 1
	ConfirmationConsumed
	ConfirmationExpired
)

type ConfirmationConsumeDecision uint8

const (
	ConsumeAndConfirm ConfirmationConsumeDecision = iota + 1
	ExpireAndReject
	ConfirmationReplayExact
	DurableOperationReconciliation
	ConfirmationIdempotencyConflict
	FailClosedGenericRejection
)

type IdempotencyScope struct {
	authority AuthorityScope
	route     RouteTemplate
	keyKind   StableKeyKind
	keyDigest StableKeyDigest
}

func NewIdempotencyScope(authority AuthorityScope, route RouteTemplate, kind StableKeyKind, digest StableKeyDigest) (IdempotencyScope, error) {
	if !authority.valid() || route.value == "" || (kind != StableKeyIdempotency && kind != StableKeyRequestID) {
		return IdempotencyScope{}, ErrInvalidDTO
	}
	return IdempotencyScope{authority: authority, route: route, keyKind: kind, keyDigest: digest}, nil
}
func (v IdempotencyScope) Authority() AuthorityScope  { return v.authority }
func (v IdempotencyScope) Route() RouteTemplate       { return v.route }
func (v IdempotencyScope) KeyKind() StableKeyKind     { return v.keyKind }
func (v IdempotencyScope) KeyDigest() StableKeyDigest { return v.keyDigest }
func (v IdempotencyScope) key() string {
	a := v.authority.value
	if a.kind == AuthorityOwner {
		return strings.Join([]string{"owner", a.user.String(), a.account.String(), a.ownership.String(), v.route.String(), strconv.Itoa(int(v.keyKind)), v.keyDigest.String()}, "|")
	}
	return strings.Join([]string{strconv.Itoa(int(a.kind)), a.controlPlane.String(), v.route.String(), strconv.Itoa(int(v.keyKind)), v.keyDigest.String()}, "|")
}

type ClockSnapshot struct {
	utcNowMS         int64
	monotonicTick    uint64
	watermarkVersion uint64
}

func NewClockSnapshot(utcNowMS int64, monotonicTick, watermarkVersion uint64) (ClockSnapshot, error) {
	if utcNowMS < 0 {
		return ClockSnapshot{}, ErrInvalidDTO
	}
	return ClockSnapshot{utcNowMS, monotonicTick, watermarkVersion}, nil
}
func (v ClockSnapshot) UTCNowMS() int64          { return v.utcNowMS }
func (v ClockSnapshot) MonotonicTick() uint64    { return v.monotonicTick }
func (v ClockSnapshot) WatermarkVersion() uint64 { return v.watermarkVersion }

type CanonicalJSON struct{ bytes []byte }

func NewCanonicalJSON(raw []byte) (CanonicalJSON, error) {
	canonical, err := canonicalizeJSON(raw)
	if err != nil || !bytes.Equal(raw, canonical) {
		return CanonicalJSON{}, fmt.Errorf("%w: canonical JSON", ErrInvalidDTO)
	}
	return CanonicalJSON{bytes: append([]byte(nil), raw...)}, nil
}
func (v CanonicalJSON) Bytes() []byte { return append([]byte(nil), v.bytes...) }
func (v CanonicalJSON) valid() bool   { _, err := NewCanonicalJSON(v.bytes); return err == nil }

type ResponseHeaders struct {
	retryAfter *uint32
	setCookie  string
}

func NewResponseHeaders(retryAfter *uint32, setCookie string) (ResponseHeaders, error) {
	if setCookie != "" && len(setCookie) > 4096 {
		return ResponseHeaders{}, ErrInvalidDTO
	}
	if retryAfter != nil {
		n := *retryAfter
		return ResponseHeaders{retryAfter: &n, setCookie: setCookie}, nil
	}
	return ResponseHeaders{setCookie: setCookie}, nil
}

type ExactResponse struct {
	status  int
	body    CanonicalJSON
	headers ResponseHeaders
	class   ResultClass
}

func NewExactResponse(status int, body CanonicalJSON, headers ResponseHeaders, class ResultClass) (ExactResponse, error) {
	if status < 100 || status > 599 || !body.valid() || len(body.bytes) > 65536 || class < CredentialSuccess || class > NonCredentialRejection {
		return ExactResponse{}, ErrInvalidDTO
	}
	if headers.setCookie != "" && class != CredentialSuccess {
		return ExactResponse{}, ErrInvalidDTO
	}
	if headers.retryAfter != nil && class != GenericRejection && class != NonCredentialRejection {
		return ExactResponse{}, ErrInvalidDTO
	}
	return ExactResponse{status: status, body: body, headers: headers, class: class}, nil
}
func (v ExactResponse) Status() int              { return v.status }
func (v ExactResponse) Body() []byte             { return v.body.Bytes() }
func (v ExactResponse) ResultClass() ResultClass { return v.class }

func canonicalizeJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing JSON")
	}
	var out bytes.Buffer
	var write func(any) error
	write = func(v any) error {
		switch x := v.(type) {
		case nil:
			out.WriteString("null")
		case bool:
			out.WriteString(strconv.FormatBool(x))
		case string:
			b, err := json.Marshal(x)
			if err != nil {
				return err
			}
			out.Write(b)
		case json.Number:
			s := string(x)
			if strings.ContainsAny(s, ".eE+") {
				return errors.New("non-canonical number")
			}
			out.WriteString(s)
		case []any:
			out.WriteByte('[')
			for i, item := range x {
				if i > 0 {
					out.WriteByte(',')
				}
				if err := write(item); err != nil {
					return err
				}
			}
			out.WriteByte(']')
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					out.WriteByte(',')
				}
				b, err := json.Marshal(k)
				if err != nil {
					return err
				}
				out.Write(b)
				out.WriteByte(':')
				if err := write(x[k]); err != nil {
					return err
				}
			}
			out.WriteByte('}')
		default:
			return errors.New("unsupported JSON")
		}
		return nil
	}
	if err := write(value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
