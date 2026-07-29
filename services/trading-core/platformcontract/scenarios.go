package platformcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"
)

// scenarioThrottleDimension is the contract-level state of one password
// throttle dimension. It intentionally models no persistence or clock; tests
// and adapters supply the serialized transaction time explicitly.
type scenarioThrottleDimension struct {
	failureTimesMS []int64
	nextAllowedMS  int64
	lockedUntilMS  int64
}

func (d scenarioThrottleDimension) clone() scenarioThrottleDimension {
	d.failureTimesMS = append([]int64(nil), d.failureTimesMS...)
	return d
}

func (d *scenarioThrottleDimension) normalize(nowMS, windowMS int64) {
	if d.lockedUntilMS > 0 && nowMS >= d.lockedUntilMS {
		*d = scenarioThrottleDimension{}
		return
	}
	kept := d.failureTimesMS[:0]
	for _, failedAt := range d.failureTimesMS {
		if failedAt > nowMS-windowMS {
			kept = append(kept, failedAt)
		}
	}
	d.failureTimesMS = kept
	if len(d.failureTimesMS) == 0 && d.lockedUntilMS == 0 {
		d.nextAllowedMS = 0
	}
}

type scenarioThrottleConfig struct {
	windowMS    int64
	delaysMS    []int64
	lockOn      int
	lockMS      int64
	knownRoutes map[string]bool
}

type scenarioThrottleEvent struct {
	nowMS              int64
	userID             string
	sourceKey          string
	route              string
	passwordValid      bool
	transactionAborted bool
}

type scenarioThrottleObservation struct {
	decision               string
	appliedDelayMS         int64
	accountFailures        int
	sourceFailures         int
	accountNextAllowedMS   int64
	sourceNextAllowedMS    int64
	accountLockedUntilMS   int64
	sourceLockedUntilMS    int64
	sourceFailureTimesMS   []int64
	accountFailureTimesMS  []int64
	routeCoveredByContract bool
}

type scenarioThrottleModel struct {
	config   scenarioThrottleConfig
	accounts map[string]scenarioThrottleDimension
	sources  map[string]scenarioThrottleDimension
}

func newScenarioThrottleModel(config scenarioThrottleConfig) *scenarioThrottleModel {
	return &scenarioThrottleModel{
		config:   config,
		accounts: make(map[string]scenarioThrottleDimension),
		sources:  make(map[string]scenarioThrottleDimension),
	}
}

// apply stages normalization and the password decision together. An aborted
// transaction returns the pre-transaction observation and commits neither
// pruning nor counters, matching the frozen serializable transaction model.
func (m *scenarioThrottleModel) apply(event scenarioThrottleEvent) (scenarioThrottleObservation, error) {
	if m == nil || m.config.windowMS <= 0 || m.config.lockOn <= 0 ||
		len(m.config.delaysMS) < m.config.lockOn-1 || event.sourceKey == "" {
		return scenarioThrottleObservation{}, fmt.Errorf("platformcontract: invalid throttle scenario")
	}
	if event.route != "" && !m.config.knownRoutes[event.route] {
		return scenarioThrottleObservation{}, fmt.Errorf("platformcontract: throttle route is not frozen")
	}

	storedSource := m.sources[event.sourceKey]
	source := storedSource.clone()
	source.normalize(event.nowMS, m.config.windowMS)

	var storedAccount, account scenarioThrottleDimension
	hasAccount := event.userID != ""
	if hasAccount {
		storedAccount = m.accounts[event.userID]
		account = storedAccount.clone()
		account.normalize(event.nowMS, m.config.windowMS)
	}

	if event.transactionAborted {
		return throttleObservation(
			"TRANSACTION_ABORTED",
			0,
			storedAccount,
			storedSource,
			hasAccount,
			event.route == "" || m.config.knownRoutes[event.route],
		), nil
	}

	dimensions := []*scenarioThrottleDimension{&source}
	if hasAccount {
		dimensions = append(dimensions, &account)
	}
	decision := ""
	appliedDelayMS := int64(0)
	switch {
	case anyThrottleDimension(dimensions, func(d *scenarioThrottleDimension) bool {
		return event.nowMS < d.lockedUntilMS
	}):
		decision = "DENY_LOCK"
	case anyThrottleDimension(dimensions, func(d *scenarioThrottleDimension) bool {
		return event.nowMS < d.nextAllowedMS
	}):
		decision = "DENY_DELAY"
	case event.passwordValid:
		if !hasAccount {
			return scenarioThrottleObservation{}, fmt.Errorf("platformcontract: unresolved password cannot succeed")
		}
		account = scenarioThrottleDimension{}
		decision = "PASSWORD_ACCEPTED"
	default:
		for _, dimension := range dimensions {
			dimension.failureTimesMS = append(dimension.failureTimesMS, event.nowMS)
			failures := len(dimension.failureTimesMS)
			if failures >= m.config.lockOn {
				dimension.lockedUntilMS = event.nowMS + m.config.lockMS
				dimension.nextAllowedMS = 0
				continue
			}
			delay := m.config.delaysMS[failures-1]
			if delay > appliedDelayMS {
				appliedDelayMS = delay
			}
			dimension.nextAllowedMS = event.nowMS + delay
		}
		if anyThrottleDimension(dimensions, func(d *scenarioThrottleDimension) bool {
			return d.lockedUntilMS > event.nowMS
		}) {
			decision = "LOCKED"
		} else {
			decision = "FAILED_DELAY"
		}
	}

	m.sources[event.sourceKey] = source.clone()
	if hasAccount {
		m.accounts[event.userID] = account.clone()
	}
	return throttleObservation(
		decision,
		appliedDelayMS,
		account,
		source,
		hasAccount,
		event.route == "" || m.config.knownRoutes[event.route],
	), nil
}

