package platformcontract

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type completeFixtureContext struct {
	validator  *Validator
	semantic   map[string]any
	executable map[string]any
	linkage    map[string]any
	golden     map[string]any
	security   map[string]any
	rateLimits map[string]any
	nats       map[string]any
}

type completeGroupHandler func(*testing.T, *completeFixtureContext, []any)

type completeGroupSpec struct {
	count   int
	handler completeGroupHandler
}

func newCompleteFixtureContext(t *testing.T) *completeFixtureContext {
	t.Helper()
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	return &completeFixtureContext{
		validator:  validator,
		semantic:   load(t, "fixtures/platform/semantic-scenarios-v1.json"),
		executable: load(t, "fixtures/platform/executable-security-scenarios-v1.json"),
		linkage:    load(t, "fixtures/platform/linkage-chains-v1.json"),
		golden:     load(t, "fixtures/platform/golden-vectors-v1.json"),
		security:   load(t, "platform/manifests/security-values-v1.json"),
		rateLimits: load(t, "platform/manifests/http-rate-limits-v1.json"),
		nats:       load(t, "platform/manifests/nats-permissions-v1.json"),
	}
}

func completeSemanticRegistry() map[string]completeGroupSpec {
	return map[string]completeGroupSpec{
		"challenge_cases":                 {5, completeChallengeCases},
		"refresh_cases":                   {6, completeRefreshCases},
		"refresh_expiry_cases":            {2, completeRefreshExpiryCases},
		"throttle_cases":                  {5, completeThrottleCases},
		"password_failure_response_cases": {6, completePasswordFailureCases},
		"throttle_reset_cases":            {3, completeThrottleResetCases},
		"websocket_cases":                 {13, completeWebSocketCases},
		"websocket_deadline_cases":        {2, completeWebSocketDeadlineCases},
		"idempotency_cases":               {4, completeIdempotencyCases},
		"request_decode_cases":            {5, completeRequestDecodeCases},
		"source_key_vectors":              {3, completeSourceKeyCases},
		"source_key_rotation_cases":       {3, completeSourceKeyRotationCases},
		"rate_limit_cases":                {10, completeRateLimitCases},
		"enrollment_effect_cases":         {2, completeEnrollmentEffectCases},
		"device_key_cases":                {4, completeDeviceKeyCases},
		"session_fixation_cases":          {3, completeSessionFixationCases},
		"revocation_deadline_cases":       {5, completeRevocationDeadlineCases},
		"wal_cases":                       {7, completeWALCases},
	}
}

func completeSemanticSingletonRegistry() map[string]func(*testing.T, *completeFixtureContext, map[string]any) {
	return map[string]func(*testing.T, *completeFixtureContext, map[string]any){
		"cross_route_throttle_case": completeCrossRouteThrottleCase,
		"forwarding_header_case":    completeForwardingHeaderCase,
		"recovery_calculation_case": completeRecoveryCalculationCase,
	}
}

func completeExecutableRegistry() map[string]completeGroupSpec {
	return map[string]completeGroupSpec{
		"identity_directory":          {1, completeIdentityDirectory},
		"ownership_records":           {1, completeOwnershipRecords},
		"enrollment_transition_cases": {5, completeEnrollmentTransitions},
		"password_throttle_timelines": {7, completePasswordThrottleTimelines},
		"websocket_upgrade_cases":     {13, completeWebSocketUpgradeCases},
		"websocket_redaction_cases":   {3, completeWebSocketRedactionCases},
		"raw_request_target_cases":    {12, completeRawRequestTargetCases},
		"raw_entrypoint_cases":        {7, completeRawEntrypointCases},
	}
}

func TestCompleteFrozenSemanticScenarios(t *testing.T) {
	context := newCompleteFixtureContext(t)
	if got, want := completeString(t, context.semantic["schema_version"]), "fit.platform.semantic-scenarios.v1"; got != want {
		t.Fatalf("semantic schema version = %q, want %q", got, want)
	}

	groups := completeSemanticRegistry()
	singletons := completeSemanticSingletonRegistry()
	count := 0
	for name, raw := range context.semantic {
		if name == "schema_version" {
			continue
		}
		if values, ok := raw.([]any); ok {
			spec, exists := groups[name]
			if !exists {
				t.Fatalf("semantic fixture group %q appeared without a handler", name)
			}
			if len(values) != spec.count {
				t.Fatalf("semantic fixture group %q has %d cases, want exactly %d", name, len(values), spec.count)
			}
			count += len(values)
			t.Run(name, func(t *testing.T) { spec.handler(t, context, values) })
			continue
		}
		value, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("semantic fixture %q has unsupported top-level type %T", name, raw)
		}
		handler, exists := singletons[name]
		if !exists {
			t.Fatalf("semantic singleton %q appeared without a handler", name)
		}
		t.Run(name, func(t *testing.T) { handler(t, context, value) })
	}
	for name := range groups {
		if _, exists := context.semantic[name]; !exists {
			t.Errorf("semantic fixture group %q is missing", name)
		}
	}
	for name := range singletons {
		if _, exists := context.semantic[name]; !exists {
			t.Errorf("semantic singleton %q is missing", name)
		}
	}
	if count != 88 {
		t.Fatalf("executed semantic array cases = %d, want exactly 88", count)
	}
	if len(groups) != 18 || len(singletons) != 3 {
		t.Fatalf("semantic handler inventory drift: %d array groups, %d singletons", len(groups), len(singletons))
	}
	t.Logf("executed semantic cases=%d singleton_cases=%d groups=%d", count, len(singletons), len(groups))
}

func TestCompleteFrozenExecutableSecurityScenarios(t *testing.T) {
	context := newCompleteFixtureContext(t)
	if got, want := completeString(t, context.executable["schema_version"]), "fit.platform.executable-security-scenarios.v1"; got != want {
		t.Fatalf("executable schema version = %q, want %q", got, want)
	}
	groups := completeExecutableRegistry()
	count := 0
	for name, raw := range context.executable {
		if name == "schema_version" {
			continue
		}
		values, ok := raw.([]any)
		if !ok {
			t.Fatalf("executable fixture group %q has unsupported top-level type %T", name, raw)
		}
		spec, exists := groups[name]
		if !exists {
			t.Fatalf("executable fixture group %q appeared without a handler", name)
		}
		if len(values) != spec.count {
			t.Fatalf("executable fixture group %q has %d cases, want exactly %d", name, len(values), spec.count)
		}
		count += len(values)
		t.Run(name, func(t *testing.T) { spec.handler(t, context, values) })
	}
	for name := range groups {
		if _, exists := context.executable[name]; !exists {
			t.Errorf("executable fixture group %q is missing", name)
		}
	}
	if count != 49 || len(groups) != 8 {
		t.Fatalf("executable handler inventory drift: cases=%d groups=%d", count, len(groups))
	}
	t.Logf("executed executable/security cases=%d groups=%d throttle_events=35", count, len(groups))
}

func completeChallengeCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"valid proof strictly before expiry consumes once",
		"deadline equality is expired",
		"replay after first attempt fails",
		"invalid proof consumes the one attempt",
		"synthetic challenge has no completion path",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		now := completeTime(t, item["now"])
		expires := completeTime(t, item["expires_at"])
		got := scenarioChallengeOutcome(
			completeBool(t, item["synthetic"]),
			completeBool(t, item["already_attempted"]),
			completeBool(t, item["proof_valid"]),
			now,
			expires,
		)
		completeDecision(t, item, got)
	}
}

func completeRefreshCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"active token rotates before both deadlines",
		"never-rotated individual expiry cannot rotate",
		"rotated ancestor reuse before individual expiry revokes family",
		"rotated ancestor reuse after individual expiry still revokes family",
		"family deadline wins over reuse",
		"revoked descendant always rejects",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		got := scenarioRefreshOutcome(
			completeString(t, item["status"]),
			completeTime(t, item["now"]),
			completeTime(t, item["individual_expires_at"]),
			completeTime(t, item["family_deadline"]),
		)
		completeDecision(t, item, got)
	}
}

func completeRefreshExpiryCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{"normal seven-day expiry", "family deadline truncates descendant"})
	for _, raw := range cases {
		item := completeObject(t, raw)
		issued := completeTime(t, item["issued_at"])
		deadline := completeTime(t, item["family_deadline"])
		got := issued + 604_800_000
		if deadline < got {
			got = deadline
		}
		if want := completeTime(t, item["expected_expires_at"]); got != want {
			t.Errorf("%s: expiry=%d, want %d", item["name"], got, want)
		}
	}
}

func completeThrottleCases(t *testing.T, context *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"failures one through four use frozen delays",
		"second failure",
		"third failure",
		"fourth failure",
		"fifth failure locks",
	})
	throttle := completeObject(t, context.security["password_throttle"])
	delays := completeArray(t, throttle["failure_delays_seconds"])
	lockOn := completeInt(t, throttle["lock_on_failure"])
	lockSeconds := completeInt(t, throttle["lock_seconds"])
	for _, raw := range cases {
		item := completeObject(t, raw)
		failure := completeInt(t, item["failure_number"])
		var delay, lock int64
		if failure < lockOn {
			delay = completeInt(t, delays[failure-1])
		}
		if failure == lockOn {
			lock = lockSeconds
		}
		if delay != completeInt(t, item["expected_delay_seconds"]) ||
			lock != completeInt(t, item["expected_lock_seconds"]) {
			t.Errorf("%s: delay/lock=%d/%d", item["name"], delay, lock)
		}
	}
}

func completePasswordFailureCases(t *testing.T, context *completeFixtureContext, cases []any) {
	throttle := completeObject(t, context.security["password_throttle"])
	routes := completeStringArray(t, throttle["routes"])
	if len(routes) != 3 {
		t.Fatalf("password throttle routes=%d, want 3", len(routes))
	}
	for _, route := range routes {
		var matched []map[string]any
		for _, raw := range cases {
			item := completeObject(t, raw)
			if completeString(t, item["route"]) == route {
				matched = append(matched, item)
			}
		}
		if len(matched) != 2 {
			t.Fatalf("%s has %d password-failure cases, want 2", route, len(matched))
		}
		failures := []string{completeString(t, matched[0]["failure"]), completeString(t, matched[1]["failure"])}
		sort.Strings(failures)
		if !reflect.DeepEqual(failures, []string{"UNKNOWN_IDENTIFIER", "WRONG_PASSWORD"}) {
			t.Errorf("%s failure classes=%v", route, failures)
		}
		for _, key := range []string{"status", "envelope", "timing_class", "synthetic_challenge"} {
			if CanonicalJSON(strictValue(t, matched[0][key])) != CanonicalJSON(strictValue(t, matched[1][key])) {
				t.Errorf("%s leaks identifier resolution through %s", route, key)
			}
		}
		if completeString(t, matched[0]["timing_class"]) != "PASSWORD_VERIFICATION_AND_THROTTLE_DELAY" {
			t.Errorf("%s timing class drift", route)
		}
		login := route == "POST /v1/auth/login"
		if login {
			if completeInt(t, matched[0]["status"]) != 401 ||
				completeString(t, matched[0]["envelope"]) != "AUTHENTICATION_FAILED" ||
				completeBool(t, matched[0]["synthetic_challenge"]) {
				t.Errorf("%s response contract drift", route)
			}
		} else if completeInt(t, matched[0]["status"]) != 200 ||
			completeString(t, matched[0]["envelope"]) != "ENROLLMENT_CHALLENGE_ISSUED" ||
			!completeBool(t, matched[0]["synthetic_challenge"]) {
			t.Errorf("%s response contract drift", route)
		}
	}
}

func completeThrottleResetCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"success clears account only",
		"unknown identifier increments source only",
		"expired account lock clears dimension",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		account := completeInt(t, item["account_failures_before"])
		source := completeInt(t, item["source_failures_before"])
		switch completeString(t, item["event"]) {
		case "PASSWORD_SUCCESS", "ACCOUNT_LOCK_EXPIRED":
			account = 0
		case "PASSWORD_FAILURE_UNKNOWN_IDENTIFIER":
			source++
		default:
			t.Fatalf("unknown throttle reset event %q", item["event"])
		}
		if account != completeInt(t, item["expected_account_failures"]) ||
			source != completeInt(t, item["expected_source_failures"]) {
			t.Errorf("%s reset result=%d/%d", item["name"], account, source)
		}
	}
}

func completeWebSocketCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"same binding finishes before deadline",
		"deadline equality closes",
		"unsolicited reauth closes",
		"replayed nonce closes",
		"malformed control frame closes",
		"expired token closes",
		"nonfresh token closes",
		"in-band credential closes",
		"each binding mismatch closes",
		"missing response at deadline closes",
		"application frame before token expiry remains allowed",
		"application frame after token expiry is rejected",
		"revocation wins over valid reauth",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		model := scenarioWebSocketReauth{
			event:                     completeOptionalString(item["event"]),
			challengeIssued:           completeOptionalBool(item["challenge_issued"], false),
			nonceMatches:              completeOptionalBool(item["nonce_matches"], false),
			nonceConsumed:             completeOptionalBool(item["nonce_consumed"], false),
			userActive:                completeOptionalBool(item["user_active"], true),
			accountOwnershipActive:    completeOptionalBool(item["account_ownership_active"], true),
			userMatches:               completeOptionalBool(item["user_matches"], false),
			accountMatches:            completeOptionalBool(item["account_matches"], false),
			deviceMatches:             completeOptionalBool(item["device_matches"], false),
			sessionMatches:            completeOptionalBool(item["session_matches"], false),
			familyMatches:             completeOptionalBool(item["family_matches"], false),
			tokenValid:                completeOptionalBool(item["token_valid"], false),
			tokenFresh:                completeOptionalBool(item["token_fresh"], false),
			malformed:                 completeOptionalBool(item["malformed"], false),
			revoked:                   completeOptionalBool(item["revoked"], false),
			credentialSource:          completeOptionalString(item["credential_source"]),
			verificationCompletedAtMS: completeOptionalTime(t, item["verification_completed_at"]),
			deadlineMS:                completeOptionalTime(t, item["deadline"]),
			frameReceivedAtMS:         completeOptionalTime(t, item["frame_received_at"]),
			currentAccessExpiresAtMS:  completeOptionalTime(t, item["current_access_expires_at"]),
		}
		completeDecision(t, item, scenarioWebSocketOutcome(model))
	}
}

func completeWebSocketDeadlineCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"normal challenge gets sixty seconds",
		"near-expiry challenge is truncated by access expiry",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		connection := completeTime(t, item["connection_established_at"])
		expires := completeTime(t, item["current_access_expires_at"])
		issued := expires - 60_000
		if connection > issued {
			issued = connection
		}
		if issued != completeTime(t, item["issued_at"]) {
			t.Errorf("%s issue time mismatch", item["name"])
		}
		deadline := issued + 60_000
		if expires < deadline {
			deadline = expires
		}
		if deadline != completeTime(t, item["expected_deadline"]) {
			t.Errorf("%s deadline mismatch", item["name"])
		}
	}
}

func completeIdempotencyCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"first use claims and executes",
		"same key and digest returns record",
		"same scoped key with changed body conflicts",
		"cross-owner key does not access prior record",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		existing := completeOptionalStringPointer(item["existing_digest"])
		incoming := completeOptionalStringPointer(item["incoming_digest"])
		got := scenarioIdempotencyOutcome(completeBool(t, item["same_scope"]), existing, incoming)
		completeDecision(t, item, got)
	}
}

func completeRequestDecodeCases(t *testing.T, context *completeFixtureContext, cases []any) {
	names := []string{
		"duplicate query key",
		"non JSON mutation",
		"unknown body field",
		"ambiguous normalized path",
		"strictly decoded mutation",
	}
	completeRequireNames(t, cases, names)
	for _, raw := range cases {
		item := completeObject(t, raw)
		name := completeString(t, item["name"])
		accepted := false
		switch name {
		case "duplicate query key":
			_, _, err := ParseConfirmationTarget("/v1/confirmations/70000000-0000-4000-8000-000000000001/consume?mode=a&mode=b")
			accepted = err == nil
		case "non JSON mutation":
			_, err := ParseStrictJSON([]byte("not-json"))
			accepted = err == nil
		case "unknown body field":
			_, err := context.validator.DecodeTyped("ConfirmationConsumeMutationInput", []byte(
				`{"idempotency_key":"60000000-0000-4000-8000-000000000001","path":{"confirmation_id":"70000000-0000-4000-8000-000000000001"},"query":{},"body":{"confirmation_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","undeclared_switch":true}}`,
			))
			accepted = err == nil
		case "ambiguous normalized path":
			_, _, err := ParseConfirmationTarget("/v1/confirmations/../70000000-0000-4000-8000-000000000001/consume")
			accepted = err == nil
		case "strictly decoded mutation":
			_, _, targetErr := ParseConfirmationTarget("/v1/confirmations/70000000-0000-4000-8000-000000000001/consume")
			_, decodeErr := context.validator.DecodeTyped("ConfirmationConsumeMutationInput", []byte(
				`{"idempotency_key":"60000000-0000-4000-8000-000000000001","path":{"confirmation_id":"70000000-0000-4000-8000-000000000001"},"query":{},"body":{"confirmation_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`,
			))
			accepted = targetErr == nil && decodeErr == nil
		default:
			t.Fatalf("request decode case %q lacks an implementation", name)
		}
		got := "REJECT"
		if accepted {
			got = "ACCEPT"
		}
		completeDecision(t, item, got)
	}
}

func completeSourceKeyCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"IPv4 uses mapped 16-byte IPv6 form",
		"IPv4-mapped IPv6 text normalizes to the same 16 bytes",
		"compressed IPv6 expands to 16 bytes",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		canonical, err := scenarioCanonicalIP(completeString(t, item["direct_peer_ip"]))
		if err != nil {
			t.Errorf("%s: %v", item["name"], err)
			continue
		}
		if got := hex.EncodeToString(canonical[:]); got != completeString(t, item["canonical_ip_hex"]) {
			t.Errorf("%s canonical IP=%s", item["name"], got)
		}
		got, err := SourceKey([]byte(completeString(t, item["test_key_utf8"])), completeString(t, item["direct_peer_ip"]))
		want := "src_" + completeString(t, item["expected_hmac_sha256"])
		if err != nil || got != want {
			t.Errorf("%s source key=%s want=%s err=%v", item["name"], got, want, err)
		}
	}
}

func completeSourceKeyRotationCases(t *testing.T, context *completeFixtureContext, cases []any) {
	overlap := completeInt(t, completeObject(t, context.security["source_key"])["prior_key_overlap_seconds"])
	wantElapsed := []int64{1799, 1800, 1801}
	for index, raw := range cases {
		item := completeObject(t, raw)
		elapsed := completeInt(t, item["elapsed_seconds"])
		if elapsed != wantElapsed[index] {
			t.Fatalf("source rotation case %d elapsed=%d, want %d", index, elapsed, wantElapsed[index])
		}
		if got := elapsed < overlap; got != completeBool(t, item["previous_key_accepted_for_correlation"]) {
			t.Errorf("source rotation elapsed=%d accepted=%v", elapsed, got)
		}
	}
}

func completeRateLimitCases(t *testing.T, context *completeFixtureContext, cases []any) {
	completeRateLimitManifestMappings(t, context)
	expectedNames := []string{
		"password-starts-source",
		"enrollment-completions-source",
		"refresh-source",
		"refresh-family",
		"health-live-source",
		"health-ready-source",
		"authenticated-session",
		"authenticated-source",
		"websocket-session",
		"websocket-source",
	}
	actualNames := make([]string, 0, len(cases))
	for _, raw := range cases {
		item := completeObject(t, raw)
		name := completeString(t, item["group"])
		actualNames = append(actualNames, name)
		group, err := context.validator.RateLimitGroup(name)
		if err != nil {
			t.Error(err)
			continue
		}
		for _, key := range []string{"rate", "per_seconds", "burst"} {
			if CanonicalJSON(group[key]) != CanonicalJSON(strictValue(t, item[key])) {
				t.Errorf("%s %s mapping drift", name, key)
			}
		}
		if want, exists := item["maximum_concurrent"]; exists &&
			CanonicalJSON(group["maximum_concurrent"]) != CanonicalJSON(strictValue(t, want)) {
			t.Errorf("%s maximum_concurrent mapping drift", name)
		}
	}
	if !reflect.DeepEqual(actualNames, expectedNames) {
		t.Fatalf("rate-limit fixture order/names=%v, want %v", actualNames, expectedNames)
	}

	groups := completeObject(t, context.rateLimits["groups"])
	if len(groups) != 11 {
		t.Fatalf("rate-limit manifest groups=%d, want 11", len(groups))
	}
	for name, raw := range groups {
		group := completeObject(t, raw)
		if maximum, exists := group["maximum_attempts"]; exists {
			if completeInt(t, maximum) != 1 {
				t.Errorf("%s maximum_attempts != 1", name)
			}
			continue
		}
		rate := completeFloat(t, group["rate"])
		perSeconds := completeFloat(t, group["per_seconds"])
		burst := int(completeInt(t, group["burst"]))
		atBurst := make([]int64, burst+1)
		decisions := scenarioTokenBucketDecisions(rate, perSeconds, burst, atBurst)
		for index := 0; index < burst; index++ {
			if !decisions[index] {
				t.Errorf("%s denied request %d inside burst", name, index)
			}
		}
		if decisions[burst] {
			t.Errorf("%s allowed above burst", name)
		}
		refillMS := int64((perSeconds / rate) * 1000)
		refillTimes := append(make([]int64, burst), refillMS)
		if got := scenarioTokenBucketDecisions(rate, perSeconds, burst, refillTimes); !got[len(got)-1] {
			t.Errorf("%s failed to refill one token", name)
		}
		if maximum, exists := group["maximum_concurrent"]; exists {
			limit := int(completeInt(t, maximum))
			concurrent := 0
			acquire := func() bool {
				if concurrent >= limit {
					return false
				}
				concurrent++
				return true
			}
			for index := 0; index < limit; index++ {
				if !acquire() {
					t.Errorf("%s denied within concurrency maximum", name)
				}
			}
			if acquire() {
				t.Errorf("%s exceeded concurrency maximum", name)
			}
			concurrent--
			if !acquire() {
				t.Errorf("%s did not admit after release", name)
			}
		}
	}
	session := completeObject(t, groups["authenticated-session"])
	source := completeObject(t, groups["authenticated-source"])
	sessionBurst := int(completeInt(t, session["burst"]))
	sessionTimes := make([]int64, sessionBurst+1)
	sessionAllowed := scenarioTokenBucketDecisions(
		completeFloat(t, session["rate"]),
		completeFloat(t, session["per_seconds"]),
		sessionBurst,
		sessionTimes,
	)[sessionBurst]
	sourceAllowed := scenarioTokenBucketDecisions(
		completeFloat(t, source["rate"]),
		completeFloat(t, source["per_seconds"]),
		int(completeInt(t, source["burst"])),
		[]int64{0},
	)[0]
	if sessionAllowed && sourceAllowed {
		t.Error("combined authenticated dimensions allowed when session dimension denied")
	}
}

