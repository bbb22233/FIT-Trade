package platformcontract

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"
	"sync"
)

// VerifyChallengeProof verifies the frozen Ed25519 wire proof for either
// challenge domain. It is verification only: no private key is accepted,
// created, logged, or persisted.
func VerifyChallengeProof(challenge map[string]any, publicKeyWire, signatureWire string) (bool, error) {
	const pk = "ed25519-public:"
	const sig = "ed25519-signature:"
	if !strings.HasPrefix(publicKeyWire, pk) || !strings.HasPrefix(signatureWire, sig) {
		return false, fmt.Errorf("platformcontract: invalid proof wire")
	}
	publicHex := strings.TrimPrefix(publicKeyWire, pk)
	p, e := hex.DecodeString(publicHex)
	if e != nil || len(p) != ed25519.PublicKeySize ||
		publicKeyWire != pk+hex.EncodeToString(p) {
		return false, fmt.Errorf("platformcontract: invalid public key")
	}
	if fingerprint, exists := challenge["candidate_public_key_fingerprint"]; exists {
		expected, ok := fingerprint.(string)
		actual := fmt.Sprintf("ed25519:%x", sha256.Sum256(p))
		if !ok || expected != actual {
			return false, nil
		}
	}
	signatureHex := strings.TrimPrefix(signatureWire, sig)
	s, e := hex.DecodeString(signatureHex)
	if e != nil || len(s) != ed25519.SignatureSize ||
		signatureWire != sig+hex.EncodeToString(s) {
		return false, fmt.Errorf("platformcontract: invalid signature")
	}
	b, e := ChallengeSigningBytes(challenge)
	if e != nil {
		return false, e
	}
	return ed25519.Verify(ed25519.PublicKey(p), b, s), nil
}

// ChallengeDecision is the state-machine result; it does not consume storage.
func ChallengeDecision(synthetic, attempted, proofOK bool, nowMS, expiresMS int64) string {
	if synthetic {
		return "REJECT_SYNTHETIC_NO_STATE"
	}
	if attempted {
		return "REJECT_REPLAY"
	}
	if nowMS >= expiresMS {
		return "REJECT_EXPIRED"
	}
	if !proofOK {
		return "CONSUMED_INVALID_PROOF"
	}
	return "CONSUMED_SUCCESS"
}
func RefreshDecision(status string, nowMS, individualExpiryMS, familyDeadlineMS int64) string {
	if nowMS >= familyDeadlineMS {
		return "REJECT_FAMILY_EXPIRED"
	}
	if status == "REVOKED" {
		return "REJECT_REVOKED"
	}
	if status == "ROTATED" {
		return "REVOKE_FAMILY_AND_DESCENDANTS"
	}
	if nowMS >= individualExpiryMS {
		return "REJECT_EXPIRED_NO_ROTATION"
	}
	return "ROTATE_CREATE_DESCENDANT"
}
func RefreshExpiry(issuedMS, familyDeadlineMS int64) int64 {
	x := issuedMS + 604800000
	if familyDeadlineMS < x {
		return familyDeadlineMS
	}
	return x
}
func WebSocketReauthDecision(fields map[string]bool, completedMS, deadlineMS int64, credentialSource string) string {
	for _, k := range []string{"challenge_issued", "nonce_matches", "user_active", "account_ownership_active", "user_matches", "account_matches", "device_matches", "session_matches", "family_matches", "token_valid", "token_fresh"} {
		if !fields[k] {
			return "CLOSE_4401"
		}
	}
	if fields["nonce_consumed"] || fields["revoked"] || fields["malformed"] || credentialSource != "HTTPS_REFRESH" || completedMS >= deadlineMS {
		return "CLOSE_4401"
	}
	return "ATOMIC_REBIND"
}
func WebSocketPendingDecision(revoked bool, frameMS, accessExpiryMS int64) string {
	if revoked {
		return "CLOSE_REVOKED_WITHIN_5_SECONDS"
	}
	if frameMS < accessExpiryMS {
		return "ALLOW_APPLICATION_FRAME"
	}
	return "REJECT_APPLICATION_FRAME"
}

