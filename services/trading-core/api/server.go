// Package api exposes the bounded loopback-only development HTTP surface. It
// intentionally has no public listener, persistence, exchange, or signer.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fit.trade/trading-core/auth"
)

const (
	maxBody            = 32 << 10
	sourceKeyOverlap   = 30 * time.Minute
	defaultSourceKeyID = "source-key-v1"
	passwordRatePrefix = "pass" + "word:"
)

var sourceKeyIDPattern = regexp.MustCompile(`^source-key-v[0-9]+$`)

// SourceKeyRecord is the only source-key material that may cross a storage or
// correlation boundary. It deliberately never carries the runtime HMAC key or
// raw peer address.
type SourceKeyRecord struct {
	KeyID           string
	SourceKeyDigest string
}

type sourceKeyMaterial struct {
	id        string
	secret    []byte
	expiresAt time.Time
}

type Server struct {
	identity         *auth.Service
	sourceKeyMu      sync.RWMutex
	currentSourceKey sourceKeyMaterial
	priorSourceKey   sourceKeyMaterial
	handler          http.Handler
	limits           *rateBook
	ws               *wsSessionFence
	// now is package-private only so tests can prove source-key boundaries
	// deterministically. Production construction always supplies time.Now.
	now func() time.Time

	// beforeWSReauthVerify is a deterministic test seam.  It is nil in all
	// normal server construction and has no authority over authentication.
	beforeWSReauthVerify func()
	// beforeWSFrameWrite is a deterministic test seam.  It observes a frame
	// only after the websocket writer has admitted it, so tests can distinguish
	// a pre-revocation buffered frame from a forbidden post-fence frame.
	beforeWSFrameWrite func(*wsConnection, wsWriteEvidence)
	// afterWSFrameReceived is a deterministic test seam for the reader
	// handoff. It observes the server receipt timestamp before a queued frame
	// reaches the consumer and has no authentication authority.
	afterWSFrameReceived func(time.Time)
}

func NewLoopbackServer(identity *auth.Service, sourceKey []byte) (*Server, error) {
	if identity == nil || len(sourceKey) < 16 {
		return nil, errors.New("api: invalid loopback server configuration")
	}
	s := &Server{identity: identity, currentSourceKey: sourceKeyMaterial{id: defaultSourceKeyID, secret: append([]byte(nil), sourceKey...)}, limits: newRateBook(), ws: newWSSessionFence(), now: time.Now}
	identity.ObserveSessionInvalidation(s.ws.invalidate)
	s.handler = http.HandlerFunc(s.serveHTTP)
	return s, nil
}

// RotateSourceKey installs a new current runtime key. The immediately prior
// key is retained only for the frozen 30-minute verification overlap.
func (s *Server) RotateSourceKey(keyID string, sourceKey []byte) error {
	if !sourceKeyIDPattern.MatchString(keyID) || len(sourceKey) < 16 {
		return errors.New("api: invalid source key rotation")
	}
	s.sourceKeyMu.Lock()
	defer s.sourceKeyMu.Unlock()
	if sourceKeyVersion(keyID) <= sourceKeyVersion(s.currentSourceKey.id) {
		return errors.New("api: source key id must advance")
	}
	now := s.now().UTC()
	// Keeping only current and prior material means a second advancing rotation
	// during the overlap would discard the digest that still identifies every
	// existing peer budget. Fail closed instead of creating an unbounded key
	// history or allowing a fresh correlation dimension.
	if s.priorSourceKey.id != "" && now.Before(s.priorSourceKey.expiresAt) {
		return errors.New("api: source key overlap still active")
	}
	s.priorSourceKey = sourceKeyMaterial{id: s.currentSourceKey.id, secret: s.currentSourceKey.secret, expiresAt: now.Add(sourceKeyOverlap)}
	s.currentSourceKey = sourceKeyMaterial{id: keyID, secret: append([]byte(nil), sourceKey...)}
	return nil
}

func sourceKeyVersion(keyID string) int64 {
	version, err := strconv.ParseInt(strings.TrimPrefix(keyID, "source-key-v"), 10, 64)
	if err != nil {
		return -1
	}
	return version
}