func completeRateLimitManifestMappings(t *testing.T, context *completeFixtureContext) {
	t.Helper()
	if got := completeString(t, context.rateLimits["default_route_policy"]); got != "AUTHENTICATE_BEFORE_HANDLER_DISPATCH" {
		t.Errorf("default rate-limit route policy=%q", got)
	}
	expectedRoutes := map[string][]string{
		"POST /v1/auth/login":                        {"password-starts-source"},
		"POST /v1/auth/device-enrollments/start":     {"password-starts-source"},
		"POST /v1/auth/device-replacements/start":    {"password-starts-source"},
		"POST /v1/auth/device-enrollments/complete":  {"enrollment-completions-source", "one-attempt-per-challenge"},
		"POST /v1/auth/device-replacements/complete": {"enrollment-completions-source", "one-attempt-per-challenge"},
		"POST /v1/auth/refresh":                      {"refresh-source", "refresh-family"},
		"GET /health/live":                           {"health-live-source"},
		"GET /health/ready":                          {"health-ready-source"},
	}
	groups := completeObject(t, context.rateLimits["groups"])
	seenRoutes := make(map[string]bool, len(expectedRoutes))
	for _, raw := range completeArray(t, context.rateLimits["unauthenticated_allowlist"]) {
		binding := completeObject(t, raw)
		route := completeString(t, binding["route"])
		want, exists := expectedRoutes[route]
		if !exists {
			t.Errorf("unexpected unauthenticated route %q", route)
			continue
		}
		if seenRoutes[route] {
			t.Errorf("duplicate unauthenticated route %q", route)
		}
		seenRoutes[route] = true
		got := completeStringArray(t, binding["groups"])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s rate-limit groups=%v, want %v", route, got, want)
		}
		for _, group := range got {
			if _, exists := groups[group]; !exists {
				t.Errorf("%s references unknown rate-limit group %q", route, group)
			}
		}
	}
	if len(seenRoutes) != len(expectedRoutes) {
		t.Errorf("unauthenticated route mappings=%d, want %d", len(seenRoutes), len(expectedRoutes))
	}

	expectedScopes := map[string]string{
		"password-starts-source":        "SOURCE",
		"enrollment-completions-source": "SOURCE",
		"one-attempt-per-challenge":     "CHALLENGE",
		"refresh-source":                "SOURCE",
		"refresh-family":                "REFRESH_FAMILY",
		"health-live-source":            "SOURCE",
		"health-ready-source":           "SOURCE",
		"authenticated-session":         "SESSION",
		"authenticated-source":          "SOURCE",
		"websocket-session":             "SESSION",
		"websocket-source":              "SOURCE",
	}
	if len(groups) != len(expectedScopes) {
		t.Fatalf("rate-limit group inventory=%d, want %d", len(groups), len(expectedScopes))
	}
	for name, scope := range expectedScopes {
		group := completeObject(t, groups[name])
		if got := completeString(t, group["scope"]); got != scope {
			t.Errorf("%s scope=%q, want %q", name, got, scope)
		}
		wantAggregate := name == "password-starts-source" || name == "enrollment-completions-source"
		gotAggregate, hasAggregate := group["aggregate_routes"].(bool)
		if wantAggregate != (hasAggregate && gotAggregate) {
			t.Errorf("%s aggregate_routes=%v present=%v, want %v", name, gotAggregate, hasAggregate, wantAggregate)
		}
		if !wantAggregate && hasAggregate {
			t.Errorf("%s unexpectedly declares aggregate_routes", name)
		}
	}
}

func completeEnrollmentEffectCases(t *testing.T, context *completeFixtureContext, cases []any) {
	expectedPurposes := []string{"ADDITIONAL_DEVICE", "REPLACEMENT_DEVICE"}
	enrollment := completeObject(t, context.security["enrollment"])
	for index, raw := range cases {
		item := completeObject(t, raw)
		purpose := completeString(t, item["purpose"])
		if purpose != expectedPurposes[index] {
			t.Fatalf("enrollment effect %d purpose=%s", index, purpose)
		}
		mode := "ordinary"
		if purpose == "REPLACEMENT_DEVICE" {
			mode = "replacement"
		}
		rules := completeObject(t, enrollment[mode])
		runtime := newScenarioEnrollmentRuntime()
		const userID = "10000000-0000-4000-8000-000000000001"
		const accountID = "20000000-0000-4000-8000-000000000001"
		runtime.seedPrior(userID)
		input, wire, completion := completeSignedEnrollment(t, purpose, index)
		completeStrictValid(t, context.validator, "EnrollmentStartInput", input)
		completeStrictValid(t, context.validator, "EnrollmentChallenge", wire)
		completeStrictValid(t, context.validator, "EnrollmentCompletionInput", completion)
		if !runtime.start(wire, userID, accountID, true, true, true) {
			t.Fatalf("%s effect transition did not persist its challenge", purpose)
		}
		if got := runtime.complete(completion, completeTime(t, "2026-07-29T10:01:00Z")); got != "CONSUMED_SUCCESS" {
			t.Fatalf("%s effect completion=%s", purpose, got)
		}
		deviceActive, sessionActive, familyActive := runtime.priorActive()
		gotActive := []bool{deviceActive, sessionActive, familyActive}
		ruleKeys := []string{"prior_devices", "prior_sessions", "prior_refresh_families"}
		fixtureKeys := []string{"expected_prior_devices", "expected_prior_sessions", "expected_prior_families"}
		for effectIndex, active := range gotActive {
			manifestEffect := completeString(t, rules[ruleKeys[effectIndex]])
			wantActive := manifestEffect == "PRESERVE"
			if manifestEffect != "PRESERVE" && manifestEffect != "REVOKE_ALL" {
				t.Fatalf("%s has unknown manifest effect %q", purpose, manifestEffect)
			}
			if active != wantActive {
				t.Errorf("%s %s active=%v, want %v", purpose, ruleKeys[effectIndex], active, wantActive)
			}
			gotFixtureEffect := "REVOKED_BEFORE_NEW_SESSION"
			if active {
				gotFixtureEffect = "PRESERVED"
			}
			if want := completeString(t, item[fixtureKeys[effectIndex]]); gotFixtureEffect != want {
				t.Errorf("%s %s=%s, want %s", purpose, fixtureKeys[effectIndex], gotFixtureEffect, want)
			}
		}
		counts := runtime.counts()
		if counts.devices != 2 || counts.sessions != 2 || counts.refreshFamilies != 2 || counts.enrollments != 1 {
			t.Errorf("%s post-transition entity counts=%+v", purpose, counts)
		}
		if got := completeString(t, item["new_session_origin"]); got != "SERVER_RANDOM_INDEPENDENT" {
			t.Errorf("%s session origin=%s", purpose, got)
		}
	}
}

func completeSignedEnrollment(t *testing.T, purpose string, index int) (map[string]any, map[string]any, map[string]any) {
	t.Helper()
	seed := sha256.Sum256([]byte("fit-platform-enrollment-effect:" + purpose))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicKeyWire := "ed25519-public:" + hex.EncodeToString(publicKey)
	fingerprint, err := scenarioEd25519Fingerprint(publicKeyWire)
	if err != nil {
		t.Fatal(err)
	}
	handle := fmt.Sprintf("synthetic_effect_handle_%d_0000000000000001", index)
	wire := map[string]any{
		"schema_version":                   "fit.platform.enrollment-challenge.v1",
		"domain":                           "FIT_TRADE_DEVICE_ENROLLMENT_V1",
		"purpose":                          purpose,
		"subject_handle":                   handle,
		"candidate_public_key_fingerprint": fingerprint,
		"nonce":                            fmt.Sprintf("synthetic_effect_nonce_%d_00000000000000001", index),
		"issued_at":                        "2026-07-29T10:00:00Z",
		"expires_at":                       "2026-07-29T10:02:00Z",
		"single_use":                       true,
	}
	signingBytes, err := ChallengeSigningBytes(wire)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"identifier":                       "known-user@example.invalid",
		"password_utf8":                    "synthetic-password-vector-only",
		"purpose":                          purpose,
		"candidate_public_key_fingerprint": fingerprint,
	}
	completion := map[string]any{
		"subject_handle":       handle,
		"candidate_public_key": publicKeyWire,
		"signature":            "ed25519-signature:" + hex.EncodeToString(ed25519.Sign(privateKey, signingBytes)),
	}
	return input, wire, completion
}

func completeDeviceKeyCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"unused candidate key registers",
		"same-owner duplicate key rejects",
		"cross-owner key rebinding rejects",
		"revoked registered key rejects device action",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		outcome := ""
		switch {
		case completeString(t, item["operation"]) == "DEVICE_ACTION" && completeBool(t, item["key_revoked"]):
			outcome = "REJECT_REVOKED_KEY"
		case completeString(t, item["existing_owner"]) == "NONE":
			outcome = "REGISTER"
		case completeString(t, item["existing_owner"]) == completeString(t, item["requested_owner"]):
			outcome = "REJECT_DUPLICATE_KEY"
		default:
			outcome = "REJECT_CROSS_OWNER_REBIND"
		}
		completeDecision(t, item, outcome)
	}
}

func completeSessionFixationCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	expectedWorkflows := []string{"LOGIN", "ORDINARY_ENROLLMENT", "REPLACEMENT_ENROLLMENT"}
	effects := map[string]string{
		"LOGIN":                  "INVALIDATE_PREAUTHENTICATION_IDENTIFIER",
		"ORDINARY_ENROLLMENT":    "PRESERVE_EXISTING_AUTHENTICATED_SCOPE",
		"REPLACEMENT_ENROLLMENT": "REVOKE_EXISTING_AUTHENTICATED_SCOPE_BEFORE_CREATE",
	}
	for index, raw := range cases {
		item := completeObject(t, raw)
		workflow := completeString(t, item["workflow"])
		if workflow != expectedWorkflows[index] {
			t.Fatalf("session fixation case %d workflow=%s", index, workflow)
		}
		preauth := completeString(t, item["presented_preauthentication_id"])
		session := completeString(t, item["created_session_id"])
		family := completeString(t, item["created_family_id"])
		if preauth == session || preauth == family || session == family {
			t.Errorf("%s reuses a server identity", workflow)
		}
		if completeString(t, item["prior_scope_effect"]) != effects[workflow] {
			t.Errorf("%s prior scope effect drift", workflow)
		}
	}
}

func completeRevocationDeadlineCases(t *testing.T, context *completeFixtureContext, cases []any) {
	revocation := completeObject(t, context.security["revocation"])
	wantIngress := completeStringArray(t, revocation["ingress"])
	actualIngress := make([]string, 0, len(cases))
	maximumMS := completeInt(t, revocation["maximum_propagation_seconds"]) * 1000
	for _, raw := range cases {
		item := completeObject(t, raw)
		ingress := completeString(t, item["ingress"])
		actualIngress = append(actualIngress, ingress)
		got, err := scenarioRevocationOutcome(ingress, completeInt(t, item["elapsed_ms"]), maximumMS)
		if err != nil {
			t.Errorf("%s: %v", ingress, err)
			continue
		}
		if want := completeString(t, item["expected"]); got != want {
			t.Errorf("%s revocation decision=%s, want %s", ingress, got, want)
		}
	}
	sort.Strings(actualIngress)
	sort.Strings(wantIngress)
	if !reflect.DeepEqual(actualIngress, wantIngress) {
		t.Fatalf("revocation ingress coverage=%v, want %v", actualIngress, wantIngress)
	}
	if _, err := scenarioRevocationOutcome("HTTP", maximumMS+1, maximumMS); err == nil {
		t.Error("revocation model accepted propagation after the frozen deadline")
	}
}

func completeWALCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	wantSignatures := []string{
		"false/59/0/false/HEALTHY",
		"false/60/0/false/HEALTHY",
		"false/61/0/false/OPEN_ONE_INCIDENT",
		"true/0/0/false/OPEN_ONE_INCIDENT",
		"true/90/0/true/KEEP_SAME_INCIDENT_NO_DUPLICATE",
		"false/60/1/true/CLEAR_AFTER_SECOND_SUCCESS",
		"false/61/2/true/KEEP_SAME_INCIDENT_NO_DUPLICATE",
	}
	for index, raw := range cases {
		item := completeObject(t, raw)
		signature := fmt.Sprintf(
			"%t/%d/%d/%t/%s",
			completeBool(t, item["archive_command_failed"]),
			completeInt(t, item["archive_age_seconds"]),
			completeInt(t, item["consecutive_healthy_cycles"]),
			completeBool(t, item["incident_open_before"]),
			completeString(t, item["expected"]),
		)
		if signature != wantSignatures[index] {
			t.Fatalf("WAL case %d signature=%s, want %s", index, signature, wantSignatures[index])
		}
		got := scenarioWALOutcome(
			completeBool(t, item["archive_command_failed"]),
			completeInt(t, item["archive_age_seconds"]),
			completeInt(t, item["consecutive_healthy_cycles"]),
			completeBool(t, item["incident_open_before"]),
		)
		completeDecision(t, item, got)
	}
}

