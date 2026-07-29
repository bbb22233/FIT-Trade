// Package auth is a deliberately bounded, in-memory development identity
// service. It has no database, listener, exchange, signer, or persistence
// implementation. Plaintext credentials and tokens are never retained.
package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"fit.trade/trading-core/ownership"
	"fit.trade/trading-core/platformcontract"
	"fit.trade/trading-core/session"
	"golang.org/x/crypto/argon2"
)

var (
	ErrDenied        = errors.New("auth: denied")
	ErrThrottled     = errors.New("auth: throttled")
	ErrInvalidProof  = errors.New("auth: invalid proof")
	ErrReplay        = errors.New("auth: replay")
	ErrRefreshReuse  = errors.New("auth: refresh reuse")
	ErrRefreshExpiry = errors.New("auth: refresh expired")
	// ErrRevocationFence means the state transition completed, but at least one
	// local transport fence did not acknowledge it within RevocationBound.
	// Callers must fail closed and must not treat the revocation as available.
	ErrRevocationFence = errors.New("auth: revocation fence")
)

const (
	enrollmentDomain = "FIT_TRADE_DEVICE_ENROLLMENT_V1"
	actionDomain     = "FIT_TRADE_DEVICE_ACTION_V1"
)

type Config struct {
	// Now is injectable solely for deterministic tests. A nil value uses UTC
	// wall time.
	Now        func() time.Time
	IntentSink IntentSink
}

type Identity struct {
	UserID           string `json:"user_id"`
	TradingAccountID string `json:"trading_account_id"`
	OwnershipID      string `json:"ownership_id"`
	DeviceID         string `json:"device_id"`
	SessionID        string `json:"session_id"`
	RefreshFamilyID  string `json:"refresh_family_id"`
}

type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	DeviceID     string    `json:"device_id"`
	Identity     Identity  `json:"identity"`
}

type EnrollmentChallenge struct {
	SchemaVersion                 string    `json:"schema_version"`
	Domain                        string    `json:"domain"`
	Purpose                       string    `json:"purpose"`
	SubjectHandle                 string    `json:"subject_handle"`
	CandidatePublicKeyFingerprint string    `json:"candidate_public_key_fingerprint"`
	Nonce                         string    `json:"nonce"`
	IssuedAt                      time.Time `json:"issued_at"`
	ExpiresAt                     time.Time `json:"expires_at"`
	SingleUse                     bool      `json:"single_use"`
}