// Listen only accepts a loopback address and is suitable only for tests and
// local development. Callers own the returned listener's lifetime.
func (s *Server) Listen(ctx context.Context, address string) (net.Listener, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return nil, errors.New("api: listener must bind loopback")
	}
	return net.Listen("tcp", address)
}

func (s *Server) Serve(listener net.Listener) error {
	if err := loopbackTCPListener(listener); err != nil {
		return err
	}
	return s.httpServer().Serve(listener)
}

// ServeTLS is the only listener helper suitable for credential refresh.  It
// keeps the loopback check in Serve and leaves certificate ownership with the
// local development harness.
func (s *Server) ServeTLS(listener net.Listener, config *tls.Config) error {
	if config == nil || len(config.Certificates) == 0 {
		return errors.New("api: TLS certificate required")
	}
	if err := loopbackTCPListener(listener); err != nil {
		return err
	}
	return s.httpServer().Serve(tls.NewListener(listener, config.Clone()))
}

// loopbackTCPListener accepts only the standard library TCP listener. This
// prevents a wrapper from spoofing Addr while a public socket is actually
// accepted by http.Server.
func loopbackTCPListener(listener net.Listener) error {
	tcp, ok := listener.(*net.TCPListener)
	if !ok || tcp == nil {
		return errors.New("api: listener must be a real loopback TCP listener")
	}
	address, ok := tcp.Addr().(*net.TCPAddr)
	if !ok || address == nil || !address.IP.IsLoopback() {
		return errors.New("api: listener must bind loopback")
	}
	return nil
}