func completeCrossRouteThrottleCase(t *testing.T, context *completeFixtureContext, item map[string]any) {
	routes := make(map[string]bool)
	for _, route := range completeStringArray(t, completeObject(t, context.security["password_throttle"])["routes"]) {
		routes[route] = true
	}
	var accountFailures, sourceFailures int64
	attempts := completeArray(t, item["attempts"])
	if len(attempts) != 5 {
		t.Fatalf("cross-route attempts=%d, want 5", len(attempts))
	}
	covered := make(map[string]bool)
	for _, raw := range attempts {
		attempt := completeObject(t, raw)
		route := completeString(t, attempt["route"])
		if !routes[route] {
			t.Fatalf("cross-route fixture uses unfrozen route %q", route)
		}
		covered[route] = true
		if !completeBool(t, attempt["success"]) {
			sourceFailures++
			if completeBool(t, attempt["resolved_user"]) {
				accountFailures++
			}
		}
	}
	if len(covered) != len(routes) {
		t.Errorf("cross-route fixture covers %d/%d password routes", len(covered), len(routes))
	}
	if accountFailures != completeInt(t, item["expected_account_failures"]) ||
		sourceFailures != completeInt(t, item["expected_source_failures"]) {
		t.Errorf("cross-route failures=%d/%d", accountFailures, sourceFailures)
	}
	if got := accountFailures >= 5 && sourceFailures >= 5; got != completeBool(t, item["expected_both_locked"]) {
		t.Errorf("cross-route locked=%v", got)
	}
}

func completeForwardingHeaderCase(t *testing.T, context *completeFixtureContext, item map[string]any) {
	sourcePolicy := completeObject(t, context.security["source_key"])
	if completeString(t, sourcePolicy["forwarding_headers"]) != "IGNORE" {
		t.Fatal("source policy no longer ignores forwarding headers")
	}
	direct, err := scenarioCanonicalIP(completeString(t, item["direct_peer_ip"]))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(direct[:]); got != completeString(t, item["expected_canonical_ip_hex"]) {
		t.Errorf("direct peer canonical IP=%s", got)
	}
	forwarded, err := scenarioCanonicalIP(completeString(t, item["forwarding_header_value"]))
	if err != nil {
		t.Fatal(err)
	}
	if direct == forwarded {
		t.Error("fixture does not distinguish direct peer from forwarding header")
	}
}

func completeRecoveryCalculationCase(t *testing.T, _ *completeFixtureContext, item map[string]any) {
	sequenceGap := completeInt(t, item["last_committed_sequence"]) - completeInt(t, item["last_recovered_sequence"])
	timeGap := completeTime(t, item["last_committed_at"]) - completeTime(t, item["last_recovered_at"])
	rto := completeTime(t, item["verification_completed_at"]) - completeTime(t, item["restore_invoked_at"])
	if sequenceGap != completeInt(t, item["expected_sequence_gap"]) ||
		timeGap != completeInt(t, item["expected_time_gap_ms"]) ||
		rto != completeInt(t, item["expected_rto_ms"]) {
		t.Errorf("recovery calculation=%d/%d/%d", sequenceGap, timeGap, rto)
	}
}

func completeIdentityDirectory(t *testing.T, context *completeFixtureContext, cases []any) {
	identity := completeObject(t, cases[0])
	if completeString(t, identity["identifier"]) != "known-user@example.invalid" {
		t.Errorf("identity fixture identifier drift")
	}
	for _, key := range []string{"user_id", "trading_account_id", "source_key", "test_only_password_sha256"} {
		if completeString(t, identity[key]) == "" {
			t.Errorf("identity fixture lacks %s", key)
		}
	}
	source := completeString(t, identity["source_key"])
	if !strings.HasPrefix(source, "src_") || len(source) != 68 {
		t.Errorf("identity source key has invalid pseudonymous wire form")
	}
	password := completeString(t, completeObject(t, context.golden["argon2id_production"])["password_utf8"])
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))
	if digest != completeString(t, identity["test_only_password_sha256"]) {
		t.Error("identity test-only password digest does not bind the golden password")
	}
	verifier := completeObject(t, identity["password_argon2id"])
	golden := completeObject(t, context.golden["argon2id_production"])
	for _, key := range []string{"profile_id", "version", "salt_hex", "result_hex"} {
		if CanonicalJSON(strictValue(t, verifier[key])) != CanonicalJSON(strictValue(t, golden[key])) {
			t.Errorf("identity Argon2 verifier %s drift", key)
		}
	}
	if completeInt(t, golden["memory_kib"]) != 65_536 ||
		completeInt(t, golden["iterations"]) != 3 ||
		completeInt(t, golden["parallelism"]) != 1 ||
		completeInt(t, golden["result_bytes"]) != 32 {
		t.Error("golden Argon2 production cost profile drift")
	}
	passwordPolicy := completeObject(t, context.security["password_hashing"])
	profile := completeObject(t, passwordPolicy["production_profile"])
	if verifier["profile_id"] != profile["profile_id"] || verifier["version"] != passwordPolicy["version"] {
		t.Error("identity verifier is not bound to the frozen production Argon2id profile")
	}
	if !completeArgon2Matches(t, password, verifier, profile) {
		t.Error("frozen production Argon2id verifier rejected its password")
	}
	if completeArgon2Matches(t, password+"-wrong", verifier, profile) {
		t.Error("frozen production Argon2id verifier accepted a wrong password")
	}
}

func completeOwnershipRecords(t *testing.T, context *completeFixtureContext, cases []any) {
	ownership := completeObject(t, cases[0])
	completeStrictValid(t, context.validator, "TradingAccountOwnership", ownership)
	if completeString(t, ownership["ownership_id"]) != "10000000-0000-4000-8000-000000000002" ||
		completeString(t, ownership["role"]) != "OWNER" ||
		completeString(t, ownership["status"]) != "ACTIVE" {
		t.Error("ownership fixture is not the frozen active owner")
	}
	identity := completeObject(t, completeArray(t, context.executable["identity_directory"])[0])
	if ownership["user_id"] != identity["user_id"] ||
		ownership["trading_account_id"] != identity["trading_account_id"] {
		t.Error("ownership fixture is not bound to the executable identity")
	}
}

func completeEnrollmentTransitions(t *testing.T, context *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"unknown identifier ordinary path has no server state",
		"repeated unknown identifier gets unrelated process-local handle",
		"known identifier wrong password ordinary path has no state",
		"known identifier wrong password replacement path has no state",
		"known identifier correct password persists and completes",
	})
	identity := completeObject(t, completeArray(t, context.executable["identity_directory"])[0])
	ownership := completeObject(t, completeArray(t, context.executable["ownership_records"])[0])
	goldenProofs := completeObject(t, context.golden["ed25519_proofs"])
	publicKey := completeString(t, goldenProofs["public_key"])
	var enrollmentSignature string
	for _, raw := range completeArray(t, goldenProofs["vectors"]) {
		vector := completeObject(t, raw)
		if completeString(t, vector["challenge_schema"]) == "EnrollmentChallenge" {
			enrollmentSignature = completeString(t, vector["signature"])
		}
	}
	if enrollmentSignature == "" {
		t.Fatal("golden enrollment proof is missing")
	}
	verifier := completeObject(t, identity["password_argon2id"])
	profile := completeObject(t, completeObject(t, context.security["password_hashing"])["production_profile"])
	passwordResults := make(map[string]bool)

	handles := make(map[string]bool)
	successes := 0
	for _, raw := range cases {
		item := completeObject(t, raw)
		name := completeString(t, item["name"])
		input := completeObject(t, item["input"])
		wire := completeObject(t, item["wire"])
		completeStrictValid(t, context.validator, "EnrollmentStartInput", input)
		completeStrictValid(t, context.validator, "EnrollmentChallenge", wire)

		handle := completeString(t, wire["subject_handle"])
		if handles[handle] {
			t.Errorf("%s reused a process-local subject handle", name)
		}
		handles[handle] = true
		if wire["purpose"] != input["purpose"] ||
			wire["candidate_public_key_fingerprint"] != input["candidate_public_key_fingerprint"] {
			t.Errorf("%s challenge is not bound to its start input", name)
		}
		if completeString(t, wire["domain"]) != "FIT_TRADE_DEVICE_ENROLLMENT_V1" ||
			!completeBool(t, wire["single_use"]) ||
			completeTime(t, wire["expires_at"])-completeTime(t, wire["issued_at"]) != 120_000 {
			t.Errorf("%s challenge window/domain drift", name)
		}
		for _, forbidden := range []string{"user_id", "trading_account_id", "device_id", "session_id", "refresh_family_id"} {
			if _, exists := wire[forbidden]; exists {
				t.Errorf("%s leaks server authority field %s", name, forbidden)
			}
		}

		password := completeString(t, input["password_utf8"])
		passwordValid, verified := passwordResults[password]
		if !verified {
			passwordValid = completeArgon2Matches(t, password, verifier, profile)
			passwordResults[password] = passwordValid
		}
		knownIdentity := input["identifier"] == identity["identifier"]
		activeOwnership := ownership["user_id"] == identity["user_id"] &&
			ownership["trading_account_id"] == identity["trading_account_id"] &&
			ownership["role"] == "OWNER" && ownership["status"] == "ACTIVE"
		runtime := newScenarioEnrollmentRuntime()
		persisted := runtime.start(
			wire,
			completeString(t, identity["user_id"]),
			completeString(t, identity["trading_account_id"]),
			knownIdentity,
			passwordValid,
			activeOwnership,
		)
		if persisted != completeBool(t, item["expected_start_persisted"]) {
			t.Errorf("%s persisted=%v", name, persisted)
		}

		proofExpected := completeBool(t, completeObject(t, item["completion"])["proof_valid"])
		signature := enrollmentSignature
		if !proofExpected {
			signature = "ed25519-signature:" + strings.Repeat("0", ed25519.SignatureSize*2)
		}
		completion := map[string]any{
			"subject_handle":       handle,
			"candidate_public_key": publicKey,
			"signature":            signature,
		}
		completeStrictValid(t, context.validator, "EnrollmentCompletionInput", completion)
		outcome := runtime.complete(completion, completeTime(t, "2026-07-29T10:01:00Z"))
		if outcome != completeString(t, item["expected_completion"]) {
			t.Errorf("%s completion=%s", name, outcome)
		}
		if outcome == "CONSUMED_SUCCESS" {
			successes++
		}
		counts := runtime.counts()
		expected := completeObject(t, item["expected_durable_counts"])
		gotCounts := map[string]int{
			"challenges":       counts.challenges,
			"devices":          counts.devices,
			"sessions":         counts.sessions,
			"refresh_families": counts.refreshFamilies,
			"enrollments":      counts.enrollments,
			"audit_events":     counts.auditEvents,
			"notifications":    counts.notifications,
			"outbox_records":   counts.outboxRecords,
		}
		for key, got := range gotCounts {
			if want := int(completeInt(t, expected[key])); got != want {
				t.Errorf("%s durable %s=%d, want %d", name, key, got, want)
			}
		}
		if persisted {
			beforeReplay := runtime.counts()
			if replay := runtime.complete(completion, completeTime(t, "2026-07-29T10:01:01Z")); replay != "REJECT_REPLAY" {
				t.Errorf("%s replay completion=%s", name, replay)
			}
			if afterReplay := runtime.counts(); afterReplay != beforeReplay {
				t.Errorf("%s replay mutated durable state: before=%+v after=%+v", name, beforeReplay, afterReplay)
			}
		}
	}
	if len(handles) != 5 || successes != 1 {
		t.Fatalf("enrollment transitions handles=%d successes=%d", len(handles), successes)
	}
}