type throttleDimension struct {
	failures                               int
	delayedUntil, lockedUntil, lastFailure int64
}

// Throttle is an in-memory contract model for one serializable password
// decision. Production persistence/transactions are intentionally out of
// scope. Its caller must serialize attempts for a shared scope.
type Throttle struct {
	mu             sync.Mutex
	users, sources map[string]throttleDimension
}

func NewThrottle() *Throttle {
	return &Throttle{users: map[string]throttleDimension{}, sources: map[string]throttleDimension{}}
}
func clear(d throttleDimension, now int64) throttleDimension {
	if d.lockedUntil > 0 && now >= d.lockedUntil {
		return throttleDimension{}
	}
	if d.failures > 0 && now-d.lastFailure >= 900000 {
		return throttleDimension{}
	}
	return d
}
func (t *Throttle) Attempt(nowMS int64, userID, source string, passwordOK bool) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := clear(t.sources[source], nowMS)
	u := throttleDimension{}
	if userID != "" {
		u = clear(t.users[userID], nowMS)
	}
	if s.lockedUntil > nowMS || u.lockedUntil > nowMS {
		return "DENY_LOCK"
	}
	if s.delayedUntil > nowMS || u.delayedUntil > nowMS {
		return "DENY_DELAY"
	}
	if passwordOK {
		if userID != "" {
			t.users[userID] = throttleDimension{}
		}
		t.sources[source] = s
		return "PASSWORD_ACCEPTED"
	}
	s = failed(s, nowMS)
	t.sources[source] = s
	if userID != "" {
		u = failed(u, nowMS)
		t.users[userID] = u
	}
	if s.lockedUntil > nowMS || u.lockedUntil > nowMS {
		return "LOCKED"
	}
	return "FAILED_DELAY"
}
func failed(d throttleDimension, now int64) throttleDimension {
	d.failures++
	d.lastFailure = now
	if d.failures >= 5 {
		d.delayedUntil = 0
		d.lockedUntil = now + 900000
		return d
	}
	delays := []int64{1000, 2000, 4000, 8000}
	d.delayedUntil = now + delays[d.failures-1]
	return d
}
func (t *Throttle) Failures(userID, source string) (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.users[userID].failures, t.sources[source].failures
}

// SourceKey derives the pseudonymous source key over a normalized 16-byte IP.
func SourceKey(key []byte, peer string) (string, error) {
	a, e := netip.ParseAddr(peer)
	if e != nil {
		return "", e
	}
	a = a.Unmap()
	raw := a.As16()
	h := hmac.New(sha256.New, key)
	h.Write(raw[:])
	return "src_" + hex.EncodeToString(h.Sum(nil)), nil
}

// NATSAllowed mechanically checks the generated exact-permission manifest;
// it opens no NATS connection.
func (v *Validator) NATSAllowed(principal, action, subject string) (bool, error) {
	d := v.docs["manifests/nats-permissions-v1.json"]
	if d == nil {
		return false, fmt.Errorf("platformcontract: NATS manifest unavailable")
	}
	for _, p := range slice(d["principals"]) {
		m := schemaMap(p)
		if m["name"] != principal {
			continue
		}
		for _, x := range strSlice(m[action]) {
			if subjectMatch(x, subject) {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("platformcontract: unknown NATS principal")
}

// RateLimitGroup returns a defensive, number-preserving contract mapping for
// an exact frozen rate-limit group. It models no clock or limiter state.
func (v *Validator) RateLimitGroup(name string) (map[string]any, error) {
	d := v.docs["manifests/http-rate-limits-v1.json"]
	group := schemaMap(schemaMap(d["groups"])[name])
	if group == nil {
		return nil, fmt.Errorf("platformcontract: unknown rate-limit group %q", name)
	}
	return cloneObject(group), nil
}
func subjectMatch(pattern, subject string) bool {
	if pattern == subject {
		return true
	}
	if len(pattern) > 2 && pattern[len(pattern)-1] == '>' && pattern[len(pattern)-2] == '.' {
		return len(subject) > len(pattern)-1 && subject[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return false
}