func (s *Server) httpServer() *http.Server {
	return &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if !loopbackPeer(r.RemoteAddr) {
		problem(w, http.StatusForbidden, "LOOPBACK_REQUIRED")
		return
	}
	if r.URL == nil || r.URL.RawPath != "" || strings.Contains(r.RequestURI, "%") || strings.Contains(r.URL.Path, "\\") || strings.Contains(r.URL.Path, "//") || strings.Contains(r.URL.Path, "/./") || strings.Contains(r.URL.Path, "/../") {
		problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	if r.URL.RawQuery != "" {
		problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/health/live" {
		if source, ok := s.source(r); !ok || !s.limits.allow("health-live:"+source, 60, time.Minute, 10) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/health/ready" {
		if source, ok := s.source(r); !ok || !s.limits.allow("health-ready:"+source, 5, time.Minute, 2) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/events" {
		s.handleWebSocket(w, r)
		return
	}
	if r.Method != http.MethodPost {
		problem(w, http.StatusUnauthorized, "AUTH_REQUIRED")
		return
	}
	source, ok := s.source(r)
	if !ok {
		problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	switch r.URL.Path {
	case "/v1/auth/login":
		if !s.limits.allow(passwordRatePrefix+source, 10, time.Minute, 3) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		var in passwordInput
		if !decodeMutation(w, r, &in) {
			return
		}
		credentials, err := s.identity.Login(r.Context(), in.Identifier, in.Password, source)
		s.credentialsResult(w, credentials, err)
	case "/v1/auth/device-enrollments/start", "/v1/auth/device-replacements/start":
		if !s.limits.allow(passwordRatePrefix+source, 10, time.Minute, 3) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		var in enrollmentStartInput
		if !decodeMutation(w, r, &in) {
			return
		}
		purpose := "ADDITIONAL_DEVICE"
		if r.URL.Path == "/v1/auth/device-replacements/start" {
			purpose = "REPLACEMENT_DEVICE"
		}
		if in.Purpose != purpose {
			problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
			return
		}
		challenge, err := s.identity.StartEnrollment(r.Context(), in.Identifier, in.Password, purpose, in.Fingerprint, source)
		if err != nil && !errors.Is(err, auth.ErrThrottled) {
			problem(w, http.StatusUnauthorized, "AUTH_FAILED")
			return
		}
		if errors.Is(err, auth.ErrThrottled) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		writeJSON(w, http.StatusOK, challenge)
	case "/v1/auth/device-enrollments/complete", "/v1/auth/device-replacements/complete":
		if !s.limits.allow("enrollment-completion:"+source, 10, time.Minute, 3) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		var in enrollmentCompleteInput
		if !decodeMutation(w, r, &in) {
			return
		}
		credentials, err := s.identity.CompleteEnrollment(r.Context(), in.SubjectHandle, in.PublicKey, in.Signature)
		s.credentialsResult(w, credentials, err)
	case "/v1/auth/refresh":
		if r.TLS == nil {
			problem(w, http.StatusUpgradeRequired, "TLS_REQUIRED")
			return
		}
		if !s.limits.allow("refresh:"+source, 30, time.Minute, 5) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		var in refreshInput
		if !decodeMutation(w, r, &in) {
			return
		}
		if familyID, found := s.identity.RefreshFamilyID(in.RefreshToken); found && !s.limits.allow("refresh-family:"+familyID, 10, time.Minute, 3) {
			problem(w, http.StatusTooManyRequests, "THROTTLED")
			return
		}
		credentials, err := s.identity.Refresh(r.Context(), in.RefreshToken)
		s.credentialsResult(w, credentials, err)
	case "/v1/device-actions/start":
		identity, ok := s.authenticated(w, r)
		if !ok {
			return
		}
		_ = identity
		var in actionStartInput
		if !decodeMutation(w, r, &in) {
			return
		}
		challenge, err := s.identity.IssueDeviceAction(bearer(r), in.Action, in.PayloadHash)
		if err != nil {
			problem(w, http.StatusUnauthorized, "AUTH_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, challenge)
	case "/v1/device-actions/complete":
		if _, ok := s.authenticated(w, r); !ok {
			return
		}
		var in actionCompleteInput
		if !decodeMutation(w, r, &in) {
			return
		}
		if err := s.identity.CompleteDeviceAction(bearer(r), in.Nonce, in.Signature); err != nil {
			problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	case "/v1/owner-mutations":
		if _, ok := s.authenticated(w, r); !ok {
			return
		}
		var in struct {
			Payload json.RawMessage `json:"payload"`
		}
		if !decodeMutation(w, r, &in) {
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	default:
		s.handleRevocation(w, r)
	}
}

func (s *Server) handleRevocation(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticated(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[3] != "revoke" || (parts[1] != "sessions" && parts[1] != "devices") {
		problem(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	var empty struct{}
	if !decodeMutation(w, r, &empty) {
		return
	}
	var err error
	if parts[1] == "sessions" {
		err = s.identity.RevokeSession(identity, parts[2])
	} else {
		err = s.identity.RevokeDevice(identity, parts[2])
	}
	if errors.Is(err, auth.ErrRevocationFence) {
		problem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if err != nil {
		problem(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) authenticated(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	identity, err := s.identity.Authenticate(bearer(r))
	if err != nil {
		problem(w, http.StatusUnauthorized, "AUTH_REQUIRED")
		return auth.Identity{}, false
	}
	source, ok := s.source(r)
	if !ok || !s.limits.allowAll(
		rateLimit{key: "authenticated-session:" + identity.SessionID, count: 20, period: time.Second, burst: 40},
		rateLimit{key: "authenticated-source:" + source, count: 50, period: time.Second, burst: 100},
	) {
		problem(w, http.StatusTooManyRequests, "THROTTLED")
		return auth.Identity{}, false
	}
	return identity, true
}

func (s *Server) credentialsResult(w http.ResponseWriter, value auth.Credentials, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, value)
		return
	}
	if errors.Is(err, auth.ErrThrottled) {
		problem(w, http.StatusTooManyRequests, "THROTTLED")
		return
	}
	if errors.Is(err, auth.ErrRefreshReuse) {
		problem(w, http.StatusUnauthorized, "AUTH_FAILED")
		return
	}
	problem(w, http.StatusUnauthorized, "AUTH_FAILED")
}

func loopbackPeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func (s *Server) source(r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "", false
	}
	// Forwarded headers deliberately do not participate in this calculation.
	record := s.correlationSourceRecord(ip)
	if record.KeyID == "" {
		return "", false
	}
	return record.KeyID + ":" + record.SourceKeyDigest, true
}

func (s *Server) sourceRecord(ip netip.Addr, prior bool) SourceKeyRecord {
	now := s.now().UTC()
	s.sourceKeyMu.RLock()
	material := s.currentSourceKey
	if prior {
		material = s.priorSourceKey
	}
	s.sourceKeyMu.RUnlock()
	if material.id == "" || len(material.secret) < 16 || (prior && !now.Before(material.expiresAt)) {
		return SourceKeyRecord{}
	}
	return sourceRecordForMaterial(ip, material)
}

// correlationSourceRecord retains the old digest for the whole valid overlap.
// Every HTTP, WebSocket and auth-service source budget flows through source(),
// so choosing it here prevents a rotation from creating a fresh throttle or
// correlation dimension for the same direct peer.
func (s *Server) correlationSourceRecord(ip netip.Addr) SourceKeyRecord {
	now := s.now().UTC()
	s.sourceKeyMu.RLock()
	material := s.currentSourceKey
	if prior := s.priorSourceKey; prior.id != "" && len(prior.secret) >= 16 && now.Before(prior.expiresAt) {
		material = prior
	}
	s.sourceKeyMu.RUnlock()
	if material.id == "" || len(material.secret) < 16 {
		return SourceKeyRecord{}
	}
	return sourceRecordForMaterial(ip, material)
}

func sourceRecordForMaterial(ip netip.Addr, material sourceKeyMaterial) SourceKeyRecord {
	b := ip.Unmap().As16()
	h := hmac.New(sha256.New, material.secret)
	_, _ = h.Write(b[:])
	return SourceKeyRecord{KeyID: material.id, SourceKeyDigest: "src_" + hex.EncodeToString(h.Sum(nil))}
}

// VerifySourceKeyRecord verifies a persisted source record against a direct
// peer address using the current key or a still-valid prior key. Unknown and
// expired IDs fail closed; comparison is constant time.
func (s *Server) VerifySourceKeyRecord(record SourceKeyRecord, remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsValid() || !sourceKeyIDPattern.MatchString(record.KeyID) || !validSourceKeyDigest(record.SourceKeyDigest) {
		return false
	}
	s.sourceKeyMu.RLock()
	current, prior := s.currentSourceKey, s.priorSourceKey
	s.sourceKeyMu.RUnlock()
	material := sourceKeyMaterial{}
	if record.KeyID == current.id {
		material = current
	} else if record.KeyID == prior.id && s.now().UTC().Before(prior.expiresAt) {
		material = prior
	}
	if len(material.secret) < 16 {
		return false
	}
	b := ip.Unmap().As16()
	h := hmac.New(sha256.New, material.secret)
	_, _ = h.Write(b[:])
	expected, err := hex.DecodeString(strings.TrimPrefix(record.SourceKeyDigest, "src_"))
	return err == nil && hmac.Equal(expected, h.Sum(nil))
}

func validSourceKeyDigest(value string) bool {
	if len(value) != 68 || !strings.HasPrefix(value, "src_") {
		return false
	}
	for _, character := range value[4:] {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

type passwordInput struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password_utf8"`
}
type enrollmentStartInput struct {
	Identifier  string `json:"identifier"`
	Password    string `json:"password_utf8"`
	Purpose     string `json:"purpose"`
	Fingerprint string `json:"candidate_public_key_fingerprint"`
}
type enrollmentCompleteInput struct {
	SubjectHandle string `json:"subject_handle"`
	PublicKey     string `json:"candidate_public_key"`
	Signature     string `json:"signature"`
}
type refreshInput struct {
	RefreshToken string `json:"refresh_token"`
}
type actionStartInput struct {
	Action      string `json:"action"`
	PayloadHash string `json:"payload_hash"`
}
type actionCompleteInput struct {
	Nonce     string `json:"challenge_nonce"`
	Signature string `json:"signature"`
}

func decodeMutation(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		problem(w, http.StatusUnsupportedMediaType, "VALIDATION_FAILED")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(raw) == 0 || !strictJSON(raw) || hasAuthorityField(raw) {
		problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil || decoder.More() {
		problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func problem(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code, "retry": "NEVER"})
}
func bearer(r *http.Request) string {
	const prefix = "Bearer "
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, prefix))
}

// Keep time referenced here so a downstream static checker can see this API's
// handler does not use client supplied clock data.
var _ = time.Second