func completePasswordThrottleTimelines(t *testing.T, context *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"resolved user delay lock expiry and atomic concurrency",
		"unknown identifier locks only source and either dimension denies",
		"resolved account locks across distinct sources",
		"failures outside rolling window do not accumulate",
		"one shared cross-route transaction state with success reset and abort",
		"source and account delay schedules remain independent",
		"aborted prune rolls back the complete transaction",
	})
	manifest := completeObject(t, context.security["password_throttle"])
	routes := make(map[string]bool)
	for _, route := range completeStringArray(t, manifest["routes"]) {
		routes[route] = true
	}
	delays := completeArray(t, manifest["failure_delays_seconds"])
	config := scenarioThrottleConfig{
		windowMS:    completeInt(t, manifest["window_seconds"]) * 1000,
		lockOn:      int(completeInt(t, manifest["lock_on_failure"])),
		lockMS:      completeInt(t, manifest["lock_seconds"]) * 1000,
		knownRoutes: routes,
	}
	for _, raw := range delays {
		config.delaysMS = append(config.delaysMS, completeInt(t, raw)*1000)
	}
	eventCount := 0
	for _, raw := range cases {
		timeline := completeObject(t, raw)
		model := newScenarioThrottleModel(config)
		defaultSource := completeOptionalString(timeline["source_key"])
		for index, eventRaw := range completeArray(t, timeline["events"]) {
			eventCount++
			item := completeObject(t, eventRaw)
			source := completeOptionalString(item["source_key"])
			if source == "" {
				source = defaultSource
			}
			event := scenarioThrottleEvent{
				nowMS:              int64(completeFloat(t, item["at_seconds"]) * 1000),
				userID:             completeOptionalString(item["user_id"]),
				sourceKey:          source,
				route:              completeOptionalString(item["route"]),
				passwordValid:      completeBool(t, item["password_valid"]),
				transactionAborted: completeOptionalBool(item["transaction_aborted"], false),
			}
			observation, err := model.apply(event)
			if err != nil {
				t.Fatalf("%s event %d: %v", timeline["name"], index, err)
			}
			if observation.decision != completeString(t, item["expected_decision"]) {
				t.Errorf("%s event %d decision=%s", timeline["name"], index, observation.decision)
			}
			if int64(observation.accountFailures) != completeInt(t, item["expected_account_failures"]) ||
				int64(observation.sourceFailures) != completeInt(t, item["expected_source_failures"]) {
				t.Errorf("%s event %d failures=%d/%d", timeline["name"], index, observation.accountFailures, observation.sourceFailures)
			}
			if !observation.routeCoveredByContract {
				t.Errorf("%s event %d used an unfrozen route", timeline["name"], index)
			}
			completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_delay_seconds", observation.appliedDelayMS)
			completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_lock_until_seconds", observation.sourceLockedUntilMS)
			if _, exists := item["expected_lock_until_seconds"]; exists && event.userID != "" {
				completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_lock_until_seconds", observation.accountLockedUntilMS)
			}
			completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_source_lock_until_seconds", observation.sourceLockedUntilMS)
			completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_account_lock_until_seconds", observation.accountLockedUntilMS)
			completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_source_next_allowed_seconds", observation.sourceNextAllowedMS)
			completeOptionalMillisecondsEvidence(t, timeline, index, item, "expected_account_next_allowed_seconds", observation.accountNextAllowedMS)
			if rawTimes, exists := item["expected_source_failure_times_seconds"]; exists {
				wantRaw := completeArray(t, rawTimes)
				want := make([]int64, len(wantRaw))
				for i, value := range wantRaw {
					want[i] = int64(completeFloat(t, value) * 1000)
				}
				if !reflect.DeepEqual(observation.sourceFailureTimesMS, want) {
					t.Errorf("%s event %d source failure times=%v, want %v", timeline["name"], index, observation.sourceFailureTimesMS, want)
				}
			}
		}
	}
	if eventCount != 35 {
		t.Fatalf("executed throttle timeline events=%d, want exactly 35", eventCount)
	}
}

func completeWebSocketUpgradeCases(t *testing.T, context *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"exact server binding is accepted",
		"user mismatch rejects",
		"account mismatch rejects",
		"device mismatch rejects",
		"session mismatch rejects",
		"family mismatch rejects",
		"invalid token signature rejects",
		"expired access token rejects",
		"revoked session rejects",
		"revoked device rejects",
		"revoked refresh family rejects",
		"disabled user rejects",
		"revoked account ownership rejects",
	})
	binding := fixtureCase(t, "server-authored websocket binding")
	fields := []string{"user_id", "trading_account_id", "device_id", "session_id", "refresh_family_id"}
	for _, raw := range cases {
		item := completeObject(t, raw)
		claims := make(map[string]any, len(fields))
		for _, field := range fields {
			claims[field] = binding[field]
		}
		if mismatch := completeOptionalString(item["mismatch_field"]); mismatch != "" {
			if _, exists := claims[mismatch]; !exists {
				t.Fatalf("%s uses unknown mismatch field %q", item["name"], mismatch)
			}
			claims[mismatch] = "99000000-0000-4000-8000-000000000099"
		}
		matches := true
		for _, field := range fields {
			matches = matches && claims[field] == binding[field]
		}
		accepted := matches &&
			completeBool(t, item["token_signature_valid"]) &&
			completeBool(t, item["token_not_expired"]) &&
			completeOptionalBool(item["user_active"], true) &&
			completeOptionalBool(item["account_ownership_active"], true) &&
			completeBool(t, item["session_active"]) &&
			completeBool(t, item["device_active"]) &&
			completeBool(t, item["refresh_family_active"])
		got := "REJECT"
		if accepted {
			got = "ACCEPT"
		}
		completeDecision(t, item, got)
	}
}

func completeWebSocketRedactionCases(t *testing.T, context *completeFixtureContext, cases []any) {
	websocket := completeObject(t, context.security["websocket_reauthorization"])
	wantFields := completeStringArray(t, websocket["redacted_fields"])
	wantSinks := completeStringArray(t, websocket["redaction_sinks"])
	actualFields := make([]string, 0, len(cases))
	denied := make(map[string]bool, len(wantFields))
	for _, field := range wantFields {
		denied[authorityAlias(field)] = true
	}
	sort.Strings(wantSinks)
	for _, raw := range cases {
		item := completeObject(t, raw)
		actualFields = append(actualFields, completeString(t, item["field"]))
		sinks := completeStringArray(t, item["forbidden_sinks"])
		sort.Strings(sinks)
		if !reflect.DeepEqual(sinks, wantSinks) {
			t.Errorf("%s redaction sinks=%v", item["field"], sinks)
		}
	}
	sort.Strings(actualFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(actualFields, wantFields) {
		t.Fatalf("websocket redaction fields=%v, want %v", actualFields, wantFields)
	}
	if completeBool(t, websocket["sensitive_control_data_in_logs_traces_metrics_application_messages"]) {
		t.Error("WebSocket sensitive control data was enabled for observable sinks")
	}

	var controlRaw string
	for _, raw := range completeArray(t, context.executable["raw_entrypoint_cases"]) {
		item := completeObject(t, raw)
		if completeString(t, item["name"]) == "WebSocket valid control frame strict JSON" {
			controlRaw = completeString(t, item["raw"])
			break
		}
	}
	if controlRaw == "" {
		t.Fatal("valid WebSocket control fixture is missing")
	}
	parsed, err := ParseStrictJSON([]byte(controlRaw))
	if err != nil {
		t.Fatal(err)
	}
	control := completeObject(t, parsed)
	accessToken := completeString(t, control["access_token_transport"])
	nonce := completeString(t, control["nonce"])
	observation := map[string]any{
		"event":                  "websocket-control",
		"access_token_transport": accessToken,
		"nonce":                  nonce,
		"raw_control_frame":      controlRaw,
		"nested": []any{map[string]any{
			"nonce": nonce,
		}},
	}
	redacted := CanonicalJSON(scenarioRedactedObservation(observation, denied))
	if redacted == "" || !strings.Contains(redacted, "[REDACTED]") {
		t.Fatal("WebSocket observability redactor produced no redacted evidence")
	}
	for _, sensitive := range []string{accessToken, nonce, controlRaw} {
		if strings.Contains(redacted, sensitive) {
			t.Error("WebSocket observability redactor retained a sensitive value")
		}
	}
	for _, sink := range wantSinks {
		if !strings.Contains(redacted, `"event":"websocket-control"`) {
			t.Errorf("%s redaction destroyed non-sensitive observability data", sink)
		}
	}
	typed, err := context.validator.DecodeTyped("WebSocketReauth", []byte(controlRaw))
	if err != nil {
		t.Fatal(err)
	}
	reauth, ok := typed.(WebSocketReauth)
	if !ok {
		t.Fatalf("WebSocketReauth decoded as %T", typed)
	}
	if marshaled, err := json.Marshal(reauth); err == nil {
		t.Errorf("transport-only WebSocketReauth marshaled as %s", marshaled)
	}
	formatted := fmt.Sprintf("%v", reauth)
	if strings.Contains(formatted, accessToken) || strings.Contains(formatted, nonce) ||
		!strings.Contains(formatted, "[REDACTED]") {
		t.Error("WebSocketReauth formatter did not redact transport data")
	}
}

func completeRawRequestTargetCases(t *testing.T, _ *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"exact confirmation target",
		"confirmation route rejects every query parameter",
		"uppercase general UUID remains compatible",
		"UUIDv7 remains compatible",
		"UUID version zero is rejected",
		"UUID invalid variant is rejected",
		"duplicate raw query key",
		"duplicate decoded query key",
		"percent encoded path byte",
		"dot path segment",
		"empty path segment",
		"fragment is forbidden",
	})
	for _, raw := range cases {
		item := completeObject(t, raw)
		path, query, err := ParseConfirmationTarget(completeString(t, item["raw_target"]))
		got := "REJECT"
		if err == nil {
			got = "ACCEPT"
			id := completeString(t, path["confirmation_id"])
			if id != strings.ToLower(id) || len(query) != 0 {
				t.Errorf("%s did not produce canonical path/query", item["name"])
			}
		}
		completeDecision(t, item, got)
	}
}