func anyThrottleDimension(dimensions []*scenarioThrottleDimension, predicate func(*scenarioThrottleDimension) bool) bool {
	for _, dimension := range dimensions {
		if predicate(dimension) {
			return true
		}
	}
	return false
}

func throttleObservation(
	decision string,
	appliedDelayMS int64,
	account, source scenarioThrottleDimension,
	hasAccount, routeCovered bool,
) scenarioThrottleObservation {
	observation := scenarioThrottleObservation{
		decision:               decision,
		appliedDelayMS:         appliedDelayMS,
		sourceFailures:         len(source.failureTimesMS),
		sourceNextAllowedMS:    source.nextAllowedMS,
		sourceLockedUntilMS:    source.lockedUntilMS,
		sourceFailureTimesMS:   append([]int64(nil), source.failureTimesMS...),
		routeCoveredByContract: routeCovered,
	}
	if hasAccount {
		observation.accountFailures = len(account.failureTimesMS)
		observation.accountNextAllowedMS = account.nextAllowedMS
		observation.accountLockedUntilMS = account.lockedUntilMS
		observation.accountFailureTimesMS = append([]int64(nil), account.failureTimesMS...)
	}
	return observation
}

func scenarioChallengeOutcome(synthetic, alreadyAttempted, proofValid bool, nowMS, expiresMS int64) string {
	switch {
	case synthetic:
		return "REJECT_SYNTHETIC_NO_STATE"
	case alreadyAttempted:
		return "REJECT_REPLAY"
	case nowMS >= expiresMS:
		return "REJECT_EXPIRED"
	case proofValid:
		return "CONSUMED_SUCCESS"
	default:
		return "CONSUMED_INVALID_PROOF"
	}
}

func scenarioRefreshOutcome(status string, nowMS, individualExpiryMS, familyDeadlineMS int64) string {
	switch {
	case nowMS >= familyDeadlineMS:
		return "REJECT_FAMILY_EXPIRED"
	case status == "ROTATED":
		return "REVOKE_FAMILY_AND_DESCENDANTS"
	case status == "REVOKED":
		return "REJECT_REVOKED"
	case nowMS >= individualExpiryMS:
		return "REJECT_EXPIRED_NO_ROTATION"
	default:
		return "ROTATE_CREATE_DESCENDANT"
	}
}

type scenarioWebSocketReauth struct {
	event                     string
	challengeIssued           bool
	nonceMatches              bool
	nonceConsumed             bool
	userActive                bool
	accountOwnershipActive    bool
	userMatches               bool
	accountMatches            bool
	deviceMatches             bool
	sessionMatches            bool
	familyMatches             bool
	tokenValid                bool
	tokenFresh                bool
	malformed                 bool
	revoked                   bool
	credentialSource          string
	verificationCompletedAtMS int64
	deadlineMS                int64
	frameReceivedAtMS         int64
	currentAccessExpiresAtMS  int64
}