type DeviceActionChallenge struct {
	SchemaVersion    string    `json:"schema_version"`
	Domain           string    `json:"domain"`
	UserID           string    `json:"user_id"`
	TradingAccountID string    `json:"trading_account_id"`
	DeviceID         string    `json:"device_id"`
	SessionID        string    `json:"session_id"`
	Action           string    `json:"action"`
	PayloadHash      string    `json:"payload_hash"`
	Nonce            string    `json:"nonce"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	SingleUse        bool      `json:"single_use"`
}

type throttle struct {
	failures int
	windowAt time.Time
	nextAt   time.Time
	lockedAt time.Time
}

type user struct {
	id, identifier, accountID, ownershipID string
	salt, verifier                         []byte
	devices                                map[string]*device
}

type device struct {
	id, userID, fingerprint string
	publicKey               ed25519.PublicKey
	revoked                 bool
}

type family struct {
	id, userID, accountID, sessionID string
	createdAt, deadline              time.Time
	revoked                          bool
}

type authSession struct {
	id, userID, accountID, deviceID, familyID string
	revoked                                   bool
}

type access struct {
	identity Identity
	expires  time.Time
}

type refresh struct {
	familyID, sessionID string
	expires             time.Time
	rotated             bool
}

type enrollmentState struct {
	challenge   EnrollmentChallenge
	userID      string
	fingerprint string
	consumed    bool
}

type actionState struct {
	challenge DeviceActionChallenge
	consumed  bool
}

type invalidationObserver struct {
	callback func(context.Context, string) error
	mu       sync.Mutex
	inFlight map[string]*invalidationAttempt
}

type invalidationAttempt struct {
	done chan struct{}
	err  error
}

// run attaches this revocation to its observer-session attempts before waiting
// for any result. A stuck callback can therefore occupy only its own session
// entry; callbacks for other sessions are still dispatched immediately.
func (o *invalidationObserver) run(ctx context.Context, snapshotIDs []string) error {
	sessionIDs := uniqueSortedSessionIDs(snapshotIDs)
	attempts := make([]*invalidationAttempt, 0, len(sessionIDs))
	o.mu.Lock()
	if o.inFlight == nil {
		o.inFlight = make(map[string]*invalidationAttempt)
	}
	for _, sessionID := range sessionIDs {
		attempt := o.inFlight[sessionID]
		if attempt == nil {
			attempt = &invalidationAttempt{done: make(chan struct{})}
			o.inFlight[sessionID] = attempt
			go o.complete(ctx, sessionID, attempt)
		}
		attempts = append(attempts, attempt)
	}
	o.mu.Unlock()

	failed := false
	for _, attempt := range attempts {
		select {
		case <-ctx.Done():
			return ErrRevocationFence
		default:
		}
		select {
		case <-attempt.done:
			if attempt.err != nil {
				failed = true
			}
		case <-ctx.Done():
			return ErrRevocationFence
		}
	}
	select {
	case <-ctx.Done():
		return ErrRevocationFence
	default:
	}
	if failed {
		return ErrRevocationFence
	}
	return nil
}

func (o *invalidationObserver) complete(ctx context.Context, sessionID string, attempt *invalidationAttempt) {
	err := o.call(ctx, sessionID)
	o.mu.Lock()
	attempt.err = err
	if o.inFlight[sessionID] == attempt {
		delete(o.inFlight, sessionID)
	}
	close(attempt.done)
	o.mu.Unlock()
}

func (o *invalidationObserver) call(ctx context.Context, sessionID string) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrRevocationFence
		}
	}()
	return o.callback(ctx, sessionID)
}

func uniqueSortedSessionIDs(snapshotIDs []string) []string {
	unique := make(map[string]struct{}, len(snapshotIDs))
	for _, sessionID := range snapshotIDs {
		unique[sessionID] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for sessionID := range unique {
		ordered = append(ordered, sessionID)
	}
	sort.Strings(ordered)
	return ordered
}

type invalidationWork struct {
	sessionIDs []string
	observers  []*invalidationObserver
}

type Service struct {
	mu                sync.RWMutex
	now               func() time.Time
	verifierProfile   session.Argon2Profile
	usersByID         map[string]*user
	usersByIdentifier map[string]*user
	devices           map[string]*device
	keyOwners         map[string]string
	sessions          map[string]*authSession
	families          map[string]*family
	access            map[[32]byte]access
	refresh           map[[32]byte]*refresh
	enrollments       map[string]*enrollmentState
	actions           map[string]*actionState
	userThrottle      map[string]*throttle
	sourceThrottle    map[string]*throttle
	owners            *ownership.Registry
	intents           IntentSink
	// invalidationObservers are bounded local session fences used only to stop
	// already-established transports. They receive no identity or credential
	// material; callbacks run only after the auth state lock is released.
	invalidationObservers map[uint64]*invalidationObserver
	nextInvalidationID    uint64
}

func NewService(config Config) *Service {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	intents := config.IntentSink
	if intents == nil {
		intents = discardIntentSink{}
	}
	return &Service{
		now: now, verifierProfile: session.ProductionArgon2Profile(),
		usersByID: make(map[string]*user), usersByIdentifier: make(map[string]*user),
		devices: make(map[string]*device), keyOwners: make(map[string]string),
		sessions: make(map[string]*authSession), families: make(map[string]*family),
		access: make(map[[32]byte]access), refresh: make(map[[32]byte]*refresh),
		enrollments: make(map[string]*enrollmentState), actions: make(map[string]*actionState),
		userThrottle: make(map[string]*throttle), sourceThrottle: make(map[string]*throttle),
		owners: ownership.NewRegistry(), intents: intents,
		invalidationObservers: make(map[uint64]*invalidationObserver),
	}
}

// ObserveSessionInvalidation installs a local transport fence for established
// sessions. It receives only an opaque session ID. Observers are called after
// state is atomically revoked and must return when ctx is canceled; a nil or
// late error makes revocation fail closed with ErrRevocationFence. Successful
// revocation waits for all registered fences, up to session.RevocationBound.
//
// The returned function is safe to call more than once. An observer may call
// Service methods because the auth state lock is never held during callbacks.
func (s *Service) ObserveSessionInvalidation(observer func(context.Context, string) error) func() {
	if observer == nil {
		return func() {}
	}
	s.mu.Lock()
	id := s.nextInvalidationID
	s.nextInvalidationID++
	s.invalidationObservers[id] = &invalidationObserver{callback: observer}
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.invalidationObservers, id)
			s.mu.Unlock()
		})
	}
}

// addSyntheticUser is test-only provisioning. It creates a server-owned
// bootstrap device so a synthetic user can exercise login before enrollment.
// It is intentionally unexported: normal consumers cannot create credentials.
func (s *Service) addSyntheticUser(identifier, password string) (Identity, error) {
	if strings.TrimSpace(identifier) == "" || password == "" {
		return Identity{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(identifier)
	if _, exists := s.usersByIdentifier[key]; exists {
		return Identity{}, ErrDenied
	}
	salt := randomBytes(int(s.verifierProfile.SaltBytes))
	if salt == nil {
		return Identity{}, ErrDenied
	}
	u := &user{id: newID(), identifier: key, accountID: newID(), ownershipID: newID(), salt: salt, devices: make(map[string]*device)}
	u.verifier = s.derive(password, salt)
	if err := s.owners.Put(ownership.Record{UserID: u.id, TradingAccountID: u.accountID, OwnershipID: u.ownershipID, Active: true}); err != nil {
		return Identity{}, err
	}
	// A synthetic bootstrap key is never returned and exists only so login has
	// an attached registered device in this test-only service.
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	d := s.addDeviceLocked(u, public)
	s.usersByID[u.id], s.usersByIdentifier[key] = u, u
	return Identity{UserID: u.id, TradingAccountID: u.accountID, OwnershipID: u.ownershipID, DeviceID: d.id}, nil
}

func (s *Service) Login(ctx context.Context, identifier, password, source string) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowUTC()
	u := s.usersByIdentifier[strings.ToLower(identifier)]
	if err := s.allowPasswordLocked(u, source, now); err != nil {
		return Credentials{}, err
	}
	ok := s.verifyLocked(u, password)
	if !ok {
		s.recordFailureLocked(u, source, now)
		return Credentials{}, ErrDenied
	}
	s.resetUserThrottleLocked(u, now)
	if _, err := s.owners.Authorize(u.id, u.accountID); err != nil {
		return Credentials{}, ErrDenied
	}
	for _, d := range u.devices {
		if !d.revoked {
			return s.issueLocked(u, d, now)
		}
	}
	return Credentials{}, ErrDenied
}

func (s *Service) StartEnrollment(ctx context.Context, identifier, password, purpose, fingerprint, source string) (EnrollmentChallenge, error) {
	if err := ctx.Err(); err != nil {
		return EnrollmentChallenge{}, err
	}
	if purpose != "ADDITIONAL_DEVICE" && purpose != "REPLACEMENT_DEVICE" || !validFingerprint(fingerprint) {
		return EnrollmentChallenge{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowUTC()
	u := s.usersByIdentifier[strings.ToLower(identifier)]
	if err := s.allowPasswordLocked(u, source, now); err != nil {
		return s.syntheticChallenge(purpose, fingerprint, now), err
	}
	if !s.verifyLocked(u, password) || s.ownerInactiveLocked(u) {
		s.recordFailureLocked(u, source, now)
		return s.syntheticChallenge(purpose, fingerprint, now), nil
	}
	s.resetUserThrottleLocked(u, now)
	c := s.newEnrollmentChallenge(purpose, fingerprint, now)
	s.enrollments[c.SubjectHandle] = &enrollmentState{challenge: c, userID: u.id, fingerprint: fingerprint}
	return c, nil
}

func (s *Service) CompleteEnrollment(ctx context.Context, handle, publicKeyWire, signatureWire string) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	s.mu.Lock()
	locked := true
	defer func() {
		if locked {
			s.mu.Unlock()
		}
	}()
	state, ok := s.enrollments[handle]
	if !ok {
		return Credentials{}, ErrDenied
	}
	if state.consumed {
		return Credentials{}, ErrReplay
	}
	state.consumed = true // invalid proofs consume real challenges atomically.
	now := s.nowUTC()
	if !now.Before(state.challenge.ExpiresAt) {
		return Credentials{}, ErrDenied
	}
	public, err := parsePublicKey(publicKeyWire)
	if err != nil || fingerprint(public) != state.fingerprint || !verifyEnrollmentProof(public, state.challenge, signatureWire) {
		return Credentials{}, ErrInvalidProof
	}
	u := s.usersByID[state.userID]
	if u == nil || s.ownerInactiveLocked(u) {
		return Credentials{}, ErrDenied
	}
	fp := fingerprint(public)
	if _, exists := s.keyOwners[fp]; exists {
		return Credentials{}, ErrDenied
	}
	invalidated := make(map[string]struct{})
	if state.challenge.Purpose == "REPLACEMENT_DEVICE" {
		s.revokeUserLocked(u, invalidated)
	}
	d := s.addDeviceLocked(u, public)
	credentials, err := s.issueLocked(u, d, now)
	if err != nil {
		return Credentials{}, err
	}
	if state.challenge.Purpose == "REPLACEMENT_DEVICE" {
		s.emitSecurityLocked("DEVICE_REPLACED", "CRITICAL", UserSecurityScope("", u.id), now)
	} else {
		s.emitSecurityLocked("DEVICE_ENROLLED", "INFO", UserSecurityScope("", u.id), now)
	}
	work := s.snapshotInvalidationWorkLocked(invalidated)
	s.mu.Unlock()
	locked = false
	if err := runInvalidationWork(work); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

func (s *Service) Refresh(ctx context.Context, plaintext string) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	digest := sha256.Sum256([]byte(plaintext))
	s.mu.Lock()
	locked := true
	defer func() {
		if locked {
			s.mu.Unlock()
		}
	}()
	r, ok := s.refresh[digest]
	if !ok {
		return Credentials{}, ErrDenied
	}
	f := s.families[r.familyID]
	now := s.nowUTC()
	if f == nil || f.revoked || !now.Before(f.deadline) {
		return Credentials{}, ErrRefreshExpiry
	}
	if r.rotated {
		invalidated := make(map[string]struct{})
		s.revokeFamilyLocked(f, invalidated)
		s.emitSecurityLocked("REFRESH_REUSE_DETECTED", "CRITICAL", UserSecurityScope("", f.userID), now)
		work := s.snapshotInvalidationWorkLocked(invalidated)
		s.mu.Unlock()
		locked = false
		_ = runInvalidationWork(work) // refresh reuse is already fail-closed.
		return Credentials{}, ErrRefreshReuse
	}
	if !now.Before(r.expires) {
		return Credentials{}, ErrRefreshExpiry
	}
	se := s.sessions[r.sessionID]
	if se == nil || se.revoked || !s.activeSessionLocked(se) {
		return Credentials{}, ErrDenied
	}
	u := s.usersByID[se.userID]
	d := s.devices[se.deviceID]
	if u == nil || d == nil || d.revoked {
		return Credentials{}, ErrDenied
	}
	r.rotated = true
	return s.issueForSessionLocked(u, d, se, f, now)
}

// RefreshFamilyID is intentionally a digest-only lookup used solely by the
// transport rate limiter.  It never returns a token or any owner identity.
func (s *Service) RefreshFamilyID(plaintext string) (string, bool) {
	digest := sha256.Sum256([]byte(plaintext))
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.refresh[digest]
	if !ok || r == nil || r.familyID == "" {
		return "", false
	}
	return r.familyID, true
}

func (s *Service) Authenticate(accessToken string) (Identity, error) {
	d := sha256.Sum256([]byte(accessToken))
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.access[d]
	if !ok || !s.nowUTC().Before(a.expires) || !s.activeIdentityLocked(a.identity) {
		return Identity{}, ErrDenied
	}
	return a.identity, nil
}

func (s *Service) RevokeSession(actor Identity, sessionID string) error {
	s.mu.Lock()
	if !s.activeIdentityLocked(actor) {
		s.mu.Unlock()
		return ErrDenied
	}
	target := s.sessions[sessionID]
	if target == nil || target.userID != actor.UserID || target.accountID != actor.TradingAccountID {
		s.mu.Unlock()
		return ErrDenied
	}
	invalidated := make(map[string]struct{})
	s.revokeSessionLocked(target, invalidated)
	if f := s.families[target.familyID]; f != nil {
		s.revokeFamilyLocked(f, invalidated)
	}
	s.emitSecurityLocked("SESSION_REVOKED", "INFO", UserSecurityScope("", actor.UserID), s.nowUTC())
	work := s.snapshotInvalidationWorkLocked(invalidated)
	s.mu.Unlock()
	return runInvalidationWork(work)
}

func (s *Service) RevokeDevice(actor Identity, deviceID string) error {
	s.mu.Lock()
	if !s.activeIdentityLocked(actor) {
		s.mu.Unlock()
		return ErrDenied
	}
	d := s.devices[deviceID]
	if d == nil || d.userID != actor.UserID {
		s.mu.Unlock()
		return ErrDenied
	}
	d.revoked = true
	invalidated := make(map[string]struct{})
	for _, se := range s.sessions {
		if se.deviceID == d.id {
			s.revokeSessionLocked(se, invalidated)
			if f := s.families[se.familyID]; f != nil {
				s.revokeFamilyLocked(f, invalidated)
			}
		}
	}
	s.emitSecurityLocked("DEVICE_REVOKED", "WARNING", UserSecurityScope("", actor.UserID), s.nowUTC())
	work := s.snapshotInvalidationWorkLocked(invalidated)
	s.mu.Unlock()
	return runInvalidationWork(work)
}

func (s *Service) IssueDeviceAction(accessToken, action, payloadHash string) (DeviceActionChallenge, error) {
	if (action != "OPEN_POSITION" && action != "ADD_POSITION") || !validDigest(payloadHash) {
		return DeviceActionChallenge{}, ErrDenied
	}
	id, err := s.Authenticate(accessToken)
	if err != nil {
		return DeviceActionChallenge{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowUTC()
	c := DeviceActionChallenge{SchemaVersion: "fit.platform.device-action-challenge.v1", Domain: actionDomain, UserID: id.UserID, TradingAccountID: id.TradingAccountID, DeviceID: id.DeviceID, SessionID: id.SessionID, Action: action, PayloadHash: payloadHash, Nonce: randomToken(24), IssuedAt: now, ExpiresAt: now.Add(120 * time.Second), SingleUse: true}
	s.actions[c.Nonce] = &actionState{challenge: c}
	return c, nil
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func (s *Service) CompleteDeviceAction(accessToken, nonce, signatureWire string) error {
	id, err := s.Authenticate(accessToken)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.actions[nonce]
	if !ok {
		return ErrDenied
	}
	if state.consumed {
		return ErrReplay
	}
	state.consumed = true
	if !s.nowUTC().Before(state.challenge.ExpiresAt) || state.challenge.UserID != id.UserID || state.challenge.TradingAccountID != id.TradingAccountID || state.challenge.DeviceID != id.DeviceID || state.challenge.SessionID != id.SessionID {
		return ErrDenied
	}
	d := s.devices[id.DeviceID]
	if d == nil || d.revoked || !verifyActionProof(d.publicKey, state.challenge, signatureWire) {
		return ErrInvalidProof
	}
	return nil
}

func (s *Service) AccessExpiry(accessToken string) (time.Time, error) {
	d := sha256.Sum256([]byte(accessToken))
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.access[d]
	if !ok || !s.activeIdentityLocked(a.identity) {
		return time.Time{}, ErrDenied
	}
	return a.expires, nil
}

func (s *Service) SameActiveBinding(accessToken string, current Identity) (Identity, time.Time, error) {
	id, err := s.Authenticate(accessToken)
	if err != nil {
		return Identity{}, time.Time{}, err
	}
	if id.UserID != current.UserID || id.TradingAccountID != current.TradingAccountID || id.DeviceID != current.DeviceID || id.SessionID != current.SessionID || id.RefreshFamilyID != current.RefreshFamilyID {
		return Identity{}, time.Time{}, ErrDenied
	}
	expires, err := s.AccessExpiry(accessToken)
	return id, expires, err
}

// Active is used by the websocket harness to make revocation observable on
// established connections without trusting a frame from that connection.
func (s *Service) Active(identity Identity) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeIdentityLocked(identity)
}

func (s *Service) derive(password string, salt []byte) []byte {
	profile := s.verifierProfile
	return argon2.IDKey([]byte(password), salt, profile.Iterations, profile.MemoryKiB, profile.Parallelism, profile.KeyBytes)
}

// Argon2Profile reports the frozen verifier profile selected by NewService.
func (s *Service) Argon2Profile() session.Argon2Profile { return s.verifierProfile }

func (s *Service) verifyLocked(u *user, password string) bool {
	// Always execute Argon2id, including the unknown-user path.
	if u == nil {
		profile := s.verifierProfile
		dummy := s.derive(password, make([]byte, profile.SaltBytes))
		return subtle.ConstantTimeCompare(dummy, make([]byte, profile.KeyBytes)) == 1
	}
	return subtle.ConstantTimeCompare(s.derive(password, u.salt), u.verifier) == 1
}

func (s *Service) issueLocked(u *user, d *device, now time.Time) (Credentials, error) {
	se := &authSession{id: newID(), userID: u.id, accountID: u.accountID, deviceID: d.id, familyID: newID()}
	f := &family{id: se.familyID, userID: u.id, accountID: u.accountID, sessionID: se.id, createdAt: now, deadline: now.Add(session.FamilyMaximumTTL)}
	s.sessions[se.id], s.families[f.id] = se, f
	return s.issueForSessionLocked(u, d, se, f, now)
}

func (s *Service) issueForSessionLocked(u *user, d *device, se *authSession, f *family, now time.Time) (Credentials, error) {
	accessToken, refreshToken := randomToken(32), randomToken(32)
	if accessToken == "" || refreshToken == "" {
		return Credentials{}, ErrDenied
	}
	id := Identity{UserID: u.id, TradingAccountID: u.accountID, OwnershipID: u.ownershipID, DeviceID: d.id, SessionID: se.id, RefreshFamilyID: f.id}
	expires := now.Add(session.AccessTTL)
	s.access[sha256.Sum256([]byte(accessToken))] = access{identity: id, expires: expires}
	rexp := now.Add(session.RefreshTTL)
	if rexp.After(f.deadline) {
		rexp = f.deadline
	}
	s.refresh[sha256.Sum256([]byte(refreshToken))] = &refresh{familyID: f.id, sessionID: se.id, expires: rexp}
	return Credentials{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expires, DeviceID: d.id, Identity: id}, nil
}

func (s *Service) addDeviceLocked(u *user, public ed25519.PublicKey) *device {
	d := &device{id: newID(), userID: u.id, publicKey: append(ed25519.PublicKey(nil), public...), fingerprint: fingerprint(public)}
	u.devices[d.id], s.devices[d.id], s.keyOwners[d.fingerprint] = d, d, u.id
	return d
}

func (s *Service) revokeUserLocked(u *user, invalidated map[string]struct{}) {
	for _, d := range u.devices {
		d.revoked = true
	}
	for _, se := range s.sessions {
		if se.userID == u.id {
			s.revokeSessionLocked(se, invalidated)
		}
	}
	for _, f := range s.families {
		if f.userID == u.id {
			s.revokeFamilyLocked(f, invalidated)
		}
	}
}
func (s *Service) revokeSessionLocked(se *authSession, invalidated map[string]struct{}) {
	if se == nil || se.revoked {
		return
	}
	se.revoked = true
	invalidated[se.id] = struct{}{}
}
func (s *Service) revokeFamilyLocked(f *family, invalidated map[string]struct{}) {
	if f == nil || f.revoked {
		return
	}
	f.revoked = true
	// A family is exactly one server-authored session in this bounded phase.
	// Reuse makes that session inactive even if its session record has not yet
	// been marked revoked, so the WebSocket fence must observe it as well.
	if se := s.sessions[f.sessionID]; se == nil || !se.revoked {
		invalidated[f.sessionID] = struct{}{}
	}
}

// snapshotInvalidationWorkLocked captures exactly the observers relevant to
// already-revoked sessions. Callers must execute the result after unlocking.
func (s *Service) snapshotInvalidationWorkLocked(invalidated map[string]struct{}) invalidationWork {
	if len(invalidated) == 0 || len(s.invalidationObservers) == 0 {
		return invalidationWork{}
	}
	work := invalidationWork{
		sessionIDs: make([]string, 0, len(invalidated)),
		observers:  make([]*invalidationObserver, 0, len(s.invalidationObservers)),
	}
	for sessionID := range invalidated {
		work.sessionIDs = append(work.sessionIDs, sessionID)
	}
	sort.Strings(work.sessionIDs)
	for _, observer := range s.invalidationObservers {
		work.observers = append(work.observers, observer)
	}
	return work
}

// runInvalidationWork gives every observer callback the same frozen deadline.
// A result channel is fully buffered so observers that finish after its caller
// has failed closed never block a sender.
func runInvalidationWork(work invalidationWork) error {
	if len(work.sessionIDs) == 0 || len(work.observers) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), session.RevocationBound)
	defer cancel()
	results := make(chan error, len(work.observers))
	for _, observer := range work.observers {
		go func(observer *invalidationObserver) {
			results <- observer.run(ctx, work.sessionIDs)
		}(observer)
	}
	failed := false
	remaining := len(work.observers)
	for remaining > 0 {
		select {
		case err := <-results:
			remaining--
			if err != nil {
				failed = true
			}
		case <-ctx.Done():
			return ErrRevocationFence
		}
	}
	if failed {
		return ErrRevocationFence
	}
	return nil
}
func (s *Service) ownerInactiveLocked(u *user) bool {
	if u == nil {
		return true
	}
	_, err := s.owners.Authorize(u.id, u.accountID)
	return err != nil
}
func (s *Service) activeSessionLocked(se *authSession) bool {
	if se == nil || se.revoked {
		return false
	}
	f := s.families[se.familyID]
	d := s.devices[se.deviceID]
	u := s.usersByID[se.userID]
	return f != nil && !f.revoked && d != nil && !d.revoked && !s.ownerInactiveLocked(u)
}
func (s *Service) activeIdentityLocked(id Identity) bool {
	se := s.sessions[id.SessionID]
	return se != nil && se.userID == id.UserID && se.accountID == id.TradingAccountID && se.deviceID == id.DeviceID && se.familyID == id.RefreshFamilyID && s.activeSessionLocked(se)
}

func (s *Service) allowPasswordLocked(u *user, source string, now time.Time) error {
	if source == "" {
		return ErrDenied
	}
	if throttled(s.sourceThrottle[source], now) || (u != nil && throttled(s.userThrottle[u.id], now)) {
		return ErrThrottled
	}
	return nil
}
func throttled(v *throttle, now time.Time) bool {
	if v == nil {
		return false
	}
	if !v.lockedAt.IsZero() && !now.Before(v.lockedAt) {
		*v = throttle{}
	}
	return now.Before(v.nextAt) || now.Before(v.lockedAt)
}
func (s *Service) recordFailureLocked(u *user, source string, now time.Time) {
	if s.recordOneLocked(s.sourceThrottle, source, now) {
		s.emitSecurityLocked("LOGIN_SOURCE_LOCKED", "WARNING", SourceSecurityScope(source), now)
	}
	if u != nil {
		if s.recordOneLocked(s.userThrottle, u.id, now) {
			s.emitSecurityLocked("LOGIN_ACCOUNT_LOCKED", "WARNING", UserSecurityScope(source, u.id), now)
		}
	}
}
func (s *Service) recordOneLocked(m map[string]*throttle, key string, now time.Time) bool {
	v := m[key]
	if v == nil || now.Sub(v.windowAt) >= 15*time.Minute {
		v = &throttle{windowAt: now}
		m[key] = v
	}
	v.failures++
	if v.failures >= 5 {
		v.lockedAt = now.Add(15 * time.Minute)
		v.nextAt = time.Time{}
		return true
	}
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	v.nextAt = now.Add(delays[v.failures-1])
	return false
}
func (s *Service) resetUserThrottleLocked(u *user, now time.Time) {
	if u != nil {
		if v := s.userThrottle[u.id]; v != nil {
			*v = throttle{windowAt: now}
		}
	}
}

func (s *Service) newEnrollmentChallenge(purpose, fp string, now time.Time) EnrollmentChallenge {
	return EnrollmentChallenge{SchemaVersion: "fit.platform.enrollment-challenge.v1", Domain: enrollmentDomain, Purpose: purpose, SubjectHandle: randomToken(24), CandidatePublicKeyFingerprint: fp, Nonce: randomToken(24), IssuedAt: now, ExpiresAt: now.Add(120 * time.Second), SingleUse: true}
}
func (s *Service) syntheticChallenge(purpose, fp string, now time.Time) EnrollmentChallenge {
	return s.newEnrollmentChallenge(purpose, fp, now)
}
func (s *Service) emitSecurityLocked(kind, severity string, scope SecurityScope, now time.Time) {
	s.intents.Record(AuditIntent{kind: kind, scope: scope, occurredAt: now}, NotificationIntent{kind: kind, severity: severity, scope: scope, occurredAt: now})
}
func (s *Service) nowUTC() time.Time { return s.now().UTC().Truncate(time.Millisecond) }

func randomBytes(n int) []byte {
	v := make([]byte, n)
	if _, err := rand.Read(v); err != nil {
		return nil
	}
	return v
}
func randomToken(n int) string {
	v := randomBytes(n)
	if v == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(v)
}
func newID() string {
	b := randomBytes(16)
	if b == nil {
		panic("crypto/rand unavailable")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func fingerprint(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return "ed25519:" + hex.EncodeToString(sum[:])
}
func validFingerprint(v string) bool {
	if !strings.HasPrefix(v, "ed25519:") || len(v) != len("ed25519:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(v, "ed25519:"))
	return err == nil && strings.ToLower(v) == v
}
func parsePublicKey(wire string) (ed25519.PublicKey, error) {
	raw := strings.TrimPrefix(wire, "ed25519-public:")
	if raw == wire || len(raw) != 64 || strings.ToLower(raw) != raw {
		return nil, ErrInvalidProof
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, ErrInvalidProof
	}
	return ed25519.PublicKey(b), nil
}
func parseSignature(wire string) ([]byte, error) {
	raw := strings.TrimPrefix(wire, "ed25519-signature:")
	if raw == wire || len(raw) != 128 || strings.ToLower(raw) != raw {
		return nil, ErrInvalidProof
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != ed25519.SignatureSize {
		return nil, ErrInvalidProof
	}
	return b, nil
}
func signedMessage(domain string, value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, ErrInvalidProof
	}
	canonical := platformcontract.CanonicalJSON(decoded)
	if canonical == "" {
		return nil, ErrInvalidProof
	}
	return append(append([]byte(domain), 0), []byte(canonical)...), nil
}
func verifyEnrollmentProof(public ed25519.PublicKey, c EnrollmentChallenge, wire string) bool {
	sig, err := parseSignature(wire)
	if err != nil {
		return false
	}
	msg, err := signedMessage(enrollmentDomain, c)
	return err == nil && ed25519.Verify(public, msg, sig)
}
func verifyActionProof(public ed25519.PublicKey, c DeviceActionChallenge, wire string) bool {
	sig, err := parseSignature(wire)
	if err != nil {
		return false
	}
	msg, err := signedMessage(actionDomain, c)
	return err == nil && ed25519.Verify(public, msg, sig)
}