func completeRawEntrypointCases(t *testing.T, context *completeFixtureContext, cases []any) {
	completeRequireNames(t, cases, []string{
		"HTTP mutation valid strict JSON",
		"HTTP nested duplicate key rejects before handler",
		"HTTP NBSP is not JSON whitespace",
		"HTTP prototype mutation key rejects",
		"WebSocket valid control frame strict JSON",
		"WebSocket duplicate nonce rejects before dispatch",
		"WebSocket constructor key rejects before dispatch",
	})
	requestDigestPolicy := completeObject(t, context.security["request_digest"])
	strictDecoder := completeObject(t, requestDigestPolicy["strict_raw_json_decoder"])
	entrypoints := completeObject(t, strictDecoder["entry_points"])
	expectedEntrypoints := map[string]string{
		"HTTP_MUTATION_BODY":      "BEFORE_HANDLER_AND_SCHEMA_VALIDATION",
		"WEBSOCKET_CONTROL_FRAME": "BEFORE_CONTROL_FRAME_DISPATCH_AND_SCHEMA_VALIDATION",
	}
	if len(entrypoints) != len(expectedEntrypoints) {
		t.Fatalf("strict decoder entry points=%d, want %d", len(entrypoints), len(expectedEntrypoints))
	}
	for name, want := range expectedEntrypoints {
		if got := completeString(t, entrypoints[name]); got != want {
			t.Errorf("strict decoder entry point %s=%q, want %q", name, got, want)
		}
	}
	for _, raw := range cases {
		item := completeObject(t, raw)
		entrypoint := completeString(t, item["entry_point"])
		if _, bound := entrypoints[entrypoint]; !bound {
			t.Fatalf("%s uses unhandled entry point %q", item["name"], entrypoint)
		}
		typed, err := context.validator.DecodeTyped(
			completeString(t, item["schema"]),
			[]byte(completeString(t, item["raw"])),
		)
		got := "REJECT"
		if err == nil {
			got = "ACCEPT"
			if typed.SchemaName() != completeString(t, item["schema"]) {
				t.Errorf("%s decoded as %s", item["name"], typed.SchemaName())
			}
			if entrypoint == "HTTP_MUTATION_BODY" {
				parsed, parseErr := ParseStrictJSON([]byte(completeString(t, item["raw"])))
				if parseErr != nil {
					t.Errorf("%s accepted typed input that strict parsing rejected: %v", item["name"], parseErr)
				} else {
					decoded := completeObject(t, parsed)
					request := map[string]any{
						"schema_version":     "fit.platform.request-digest.v1",
						"method":             "POST",
						"route_template":     "/v1/confirmations/{confirmation_id}/consume",
						"path":               decoded["path"],
						"query":              decoded["query"],
						"body":               decoded["body"],
						"user_id":            "10000000-0000-4000-8000-000000000001",
						"trading_account_id": "20000000-0000-4000-8000-000000000001",
					}
					parsedDigest, digestErr := RequestDigest(request)
					normalizedDigest, normalizedErr := RequestDigest(completeNormalizedObject(t, request))
					if digestErr != nil || normalizedErr != nil || parsedDigest != normalizedDigest || len(parsedDigest) != 64 {
						t.Errorf("%s request digest binding failed: parsed_err=%v normalized_err=%v", item["name"], digestErr, normalizedErr)
					}
				}
			}
		}
		completeDecision(t, item, got)
	}
}

func completeOptionalMillisecondsEvidence(
	t *testing.T,
	timeline map[string]any,
	eventIndex int,
	item map[string]any,
	key string,
	gotMS int64,
) {
	t.Helper()
	raw, exists := item[key]
	if !exists {
		return
	}
	wantMS := int64(completeFloat(t, raw) * 1000)
	if gotMS != wantMS {
		t.Errorf("%s event %d %s=%dms, want %dms", timeline["name"], eventIndex, key, gotMS, wantMS)
	}
}

func completeNATSPermissionMatrix(t *testing.T, context *completeFixtureContext) {
	t.Helper()
	if got := completeString(t, context.nats["schema_version"]); got != "fit.platform.nats-permissions.v1" {
		t.Fatalf("NATS schema version=%q", got)
	}
	stream := completeObject(t, context.nats["stream"])
	subjects := completeStringArray(t, stream["subjects"])
	if completeString(t, stream["name"]) != "FIT_PLATFORM_V1" ||
		completeString(t, stream["delivery"]) != "AT_LEAST_ONCE" ||
		completeString(t, stream["authority"]) != "DERIVED_FROM_POSTGRESQL_OUTBOX" {
		t.Error("NATS stream authority/delivery mapping drift")
	}
	if len(subjects) != 20 {
		t.Fatalf("NATS exact subject inventory=%d, want 20", len(subjects))
	}
	subjectSet := make(map[string]bool, len(subjects))
	for _, subject := range subjects {
		if subjectSet[subject] {
			t.Errorf("duplicate NATS stream subject %q", subject)
		}
		subjectSet[subject] = true
		if strings.ContainsAny(subject, "*>") {
			t.Errorf("application NATS subject contains wildcard %q", subject)
		}
	}

	bindings := completeArray(t, context.nats["subject_bindings"])
	if len(bindings) != len(subjects) {
		t.Fatalf("NATS subject bindings=%d, want %d", len(bindings), len(subjects))
	}
	boundSubjects := make([]string, 0, len(bindings))
	filters := map[string][]string{
		"platform-consumer":              {},
		"internal-notification-consumer": {},
	}
	for _, raw := range bindings {
		binding := completeObject(t, raw)
		subject := completeString(t, binding["subject"])
		consumer := completeString(t, binding["consumer"])
		if !subjectSet[subject] {
			t.Errorf("NATS binding references unknown subject %q", subject)
		}
		if _, exists := filters[consumer]; !exists {
			t.Errorf("NATS binding references unknown consumer %q", consumer)
			continue
		}
		boundSubjects = append(boundSubjects, subject)
		filters[consumer] = append(filters[consumer], subject)
		if completeString(t, binding["event_kind"]) == "" || completeString(t, binding["scope"]) == "" {
			t.Errorf("NATS subject %q lacks event/scope binding", subject)
		}
	}
	if !reflect.DeepEqual(boundSubjects, subjects) {
		t.Error("NATS stream subjects and subject bindings are not one-to-one and ordered")
	}

	principals := make(map[string]map[string]any)
	for _, raw := range completeArray(t, context.nats["principals"]) {
		principal := completeObject(t, raw)
		name := completeString(t, principal["name"])
		if _, duplicate := principals[name]; duplicate {
			t.Fatalf("duplicate NATS principal %q", name)
		}
		principals[name] = principal
		if completeBool(t, principal["jetstream_admin"]) {
			t.Errorf("%s unexpectedly has JetStream administration", name)
		}
		if len(completeArray(t, principal["identity_claims"])) != 0 {
			t.Errorf("%s unexpectedly carries identity claims", name)
		}
	}
	if len(principals) != 3 {
		t.Fatalf("NATS principals=%d, want 3", len(principals))
	}
	publisher := principals["outbox-publisher"]
	if !reflect.DeepEqual(completeStringArray(t, publisher["publish"]), subjects) {
		t.Error("outbox publisher does not have the exact stream subject allowlist")
	}
	if got := completeStringArray(t, publisher["subscribe"]); !reflect.DeepEqual(got, []string{"_INBOX.fit-platform.outbox-publisher.>"}) {
		t.Errorf("outbox publisher subscribe permissions=%v", got)
	}
	publisherProtocol := completeObject(t, publisher["jetstream_protocol"])
	if completeString(t, publisherProtocol["publish_ack_inbox_prefix"]) != "_INBOX.fit-platform.outbox-publisher.>" ||
		completeString(t, publisherProtocol["consumer_configuration"]) != "NOT_ALLOWED" {
		t.Error("outbox publisher JetStream protocol mapping drift")
	}
	for _, subject := range subjects {
		allowed, err := context.validator.NATSAllowed("outbox-publisher", "publish", subject)
		if err != nil || !allowed {
			t.Errorf("outbox publisher denied exact subject %q: %v", subject, err)
		}
	}
	if allowed, err := context.validator.NATSAllowed("outbox-publisher", "publish", "fit.platform.v1.>"); err != nil || allowed {
		t.Errorf("outbox publisher broad wildcard decision=%v err=%v", allowed, err)
	}

	consumerDetails := map[string]string{
		"platform-consumer":              "PLATFORM_CONSUMER",
		"internal-notification-consumer": "INTERNAL_NOTIFICATION_CONSUMER",
	}
	for name, durable := range consumerDetails {
		principal := principals[name]
		inbox := "_INBOX.fit-platform." + name + ".>"
		fetch := "$JS.API.CONSUMER.MSG.NEXT.FIT_PLATFORM_V1." + durable
		ack := "$JS.ACK.FIT_PLATFORM_V1." + durable + ".>"
		if got := completeStringArray(t, principal["publish"]); !reflect.DeepEqual(got, []string{fetch, ack}) {
			t.Errorf("%s publish permissions=%v", name, got)
		}
		if got := completeStringArray(t, principal["subscribe"]); !reflect.DeepEqual(got, []string{inbox}) {
			t.Errorf("%s subscribe permissions=%v", name, got)
		}
		if got := completeStringArray(t, principal["stream_filter_subjects"]); !reflect.DeepEqual(got, filters[name]) {
			t.Errorf("%s stream filters=%v, want %v", name, got, filters[name])
		}
		for _, subject := range filters[name] {
			if strings.ContainsAny(subject, "*>") {
				t.Errorf("%s has wildcard application filter %q", name, subject)
			}
		}
		protocol := completeObject(t, principal["jetstream_protocol"])
		if completeString(t, protocol["delivery_mode"]) != "DURABLE_PULL" ||
			completeString(t, protocol["stream"]) != "FIT_PLATFORM_V1" ||
			completeString(t, protocol["durable_consumer"]) != durable ||
			completeString(t, protocol["fetch_subject"]) != fetch ||
			completeString(t, protocol["reply_inbox_prefix"]) != inbox ||
			completeString(t, protocol["ack_subject_prefix"]) != ack ||
			completeString(t, protocol["consumer_configuration"]) != "INFRASTRUCTURE_BOOTSTRAP_ONLY" {
			t.Errorf("%s JetStream protocol mapping drift", name)
		}
		for _, probe := range []struct {
			action  string
			subject string
		}{
			{"publish", fetch},
			{"publish", strings.TrimSuffix(ack, ">") + "synthetic"},
			{"subscribe", strings.TrimSuffix(inbox, ">") + "synthetic"},
		} {
			allowed, err := context.validator.NATSAllowed(name, probe.action, probe.subject)
			if err != nil || !allowed {
				t.Errorf("%s denied %s %q: %v", name, probe.action, probe.subject, err)
			}
		}
	}
	if allowed, err := context.validator.NATSAllowed(
		"platform-consumer",
		"publish",
		"$JS.ACK.FIT_PLATFORM_V1.INTERNAL_NOTIFICATION_CONSUMER.synthetic",
	); err != nil || allowed {
		t.Errorf("platform consumer cross-durable ACK decision=%v err=%v", allowed, err)
	}
	wildcards := completeObject(t, context.nats["wildcard_policy"])
	if completeString(t, wildcards["application_subjects"]) != "FORBIDDEN" ||
		completeString(t, wildcards["jetstream_protocol"]) != "ONLY_THE_EXACT_PER_PRINCIPAL_INBOX_AND_ACK_PREFIXES_LISTED_ABOVE" ||
		completeString(t, wildcards["broad_stream_or_consumer_admin"]) != "FORBIDDEN" {
		t.Error("NATS wildcard policy drift")
	}
	if completeBool(t, context.nats["api_service_has_nats_credential"]) ||
		completeBool(t, context.nats["user_or_account_claims_allowed"]) {
		t.Error("NATS credentials or identity claims escaped infrastructure scope")
	}
	semantics := completeObject(t, context.nats["event_semantics"])
	ordering := completeObject(t, semantics["ordering"])
	if completeString(t, ordering["scope"]) != "PER_AGGREGATE" ||
		!reflect.DeepEqual(completeStringArray(t, ordering["fields"]), []string{"aggregate_id", "aggregate_version"}) ||
		completeBool(t, ordering["global_order_claimed"]) {
		t.Error("NATS aggregate ordering semantics drift")
	}
	redelivery := completeObject(t, semantics["redelivery"])
	if completeString(t, redelivery["delivery"]) != "AT_LEAST_ONCE" ||
		completeString(t, redelivery["same_event_id_different_digest"]) != "REJECT_AND_ALERT" {
		t.Error("NATS redelivery semantics drift")
	}
	compatibility := completeObject(t, semantics["compatibility"])
	if completeString(t, compatibility["same_major_changes"]) != "ADDITIVE_ONLY" ||
		completeString(t, compatibility["breaking_change"]) != "NEW_VERSIONED_SUBJECT_AND_REVIEW" {
		t.Error("NATS compatibility semantics drift")
	}
}