func scenarioWebSocketOutcome(item scenarioWebSocketReauth) string {
	if item.revoked {
		return "CLOSE_REVOKED_WITHIN_5_SECONDS"
	}
	if item.event == "APPLICATION_FRAME_WHILE_REAUTH_PENDING" {
		if item.frameReceivedAtMS >= item.currentAccessExpiresAtMS {
			return "REJECT_APPLICATION_FRAME"
		}
		return "ALLOW_APPLICATION_FRAME"
	}
	if item.event == "REAUTH_DEADLINE_WITHOUT_RESPONSE" {
		return "CLOSE_4401"
	}
	if !item.challengeIssued || item.malformed || !item.nonceMatches ||
		item.nonceConsumed || !item.userActive || !item.accountOwnershipActive ||
		!item.userMatches || !item.accountMatches || !item.deviceMatches ||
		!item.sessionMatches || !item.familyMatches || !item.tokenValid ||
		!item.tokenFresh || item.credentialSource != "HTTPS_REFRESH" ||
		item.verificationCompletedAtMS >= item.deadlineMS {
		return "CLOSE_4401"
	}
	return "ATOMIC_REBIND"
}

func scenarioIdempotencyOutcome(sameScope bool, existingDigest, incomingDigest *string) string {
	if !sameScope {
		return "INDEPENDENT_SCOPE_NO_CROSS_OWNER_ACCESS"
	}
	if existingDigest == nil {
		return "CLAIM_AND_EXECUTE"
	}
	if incomingDigest != nil && *existingDigest == *incomingDigest {
		return "RETURN_RECORDED_RESULT"
	}
	return "IDEMPOTENCY_CONFLICT"
}

type scenarioEnrollmentChallenge struct {
	wire                    map[string]any
	userID                  string
	tradingAccountID        string
	candidateKeyFingerprint string
	purpose                 string
	attempted               bool
}

type scenarioEnrollmentEntity struct {
	userID string
	active bool
}

type scenarioEnrollmentRuntime struct {
	challenges      map[string]*scenarioEnrollmentChallenge
	devices         map[string]scenarioEnrollmentEntity
	sessions        map[string]scenarioEnrollmentEntity
	refreshFamilies map[string]scenarioEnrollmentEntity
	enrollments     map[string]scenarioEnrollmentEntity
	publicKeyOwners map[string]string
	auditEvents     []string
	notifications   []string
	outboxRecords   []string
	nextIdentity    uint64
}

type scenarioEnrollmentCounts struct {
	challenges      int
	devices         int
	sessions        int
	refreshFamilies int
	enrollments     int
	auditEvents     int
	notifications   int
	outboxRecords   int
}

func newScenarioEnrollmentRuntime() *scenarioEnrollmentRuntime {
	return &scenarioEnrollmentRuntime{
		challenges:      make(map[string]*scenarioEnrollmentChallenge),
		devices:         make(map[string]scenarioEnrollmentEntity),
		sessions:        make(map[string]scenarioEnrollmentEntity),
		refreshFamilies: make(map[string]scenarioEnrollmentEntity),
		enrollments:     make(map[string]scenarioEnrollmentEntity),
		publicKeyOwners: make(map[string]string),
	}
}

func (runtime *scenarioEnrollmentRuntime) start(
	wire map[string]any,
	userID, tradingAccountID string,
	knownIdentity, passwordValid, activeOwnership bool,
) bool {
	if runtime == nil || !knownIdentity || !passwordValid || !activeOwnership {
		return false
	}
	handle, handleOK := wire["subject_handle"].(string)
	fingerprint, fingerprintOK := wire["candidate_public_key_fingerprint"].(string)
	purpose, purposeOK := wire["purpose"].(string)
	if !handleOK || !fingerprintOK || !purposeOK || handle == "" || runtime.challenges[handle] != nil {
		return false
	}
	runtime.challenges[handle] = &scenarioEnrollmentChallenge{
		wire:                    cloneScenarioObject(wire),
		userID:                  userID,
		tradingAccountID:        tradingAccountID,
		candidateKeyFingerprint: fingerprint,
		purpose:                 purpose,
	}
	runtime.appendAuditAndOutbox("ENROLLMENT_CHALLENGE_ISSUED")
	return true
}