func completeAuditNotificationMatrix(t *testing.T, context *completeFixtureContext) {
	t.Helper()
	auditKinds := make(map[string]bool)
	for _, raw := range completeArray(t, context.nats["subject_bindings"]) {
		binding := completeObject(t, raw)
		subject := completeString(t, binding["subject"])
		if strings.Contains(subject, ".notification.") {
			continue
		}
		kind := completeString(t, binding["event_kind"])
		if auditKinds[kind] {
			t.Fatalf("duplicate non-notification audit kind %q", kind)
		}
		auditKinds[kind] = true
		scopeType := completeString(t, binding["scope"])
		scope := map[string]any{"type": scopeType}
		actor := map[string]any{"type": "SERVICE", "id": "auth-service"}
		trigger := "REQUEST"
		operationField := "request_id"
		switch scopeType {
		case "OWNER":
			scope["user_id"] = "10000000-0000-4000-8000-000000000001"
			scope["trading_account_id"] = "20000000-0000-4000-8000-000000000001"
			actor = map[string]any{"type": "USER", "id": "synthetic-owner"}
		case "AUTH_SECURITY":
			scope["source_key"] = "src_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
			scope["user_id"] = "10000000-0000-4000-8000-000000000001"
		case "SYSTEM":
			actor = map[string]any{"type": "SYSTEM", "id": "wal-monitor"}
			trigger = "BACKGROUND"
			operationField = "system_operation_id"
		default:
			t.Fatalf("audit kind %s has unknown scope %s", kind, scopeType)
		}
		intent := map[string]any{
			"schema_version": "fit.platform.audit-intent.v1",
			"kind":           kind,
			"actor":          actor,
			"scope":          scope,
			"trigger":        trigger,
			"causation_id":   "80000000-0000-4000-8000-000000000001",
			"correlation_id": "b0000000-0000-4000-8000-000000000001",
			operationField:   "60000000-0000-4000-8000-000000000001",
		}
		requiredKindField := ""
		switch kind {
		case "DEVICE_ENROLLED", "DEVICE_REPLACED", "DEVICE_REVOKED":
			requiredKindField = "device_id"
			intent[requiredKindField] = "30000000-0000-4000-8000-000000000001"
		case "SESSION_REVOKED", "REFRESH_REUSE_DETECTED":
			requiredKindField = "session_id"
			intent[requiredKindField] = "40000000-0000-4000-8000-000000000001"
		}
		completeStrictValid(t, context.validator, "AuditIntent", intent)
		if requiredKindField != "" {
			mutation := completeCopyObject(intent)
			delete(mutation, requiredKindField)
			completeStrictInvalid(t, context.validator, "AuditIntent", mutation)
		}
	}
	if len(auditKinds) != 12 {
		t.Fatalf("generated audit kind matrix=%d, want 12", len(auditKinds))
	}

	notificationPolicy := completeObject(t, context.security["notification"])
	severities := completeObject(t, notificationPolicy["kind_severity"])
	detailsByKind := completeNotificationDetails()
	if len(severities) != 8 || len(detailsByKind) != len(severities) {
		t.Fatalf("notification matrix severities=%d details=%d, want 8", len(severities), len(detailsByKind))
	}
	for kind, rawSeverity := range severities {
		details, exists := detailsByKind[kind]
		if !exists {
			t.Fatalf("notification kind %s lacks construction details", kind)
		}
		severity := completeString(t, rawSeverity)
		if severity != details.severity {
			t.Errorf("%s severity manifest=%s details=%s", kind, severity, details.severity)
		}
		scope := map[string]any{"type": details.scopeType}
		if details.scopeType == "AUTH_SECURITY" {
			scope["source_key"] = "src_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
			if kind == "LOGIN_ACCOUNT_LOCKED" {
				scope["user_id"] = "10000000-0000-4000-8000-000000000001"
			}
		}
		intent := map[string]any{
			"schema_version": "fit.platform.notification-intent.v1",
			"kind":           kind,
			"severity":       severity,
			"scope":          scope,
			"message_code":   details.messageCode,
			"message_args":   details.messageArgs,
			"causation_id":   "80000000-0000-4000-8000-000000000001",
			"correlation_id": "b0000000-0000-4000-8000-000000000001",
		}
		completeStrictValid(t, context.validator, "NotificationIntent", intent)
		for _, wrongSeverity := range []string{"INFO", "WARNING", "CRITICAL"} {
			if wrongSeverity == severity {
				continue
			}
			mutation := completeCopyObject(intent)
			mutation["severity"] = wrongSeverity
			completeStrictInvalid(t, context.validator, "NotificationIntent", mutation)
		}
		for _, forbidden := range completeStringArray(t, notificationPolicy["forbidden_fields"]) {
			mutation := completeCopyObject(intent)
			mutation[forbidden] = "synthetic-forbidden-value"
			completeStrictInvalid(t, context.validator, "NotificationIntent", mutation)
		}
	}
}

type completeLinkageHandler func(*testing.T, *completeFixtureContext, map[string]any, int)

func completeLinkageRegistry() map[string]completeLinkageHandler {
	return map[string]completeLinkageHandler{
		"unknown identifier source lock": completeLinkageChain,
		"resolved account lock":          completeLinkageChain,
		"WAL interruption incident":      completeLinkageChain,
		"owner mutation":                 completeLinkageChain,
	}
}

func TestCompleteFrozenLinkageChains(t *testing.T) {
	context := newCompleteFixtureContext(t)
	if got, want := completeString(t, context.linkage["schema_version"]), "fit.platform.linkage-chains.v1"; got != want {
		t.Fatalf("linkage schema version=%q, want %q", got, want)
	}
	if len(context.linkage) != 2 {
		t.Fatalf("linkage top-level keys=%d, want exactly schema_version and chains", len(context.linkage))
	}
	completeNATSPermissionMatrix(t, context)
	completeAuditNotificationMatrix(t, context)
	chains := completeArray(t, context.linkage["chains"])
	registry := completeLinkageRegistry()
	if len(chains) != 4 || len(registry) != 4 {
		t.Fatalf("linkage inventory chains=%d handlers=%d", len(chains), len(registry))
	}
	seen := make(map[string]bool)
	totalOutbox := int64(0)
	for index, raw := range chains {
		chain := completeObject(t, raw)
		name := completeString(t, chain["name"])
		handler, exists := registry[name]
		if !exists {
			t.Fatalf("linkage chain %q appeared without a handler", name)
		}
		if seen[name] {
			t.Fatalf("duplicate linkage chain %q", name)
		}
		seen[name] = true
		totalOutbox += completeInt(t, chain["expected_outbox_records"])
		t.Run(name, func(t *testing.T) { handler(t, context, chain, index) })
	}
	for name := range registry {
		if !seen[name] {
			t.Errorf("linkage chain %q is missing", name)
		}
	}
	if totalOutbox != 7 {
		t.Fatalf("linkage expected outbox total=%d, want 7", totalOutbox)
	}
	t.Log("executed linkage_chains=4 audit_records=4 notification_records=3 outbox_records=7")
}

type completeDurableLink struct {
	recordType string
	recordID   string
	subject    string
	record     map[string]any
}

func completeLinkageChain(t *testing.T, context *completeFixtureContext, chain map[string]any, chainIndex int) {
	scope := completeObject(t, chain["scope"])
	scopeType := completeString(t, scope["type"])
	actor := map[string]any{"type": "SERVICE", "id": "auth-service"}
	switch scopeType {
	case "SYSTEM":
		actor = map[string]any{"type": "SYSTEM", "id": "wal-monitor"}
	case "OWNER":
		actor = map[string]any{"type": "USER", "id": "synthetic-owner"}
	case "AUTH_SECURITY":
	default:
		t.Fatalf("unsupported linkage scope %q", scopeType)
	}
	operationField := completeString(t, chain["operation_link_field"])
	if operationField != "request_id" && operationField != "system_operation_id" {
		t.Fatalf("unsupported linkage operation field %q", operationField)
	}
	auditIntent := completeNormalizedObject(t, map[string]any{
		"schema_version": "fit.platform.audit-intent.v1",
		"kind":           chain["kind"],
		"actor":          actor,
		"scope":          scope,
		"trigger":        chain["trigger"],
		"causation_id":   chain["causation_id"],
		"correlation_id": chain["correlation_id"],
		operationField:   chain["operation_id"],
	})
	completeStrictValid(t, context.validator, "AuditIntent", auditIntent)

	auditEvent := completeCopyObject(auditIntent)
	auditEvent["schema_version"] = "fit.platform.audit-event.v1"
	auditEvent["event_id"] = chain["audit_event_id"]
	auditEvent["occurred_at"] = "2026-07-29T10:00:01Z"
	auditEvent = completeNormalizedObject(t, auditEvent)
	auditDigest, err := DurableRecordDigest(auditEvent)
	if err != nil {
		t.Fatalf("audit digest: %v", err)
	}
	auditEvent["payload_digest"] = auditDigest
	completeStrictValid(t, context.validator, "AuditEvent", auditEvent)

	durables := []completeDurableLink{{
		recordType: "audit",
		recordID:   completeString(t, chain["audit_event_id"]),
		subject:    completeString(t, chain["audit_subject"]),
		record:     auditEvent,
	}}

	if chain["notification_id"] != nil {
		details, exists := completeNotificationDetails()[completeString(t, chain["kind"])]
		if !exists {
			t.Fatalf("%s lacks a notification construction handler", chain["name"])
		}
		notificationIntent := completeNormalizedObject(t, map[string]any{
			"schema_version": "fit.platform.notification-intent.v1",
			"kind":           chain["kind"],
			"severity":       details.severity,
			"scope":          scope,
			"message_code":   details.messageCode,
			"message_args":   details.messageArgs,
			"causation_id":   chain["causation_id"],
			"correlation_id": chain["correlation_id"],
		})
		completeStrictValid(t, context.validator, "NotificationIntent", notificationIntent)
		notification := completeCopyObject(notificationIntent)
		notification["schema_version"] = "fit.platform.notification.v1"
		notification["notification_id"] = chain["notification_id"]
		notification["occurred_at"] = "2026-07-29T10:00:01Z"
		notification = completeNormalizedObject(t, notification)
		notificationDigest, err := DurableRecordDigest(notification)
		if err != nil {
			t.Fatalf("notification digest: %v", err)
		}
		notification["payload_digest"] = notificationDigest
		completeStrictValid(t, context.validator, "Notification", notification)
		durables = append(durables, completeDurableLink{
			recordType: "notification",
			recordID:   completeString(t, chain["notification_id"]),
			subject:    completeString(t, chain["notification_subject"]),
			record:     notification,
		})
	}

	outboxes := make([]map[string]any, 0, len(durables))
	events := make([]any, 0, len(durables))
	for recordIndex, durable := range durables {
		aggregateVersion := int64(recordIndex + 1)
		identity := map[string]any{
			"event_id":          durable.recordID,
			"aggregate_type":    chain["aggregate_type"],
			"aggregate_id":      chain["aggregate_id"],
			"aggregate_version": json.Number(strconv.FormatInt(aggregateVersion, 10)),
		}
		if recordIndex > 0 {
			identity["previous_event_id"] = durables[recordIndex-1].recordID
		}
		payload := completeNormalizedObject(t, map[string]any{
			"schema_version": "fit.platform.event-payload.v1",
			"subject":        durable.subject,
			"event_kind":     chain["kind"],
			"scope":          scope,
			"event_identity": identity,
			"data": map[string]any{
				"record_type": durable.recordType,
				"record_id":   durable.recordID,
				"record":      durable.record,
			},
		})
		payloadDigest, err := PayloadDigest(payload)
		if err != nil {
			t.Fatalf("payload digest: %v", err)
		}
		event := completeNormalizedObject(t, map[string]any{
			"schema_version":         "fit.platform.event-envelope.v1",
			"subject":                durable.subject,
			"stream":                 "FIT_PLATFORM_V1",
			"event_id":               durable.recordID,
			"event_kind":             chain["kind"],
			"scope":                  scope,
			"aggregate_type":         chain["aggregate_type"],
			"aggregate_id":           chain["aggregate_id"],
			"aggregate_version":      json.Number(strconv.FormatInt(aggregateVersion, 10)),
			"causation_id":           chain["causation_id"],
			"correlation_id":         chain["correlation_id"],
			"occurred_at":            "2026-07-29T10:00:01Z",
			"payload_schema_version": "fit.platform.event-payload.v1",
			"payload_digest":         payloadDigest,
			"payload":                payload,
		})
		if recordIndex > 0 {
			event["previous_event_id"] = durables[recordIndex-1].recordID
		}
		outboxID := fmt.Sprintf(
			"27000000-0000-4000-8000-%012d",
			chainIndex*2+recordIndex+1,
		)
		outbox := completeNormalizedObject(t, map[string]any{
			"schema_version":    "fit.platform.outbox-record.v1",
			"outbox_id":         outboxID,
			"event":             event,
			"created_at":        "2026-07-29T10:00:01Z",
			"publication_state": "PENDING",
			"retention":         "NEVER_DELETE_IN_PHASE_1",
		})
		completeStrictValid(t, context.validator, "OutboxRecord", outbox)
		outboxes = append(outboxes, outbox)
		events = append(events, event)
	}

	history := completeNormalizedObject(t, map[string]any{
		"schema_version":   "fit.platform.aggregate-event-history.v1",
		"aggregate_type":   chain["aggregate_type"],
		"aggregate_id":     chain["aggregate_id"],
		"starting_version": json.Number("1"),
		"events":           events,
	})
	completeStrictValid(t, context.validator, "AggregateEventHistory", history)
	if int64(len(outboxes)) != completeInt(t, chain["expected_outbox_records"]) {
		t.Errorf("outbox count=%d", len(outboxes))
	}
	for index, outbox := range outboxes {
		event := completeObject(t, outbox["event"])
		payload := completeObject(t, event["payload"])
		data := completeObject(t, payload["data"])
		if completeString(t, data["record_id"]) != durables[index].recordID ||
			CanonicalJSON(data["record"]) != CanonicalJSON(durables[index].record) {
			t.Errorf("outbox %d cannot rebuild its durable record", index)
		}
		if event["causation_id"] != chain["causation_id"] ||
			event["correlation_id"] != chain["correlation_id"] ||
			CanonicalJSON(event["scope"]) != CanonicalJSON(scope) {
			t.Errorf("outbox %d lost scope/causation/correlation", index)
		}
	}

	projection := completeObject(t, chain["recovery_projection"])
	if completeString(t, projection["source"]) != "POSTGRESQL_OUTBOX" ||
		completeString(t, projection["scope_type"]) != scopeType ||
		completeBool(t, projection["phantom_owner"]) {
		t.Error("recovery projection source/scope/phantom-owner drift")
	}
	if !reflect.DeepEqual(
		completeStringArray(t, projection["audit_event_ids"]),
		[]string{completeString(t, chain["audit_event_id"])},
	) {
		t.Error("recovery projection lost audit linkage")
	}
	wantNotifications := []string{}
	if chain["notification_id"] != nil {
		wantNotifications = append(wantNotifications, completeString(t, chain["notification_id"]))
	}
	if !reflect.DeepEqual(completeStringArray(t, projection["notification_ids"]), wantNotifications) {
		t.Error("recovery projection lost notification linkage")
	}
	triggerContext := completeObject(t, chain["trigger_context"])
	if completeBool(t, triggerContext["phantom_owner_allowed"]) {
		t.Error("linkage trigger permits phantom owner")
	}
	resolved, hasResolution := triggerContext["identifier_resolved"]
	if hasResolution && resolved != nil {
		if !completeBool(t, resolved) {
			if _, user := scope["user_id"]; user {
				t.Error("unresolved identifier acquired a user")
			}
			if _, account := scope["trading_account_id"]; account {
				t.Error("unresolved identifier acquired a trading account")
			}
		} else if scopeType == "AUTH_SECURITY" {
			if _, user := scope["user_id"]; !user {
				t.Error("resolved auth-security chain lacks user linkage")
			}
			if _, account := scope["trading_account_id"]; account {
				t.Error("auth-security chain acquired trading-account authority")
			}
		}
	}
	if scopeType == "OWNER" {
		if _, user := scope["user_id"]; !user {
			t.Error("owner chain lacks user")
		}
		if _, account := scope["trading_account_id"]; !account {
			t.Error("owner chain lacks trading account")
		}
	}
}