func (runtime *scenarioEnrollmentRuntime) complete(completion map[string]any, nowMS int64) string {
	if runtime == nil {
		return "REJECT_NO_SERVER_STATE"
	}
	handle, _ := completion["subject_handle"].(string)
	challenge := runtime.challenges[handle]
	if challenge == nil {
		return "REJECT_NO_SERVER_STATE"
	}
	if challenge.attempted {
		return "REJECT_REPLAY"
	}
	challenge.attempted = true
	expiresAt, err := scenarioMilliseconds(stringValue(challenge.wire["expires_at"]))
	if err != nil || nowMS >= expiresAt {
		return "REJECT_EXPIRED"
	}
	publicKey, _ := completion["candidate_public_key"].(string)
	signature, _ := completion["signature"].(string)
	fingerprint, err := scenarioEd25519Fingerprint(publicKey)
	proofValid := err == nil && fingerprint == challenge.candidateKeyFingerprint
	if proofValid {
		proofValid, err = VerifyChallengeProof(challenge.wire, publicKey, signature)
		proofValid = err == nil && proofValid
	}
	if !proofValid {
		runtime.appendAuditAndOutbox("ENROLLMENT_PROOF_REJECTED")
		return "CONSUMED_INVALID_PROOF"
	}
	if owner, exists := runtime.publicKeyOwners[fingerprint]; exists {
		runtime.appendAuditAndOutbox("ENROLLMENT_PROOF_REJECTED")
		if owner == challenge.userID {
			return "REJECT_DUPLICATE_KEY"
		}
		return "REJECT_CROSS_OWNER_REBIND"
	}
	runtime.commitSuccessfulEnrollment(challenge)
	return "CONSUMED_SUCCESS"
}

func (runtime *scenarioEnrollmentRuntime) commitSuccessfulEnrollment(challenge *scenarioEnrollmentChallenge) {
	if challenge.purpose == "REPLACEMENT_DEVICE" {
		deactivateScenarioEnrollmentEntities(runtime.devices, challenge.userID)
		deactivateScenarioEnrollmentEntities(runtime.sessions, challenge.userID)
		deactivateScenarioEnrollmentEntities(runtime.refreshFamilies, challenge.userID)
	}
	runtime.nextIdentity++
	suffix := fmt.Sprintf("%s:%d", challenge.userID, runtime.nextIdentity)
	runtime.devices["device:"+suffix] = scenarioEnrollmentEntity{userID: challenge.userID, active: true}
	runtime.sessions["session:"+suffix] = scenarioEnrollmentEntity{userID: challenge.userID, active: true}
	runtime.refreshFamilies["family:"+suffix] = scenarioEnrollmentEntity{userID: challenge.userID, active: true}
	runtime.enrollments["enrollment:"+suffix] = scenarioEnrollmentEntity{userID: challenge.userID, active: true}
	runtime.publicKeyOwners[challenge.candidateKeyFingerprint] = challenge.userID
	kind := "DEVICE_ENROLLED"
	if challenge.purpose == "REPLACEMENT_DEVICE" {
		kind = "DEVICE_REPLACED"
	}
	runtime.auditEvents = append(runtime.auditEvents, kind)
	runtime.notifications = append(runtime.notifications, kind)
	runtime.outboxRecords = append(runtime.outboxRecords, "audit:"+kind, "notification:"+kind)
}

func (runtime *scenarioEnrollmentRuntime) appendAuditAndOutbox(kind string) {
	runtime.auditEvents = append(runtime.auditEvents, kind)
	runtime.outboxRecords = append(runtime.outboxRecords, "audit:"+kind)
}

func (runtime *scenarioEnrollmentRuntime) seedPrior(userID string) {
	runtime.devices["prior-device"] = scenarioEnrollmentEntity{userID: userID, active: true}
	runtime.sessions["prior-session"] = scenarioEnrollmentEntity{userID: userID, active: true}
	runtime.refreshFamilies["prior-family"] = scenarioEnrollmentEntity{userID: userID, active: true}
}