type completeNotificationDetail struct {
	severity    string
	messageCode string
	messageArgs map[string]any
	scopeType   string
}

func completeNotificationDetails() map[string]completeNotificationDetail {
	return map[string]completeNotificationDetail{
		"LOGIN_SOURCE_LOCKED": {
			severity:    "WARNING",
			messageCode: "security.login.source_locked",
			messageArgs: map[string]any{"lock_seconds": json.Number("900")},
			scopeType:   "AUTH_SECURITY",
		},
		"LOGIN_ACCOUNT_LOCKED": {
			severity:    "WARNING",
			messageCode: "security.login.account_locked",
			messageArgs: map[string]any{"lock_seconds": json.Number("900")},
			scopeType:   "AUTH_SECURITY",
		},
		"DEVICE_ENROLLED": {
			severity:    "INFO",
			messageCode: "security.device.enrolled",
			messageArgs: map[string]any{"device_id": "30000000-0000-4000-8000-000000000001"},
			scopeType:   "AUTH_SECURITY",
		},
		"DEVICE_REPLACED": {
			severity:    "CRITICAL",
			messageCode: "security.device.replaced",
			messageArgs: map[string]any{"device_id": "30000000-0000-4000-8000-000000000001"},
			scopeType:   "AUTH_SECURITY",
		},
		"DEVICE_REVOKED": {
			severity:    "WARNING",
			messageCode: "security.device.revoked",
			messageArgs: map[string]any{"device_id": "30000000-0000-4000-8000-000000000001"},
			scopeType:   "AUTH_SECURITY",
		},
		"SESSION_REVOKED": {
			severity:    "INFO",
			messageCode: "security.session.revoked",
			messageArgs: map[string]any{"session_id": "40000000-0000-4000-8000-000000000001"},
			scopeType:   "AUTH_SECURITY",
		},
		"REFRESH_REUSE_DETECTED": {
			severity:    "CRITICAL",
			messageCode: "security.refresh.reuse_detected",
			messageArgs: map[string]any{"session_id": "40000000-0000-4000-8000-000000000001"},
			scopeType:   "AUTH_SECURITY",
		},
		"WAL_ARCHIVE_INTERRUPTED": {
			severity:    "CRITICAL",
			messageCode: "system.wal.archive_interrupted",
			messageArgs: map[string]any{
				"incident_id":         "25000000-0000-4000-8000-000000000003",
				"archive_age_seconds": json.Number("61"),
			},
			scopeType: "SYSTEM",
		},
	}
}

func completeRequireNames(t *testing.T, cases []any, want []string) {
	t.Helper()
	got := make([]string, len(cases))
	for index, raw := range cases {
		got[index] = completeString(t, completeObject(t, raw)["name"])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture case names/order=%v, want %v", got, want)
	}
}

func completeDecision(t *testing.T, item map[string]any, got string) {
	t.Helper()
	want := completeString(t, item["expected"])
	if got != want {
		t.Errorf("%v: decision=%s, want %s", item["name"], got, want)
	}
}

func completeArgon2Matches(
	t *testing.T,
	password string,
	verifier map[string]any,
	profile map[string]any,
) bool {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("frozen Argon2id authority unavailable: node executable not found: %v", err)
	}
	input, err := json.Marshal(map[string]any{
		"password":     password,
		"salt_hex":     completeString(t, verifier["salt_hex"]),
		"expected_hex": completeString(t, verifier["result_hex"]),
		"parallelism":  completeInt(t, profile["parallelism"]),
		"result_bytes": completeInt(t, profile["result_bytes"]),
		"memory_kib":   completeInt(t, profile["minimum_memory_kib"]),
		"iterations":   completeInt(t, profile["minimum_iterations"]),
	})
	if err != nil {
		t.Fatal(err)
	}
	const program = `
const crypto = require("node:crypto");
const fs = require("node:fs");
if (typeof crypto.argon2Sync !== "function") {
  process.stderr.write("Node crypto.argon2Sync is unavailable");
  process.exit(2);
}
const value = JSON.parse(fs.readFileSync(0, "utf8"));
const observed = crypto.argon2Sync("argon2id", {
  message: Buffer.from(value.password, "utf8"),
  nonce: Buffer.from(value.salt_hex, "hex"),
  parallelism: value.parallelism,
  tagLength: value.result_bytes,
  memory: value.memory_kib,
  passes: value.iterations,
});
process.stdout.write(String(observed.toString("hex") === value.expected_hex));
`
	command := exec.Command(nodePath, "-e", program)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("frozen Argon2id authority failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	switch strings.TrimSpace(string(output)) {
	case "true":
		return true
	case "false":
		return false
	default:
		t.Fatalf("frozen Argon2id authority returned %q", output)
		return false
	}
}

func completeObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("fixture value has type %T, want object", value)
	}
	return object
}

func completeArray(t *testing.T, value any) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("fixture value has type %T, want array", value)
	}
	return array
}

func completeString(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("fixture value has type %T, want string", value)
	}
	return text
}

func completeOptionalString(value any) string {
	text, _ := value.(string)
	return text
}

func completeBool(t *testing.T, value any) bool {
	t.Helper()
	boolean, ok := value.(bool)
	if !ok {
		t.Fatalf("fixture value has type %T, want bool", value)
	}
	return boolean
}

func completeOptionalBool(value any, fallback bool) bool {
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return fallback
}

func completeFloat(t *testing.T, value any) float64 {
	t.Helper()
	switch number := value.(type) {
	case float64:
		return number
	case json.Number:
		result, err := strconv.ParseFloat(string(number), 64)
		if err != nil {
			t.Fatalf("invalid fixture number %q: %v", number, err)
		}
		return result
	case int:
		return float64(number)
	case int64:
		return float64(number)
	default:
		t.Fatalf("fixture value has type %T, want number", value)
		return 0
	}
}

func completeInt(t *testing.T, value any) int64 {
	t.Helper()
	number := completeFloat(t, value)
	integer := int64(number)
	if float64(integer) != number {
		t.Fatalf("fixture number %v is not an integer", value)
	}
	return integer
}

func completeTime(t *testing.T, value any) int64 {
	t.Helper()
	result, err := scenarioMilliseconds(completeString(t, value))
	if err != nil {
		t.Fatalf("invalid fixture time %v: %v", value, err)
	}
	return result
}

func completeOptionalTime(t *testing.T, value any) int64 {
	t.Helper()
	if value == nil {
		return 0
	}
	return completeTime(t, value)
}

func completeOptionalStringPointer(value any) *string {
	if value == nil {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

func completeStringArray(t *testing.T, value any) []string {
	t.Helper()
	raw := completeArray(t, value)
	result := make([]string, len(raw))
	for index, item := range raw {
		result[index] = completeString(t, item)
	}
	return result
}

func completeNormalizedObject(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := ParseStrictJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return completeObject(t, normalized)
}

func completeStrictValid(t *testing.T, validator *Validator, schema string, value map[string]any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := validator.DecodeTyped(schema, raw)
	if err != nil {
		t.Fatalf("%s strict decode failed: %v", schema, err)
	}
	if typed.SchemaName() != schema {
		t.Fatalf("%s decoded as %s", schema, typed.SchemaName())
	}
}

func completeStrictInvalid(t *testing.T, validator *Validator, schema string, value map[string]any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.DecodeTyped(schema, raw); err == nil {
		t.Errorf("%s accepted generated invalid value", schema)
	}
}

func completeCopyObject(value map[string]any) map[string]any {
	copy := make(map[string]any, len(value)+4)
	for key, item := range value {
		copy[key] = item
	}
	return copy
}