func (runtime *scenarioEnrollmentRuntime) priorActive() (bool, bool, bool) {
	return runtime.devices["prior-device"].active,
		runtime.sessions["prior-session"].active,
		runtime.refreshFamilies["prior-family"].active
}

func (runtime *scenarioEnrollmentRuntime) counts() scenarioEnrollmentCounts {
	return scenarioEnrollmentCounts{
		challenges:      len(runtime.challenges),
		devices:         len(runtime.devices),
		sessions:        len(runtime.sessions),
		refreshFamilies: len(runtime.refreshFamilies),
		enrollments:     len(runtime.enrollments),
		auditEvents:     len(runtime.auditEvents),
		notifications:   len(runtime.notifications),
		outboxRecords:   len(runtime.outboxRecords),
	}
}

func deactivateScenarioEnrollmentEntities(entities map[string]scenarioEnrollmentEntity, userID string) {
	for id, entity := range entities {
		if entity.userID == userID {
			entity.active = false
			entities[id] = entity
		}
	}
}

func scenarioEd25519Fingerprint(publicKeyWire string) (string, error) {
	const prefix = "ed25519-public:"
	if !strings.HasPrefix(publicKeyWire, prefix) {
		return "", fmt.Errorf("platformcontract: invalid public key wire")
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(publicKeyWire, prefix))
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("platformcontract: invalid public key wire")
	}
	return "ed25519:" + fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func cloneScenarioObject(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func scenarioRedactedObservation(value any, fields map[string]bool) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = scenarioRedactedObservation(item, fields)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if fields[authorityAlias(key)] {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = scenarioRedactedObservation(item, fields)
		}
		return result
	default:
		return value
	}
}

func scenarioRevocationOutcome(ingress string, elapsedMS, maximumMS int64) (string, error) {
	if elapsedMS < 0 || elapsedMS > maximumMS {
		return "", fmt.Errorf("platformcontract: revocation propagation deadline violated")
	}
	switch ingress {
	case "WEBSOCKET_EXISTING":
		return "DENY_AND_CLOSE", nil
	case "HTTP", "WEBSOCKET_NEW", "REFRESH", "INTERNAL_TOOL":
		return "DENY", nil
	default:
		return "", fmt.Errorf("platformcontract: unknown revocation ingress %q", ingress)
	}
}

func scenarioCanonicalIP(peer string) ([16]byte, error) {
	address, err := netip.ParseAddr(peer)
	if err != nil {
		return [16]byte{}, err
	}
	return address.Unmap().As16(), nil
}

func scenarioTokenBucketDecisions(rate, perSeconds float64, burst int, timestampsMS []int64) []bool {
	if len(timestampsMS) == 0 {
		return nil
	}
	tokens := float64(burst)
	lastMS := timestampsMS[0]
	decisions := make([]bool, len(timestampsMS))
	for index, nowMS := range timestampsMS {
		elapsedMS := nowMS - lastMS
		if elapsedMS < 0 {
			elapsedMS = 0
		}
		tokens += (float64(elapsedMS) / 1000) * (rate / perSeconds)
		if tokens > float64(burst) {
			tokens = float64(burst)
		}
		lastMS = nowMS
		if tokens+(math.Nextafter(1, 2)-1) < 1 {
			continue
		}
		tokens--
		decisions[index] = true
	}
	return decisions
}

func scenarioWALOutcome(commandFailed bool, archiveAgeSeconds, consecutiveHealthyCycles int64, incidentOpen bool) string {
	unhealthy := commandFailed || archiveAgeSeconds > 60
	if unhealthy {
		if incidentOpen {
			return "KEEP_SAME_INCIDENT_NO_DUPLICATE"
		}
		return "OPEN_ONE_INCIDENT"
	}
	if incidentOpen && consecutiveHealthyCycles+1 >= 2 {
		return "CLEAR_AFTER_SECOND_SUCCESS"
	}
	if incidentOpen {
		return "KEEP_INCIDENT_OPEN"
	}
	return "HEALTHY"
}

func scenarioMilliseconds(value string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, err
	}
	return parsed.UnixMilli(), nil
}
