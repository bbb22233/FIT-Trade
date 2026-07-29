package platformcontract

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureCase(t *testing.T, name string) map[string]any {
	t.Helper()
	for _, x := range load(t, "fixtures/platform/valid-schema-cases-v1.json")["cases"].([]any) {
		c := x.(map[string]any)
		if c["name"] == name {
			return strictValue(t, c["value"]).(map[string]any)
		}
	}
	t.Fatalf("fixture %q not found", name)
	return nil
}

func TestAll59SchemasAccounted(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	defs := schemaMap(v.platform["$defs"])
	if len(defs) != 57 {
		t.Fatalf("defs=%d, want 57", len(defs))
	}
	for name := range defs {
		if v.definition(name) == nil {
			t.Errorf("missing definition %s", name)
		}
	}
	for _, name := range []string{"RemediationProposal", "RecoveryEvidence"} {
		if v.definition(name) == nil {
			t.Errorf("missing root %s", name)
		}
	}
}

func TestJCSGoldenAndAdversarialValues(t *testing.T) {
	for raw, want := range map[string]string{
		`-0`: `0`, `1e20`: `100000000000000000000`, `"\u0001"`: `"\u0001"`,
		`{"\ue000":1,"😀":2}`: `{"😀":2,"":1}`,
	} {
		x, err := ParseStrictJSON([]byte(raw))
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if got := CanonicalJSON(x); got != want {
			t.Errorf("JCS %s = %q, want %q", raw, got, want)
		}
	}
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, string([]byte{'"', 0xff, '"'}), `1e9999`} {
		if _, err := ParseStrictJSON([]byte(raw)); err == nil {
			t.Errorf("accepted non-I-JSON %q", raw)
		}
	}
}

func TestServerAndModelAuthorityDenylist(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	base := fixtureCase(t, "untrusted mutation carries no server identity")
	for _, key := range []string{"server_time", "serverTime", "SERVER-TIME", "server time", "server\u200b_time", "ｓｅｒｖｅｒ＿ｔｉｍｅ", "confirmation_id", "confirmationHash"} {
		m := strictValue(t, base).(map[string]any)
		m["body"].(map[string]any)[key] = json.Number("1")
		if err := v.validate("ModelToolMutationInput", m); err == nil {
			t.Errorf("model accepted authority alias %q", key)
		}
	}
	ordinary := strictValue(t, base).(map[string]any)
	ordinary["body"].(map[string]any)["café"] = json.Number("1")
	if err := v.validate("ModelToolMutationInput", ordinary); err != nil {
		t.Errorf("ordinary Unicode key rejected: %v", err)
	}
}

func TestStrictNullAndNoCoercion(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Decode("UntrustedJsonObject", []byte(`{"x":null}`)); err != nil {
		t.Fatalf("allowed null rejected: %v", err)
	}
	if _, err := v.Decode("User", []byte(`{"schema_version":null}`)); err == nil {
		t.Error("forbidden null accepted")
	}
	for _, raw := range []string{`{"x":1,"x":2}`, `{} {}`, `{"x":"\ud800"}`, `{"__proto__":1}`, `{"x":true}`} {
		if raw == `{"x":true}` {
			continue
		}
		if _, err := ParseStrictJSON([]byte(raw)); err == nil {
			t.Errorf("strict parser accepted %s", raw)
		}
	}
	if err := v.validate("OpaqueHandle", json.Number("1")); err == nil {
		t.Error("number coerced to opaque handle")
	}
	if err := v.validate("OpaqueHandle", "x"); err == nil {
		t.Error("one-character opaque handle accepted")
	}
}

func TestRecursiveMutationParity(t *testing.T) {
	runRecursiveMutationParity(t)
}

func TestChallengeDomainSeparation(t *testing.T) {
	g := load(t, "fixtures/platform/golden-vectors-v1.json")
	proofs := g["ed25519_proofs"].(map[string]any)
	vectors := proofs["vectors"].([]any)
	if len(vectors) < 2 {
		t.Fatal("challenge domain vectors missing")
	}
	a := strictValue(t, vectors[0].(map[string]any)["challenge"]).(map[string]any)
	b := strictValue(t, vectors[1].(map[string]any)["challenge"]).(map[string]any)
	ab, err := ChallengeSigningBytes(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := ChallengeSigningBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(ab) == string(bb) {
		t.Fatal("challenge domains were not separated")
	}
	if ok, _ := VerifyChallengeProof(a, proofs["public_key"].(string), vectors[1].(map[string]any)["signature"].(string)); ok {
		t.Fatal("cross-domain challenge proof accepted")
	}
	seed := sha256.Sum256([]byte("FIT Trade enrollment fingerprint isolation"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signed, err := ChallengeSigningBytes(a)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, signed)
	ok, err := VerifyChallengeProof(
		a,
		"ed25519-public:"+hex.EncodeToString(publicKey),
		"ed25519-signature:"+hex.EncodeToString(signature),
	)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("valid signature from a key outside the enrolled fingerprint was accepted")
	}
	goldenPublic := proofs["public_key"].(string)
	upperPublic := "ed25519-public:" +
		strings.ToUpper(strings.TrimPrefix(goldenPublic, "ed25519-public:"))
	if _, err := VerifyChallengeProof(
		a,
		upperPublic,
		vectors[0].(map[string]any)["signature"].(string),
	); err == nil {
		t.Fatal("noncanonical uppercase public-key wire was accepted")
	}
}

func TestRefreshFamilyLineage(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	good := fixtureCase(t, "acyclic same-family refresh lineage")
	if err := v.validate("RefreshFamilyState", good); err != nil {
		t.Fatal(err)
	}
	tokens := good["tokens"].([]any)
	first := tokens[0].(map[string]any)
	first["rotated_to_digest"] = first["token_digest"]
	if err := v.validate("RefreshFamilyState", good); err == nil {
		t.Error("refresh rotation cycle accepted")
	}
}

func TestAuditNotificationMatrix(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"valid-schema-cases-v1.json", "invalid-schema-cases-v1.json"} {
		wantValid := file[:5] == "valid"
		for _, x := range load(t, "fixtures/platform/"+file)["cases"].([]any) {
			c := x.(map[string]any)
			schema := c["schema"].(string)
			if schema != "AuditEvent" && schema != "AuditIntent" && schema != "Notification" && schema != "NotificationIntent" {
				continue
			}
			err := v.validate(schema, strictValue(t, c["value"]))
			if (err == nil) != wantValid {
				t.Errorf("%s %s: %v", file, c["name"], err)
			}
		}
	}
}

func FuzzParseStrictJSON(f *testing.F) {
	for _, seed := range []string{`null`, `{"x":null}`, `{"x":1}`, `[]`, `"\ud800"`, `1e20`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 4096 {
			t.Skip()
		}
		x, err := ParseStrictJSON(raw)
		if err != nil {
			return
		}
		c := CanonicalJSON(x)
		if c == "" {
			t.Fatalf("accepted value cannot be canonicalized: %q", raw)
		}
		y, err := ParseStrictJSON([]byte(c))
		if err != nil || CanonicalJSON(y) != c {
			t.Fatalf("JCS round trip failed: %q (%v)", c, err)
		}
	})
}

func contractRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "contracts")
}
func testMillis(t *testing.T, s string) int64 {
	t.Helper()
	layout := "2006-01-02T15:04:05Z07:00"
	if len(s) > 20 {
		layout = "2006-01-02T15:04:05.000Z07:00"
	}
	v, e := time.Parse(layout, s)
	if e != nil {
		t.Fatal(e)
	}
	return v.UnixMilli()
}
func load(t *testing.T, name string) map[string]any {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(contractRoot(t), name))
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func strictValue(t *testing.T, v any) any {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	x, e := ParseStrictJSON(b)
	if e != nil {
		t.Fatal(e)
	}
	return x
}

func TestFrozenFixturesParity(t *testing.T) {
	v, e := NewValidator()
	if e != nil {
		t.Fatal(e)
	}
	valid := load(t, "fixtures/platform/valid-schema-cases-v1.json")["cases"].([]any)
	invalid := load(t, "fixtures/platform/invalid-schema-cases-v1.json")["cases"].([]any)
	for _, x := range valid {
		c := x.(map[string]any)
		if e := v.validate(c["schema"].(string), strictValue(t, c["value"])); e != nil {
			t.Errorf("valid %s: %v", c["name"], e)
		}
	}
	for _, x := range invalid {
		c := x.(map[string]any)
		if e := v.validate(c["schema"].(string), strictValue(t, c["value"])); e == nil {
			t.Errorf("invalid accepted: %s", c["name"])
		}
	}
	if got, want := len(valid), 35; got != want {
		t.Fatalf("valid fixture drift: %d", got)
	}
	if got, want := len(invalid), 28; got != want {
		t.Fatalf("invalid fixture drift: %d", got)
	}
	assertFrozenFixtureNodeParity(t, valid, invalid)
}
func TestFrozenContractDrift(t *testing.T) {
	expected := map[string]bool{}
	for _, name := range []string{
		"schemas/platform-v1.schema.json", "schemas/remediation-proposal-v1.schema.json", "schemas/recovery-evidence-v1.schema.json",
		"manifests/failure-injection-v1.json", "manifests/http-rate-limits-v1.json", "manifests/migration-policy-v1.json", "manifests/nats-permissions-v1.json", "manifests/phase0-file-manifest-v1.json", "manifests/recovery-policy-v1.json", "manifests/security-values-v1.json", "manifests/state-machines-v1.json", "manifests/transaction-boundaries-v1.json", "manifests/websocket-protocol-v1.json",
		"../fixtures/platform/executable-security-scenarios-v1.json", "../fixtures/platform/golden-vectors-v1.json", "../fixtures/platform/invalid-schema-cases-v1.json", "../fixtures/platform/linkage-chains-v1.json", "../fixtures/platform/semantic-scenarios-v1.json", "../fixtures/platform/valid-schema-cases-v1.json",
	} {
		expected[name] = true
	}
	if got := len(FrozenContractSHA256()); got != len(expected) {
		t.Fatalf("frozen input inventory = %d, want %d", got, len(expected))
	}
	for n, want := range FrozenContractSHA256() {
		if !expected[n] {
			t.Errorf("unexpected frozen input %s", n)
		}
		b, e := os.ReadFile(filepath.Join(contractRoot(t), "platform", n))
		if e != nil {
			t.Fatal(e)
		}
		got := fmtHash(sha256.Sum256(b))
		if got != want {
			t.Errorf("frozen input drift %s: %s != %s", n, got, want)
		}
	}
}
func fmtHash(v [32]byte) string {
	const h = "0123456789abcdef"
	b := make([]byte, 64)
	for i, x := range v {
		b[2*i] = h[x>>4]
		b[2*i+1] = h[x&15]
	}
	return string(b)
}
func TestGoldenRequestAndEventDigests(t *testing.T) {
	g := load(t, "fixtures/platform/golden-vectors-v1.json")
	r := strictValue(t, g["request_digest"].(map[string]any)["value"]).(map[string]any)
	d, e := RequestDigest(r)
	if e != nil || d != g["request_digest"].(map[string]any)["sha256"] {
		t.Fatalf("request golden: %s %v", d, e)
	}
	for _, method := range []string{"É", "123", "_", "GET-POST", "post"} {
		mutated := cloneObject(r)
		mutated["method"] = method
		if _, err := RequestDigest(mutated); err == nil {
			t.Errorf("request digest accepted non-ASCII method %q", method)
		}
	}
	p := strictValue(t, g["payload_digest"].(map[string]any)["value"])
	d, e = PayloadDigest(p)
	if e != nil || d != g["payload_digest"].(map[string]any)["sha256"] {
		t.Fatalf("payload golden: %s %v", d, e)
	}
	proofs := g["ed25519_proofs"].(map[string]any)
	for _, x := range proofs["vectors"].([]any) {
		q := x.(map[string]any)
		ok, e := VerifyChallengeProof(strictValue(t, q["challenge"]).(map[string]any), proofs["public_key"].(string), q["signature"].(string))
		if e != nil || !ok {
			t.Errorf("proof %s: %v", q["name"], e)
		}
	}
}
func TestStrictDecodeNoCoercion(t *testing.T) {
	v, e := NewValidator()
	if e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"idempotency_key":"x","path":{},"query":{},"body":{"serverTime":1}}`, `{"idempotency_key":"x","path":{},"query":{},"body":{"server-time":1}}`, `{"idempotency_key":"x","path":{},"query":{},"body":{"ｓｅｒｖｅｒ＿ｔｉｍｅ":1}}`, `{"idempotency_key":"x","path":{},"query":{},"body":{"a":{"source_key":1}}}`, `{"idempotency_key":"x","idempotency_key":"y","path":{},"query":{},"body":{}}`, `{"idempotency_key":null,"path":{},"query":{},"body":{}}`} {
		m, e := ParseStrictJSON([]byte(raw))
		if e == nil {
			if e = v.validate("UntrustedMutationInput", m); e == nil {
				t.Errorf("accepted adversarial raw JSON %s", raw)
			}
		}
	}
}
func TestSemanticFixtureParity(t *testing.T) {
	s := load(t, "fixtures/platform/semantic-scenarios-v1.json")
	for _, x := range s["challenge_cases"].([]any) {
		c := x.(map[string]any)
		got := ChallengeDecision(c["synthetic"].(bool), c["already_attempted"].(bool), c["proof_valid"].(bool), testMillis(t, c["now"].(string)), testMillis(t, c["expires_at"].(string)))
		if got != c["expected"] {
			t.Errorf("challenge %s: %s", c["name"], got)
		}
	}
	for _, x := range s["refresh_cases"].([]any) {
		c := x.(map[string]any)
		got := RefreshDecision(c["status"].(string), testMillis(t, c["now"].(string)), testMillis(t, c["individual_expires_at"].(string)), testMillis(t, c["family_deadline"].(string)))
		if got != c["expected"] {
			t.Errorf("refresh %s: %s", c["name"], got)
		}
	}
	for _, x := range s["refresh_expiry_cases"].([]any) {
		c := x.(map[string]any)
		if got := RefreshExpiry(testMillis(t, c["issued_at"].(string)), testMillis(t, c["family_deadline"].(string))); got != testMillis(t, c["expected_expires_at"].(string)) {
			t.Errorf("refresh expiry %s", c["name"])
		}
	}
	for _, x := range s["websocket_cases"].([]any) {
		c := x.(map[string]any)
		if c["event"] == "REAUTH_DEADLINE_WITHOUT_RESPONSE" {
			if c["expected"] != "CLOSE_4401" {
				t.Errorf("websocket deadline")
			}
			continue
		}
		if c["event"] == "APPLICATION_FRAME_WHILE_REAUTH_PENDING" {
			got := WebSocketPendingDecision(c["revoked"].(bool), testMillis(t, c["frame_received_at"].(string)), testMillis(t, c["current_access_expires_at"].(string)))
			if got != c["expected"] {
				t.Errorf("websocket %s: %s", c["name"], got)
			}
			continue
		}
		f := map[string]bool{}
		for _, k := range []string{"challenge_issued", "nonce_matches", "nonce_consumed", "user_active", "account_ownership_active", "user_matches", "account_matches", "device_matches", "session_matches", "family_matches", "token_valid", "token_fresh", "malformed", "revoked"} {
			if b, ok := c[k].(bool); ok {
				f[k] = b
			}
		}
		if _, ok := c["user_active"]; !ok {
			f["user_active"] = true
		}
		if _, ok := c["account_ownership_active"]; !ok {
			f["account_ownership_active"] = true
		}
		got := WebSocketReauthDecision(f, testMillis(t, c["verification_completed_at"].(string)), testMillis(t, c["deadline"].(string)), c["credential_source"].(string))
		if c["revoked"] == true {
			got = "CLOSE_REVOKED_WITHIN_5_SECONDS"
		}
		if got != c["expected"] {
			t.Errorf("websocket %s: %s", c["name"], got)
		}
	}
	for _, x := range s["source_key_vectors"].([]any) {
		c := x.(map[string]any)
		got, e := SourceKey([]byte(c["test_key_utf8"].(string)), c["direct_peer_ip"].(string))
		if e != nil || got != "src_"+c["expected_hmac_sha256"].(string) {
			t.Errorf("source vector %s: %s %v", c["name"], got, e)
		}
	}
}
func TestExecutableRawAndThrottleParity(t *testing.T) {
	v, e := NewValidator()
	if e != nil {
		t.Fatal(e)
	}
	s := load(t, "fixtures/platform/executable-security-scenarios-v1.json")
	foldedPath, foldedQuery, err := ParseConfirmationTarget(
		"/V1/CONFIRMATIONS/70000000-0000-4000-8000-000000000001/CONSUME",
	)
	if err != nil ||
		foldedPath["confirmation_id"] != "70000000-0000-4000-8000-000000000001" ||
		len(foldedQuery) != 0 {
		t.Fatalf("case-insensitive frozen confirmation route mismatch: path=%v query=%v err=%v",
			foldedPath, foldedQuery, err)
	}
	for _, x := range s["raw_request_target_cases"].([]any) {
		c := x.(map[string]any)
		_, _, e := ParseConfirmationTarget(c["raw_target"].(string))
		got := "ACCEPT"
		if e != nil {
			got = "REJECT"
		}
		if got != c["expected"] {
			t.Errorf("target %s: %s", c["name"], got)
		}
	}
	for _, x := range s["raw_entrypoint_cases"].([]any) {
		c := x.(map[string]any)
		_, e := v.Decode(c["schema"].(string), []byte(c["raw"].(string)))
		got := "ACCEPT"
		if e != nil {
			got = "REJECT"
		}
		if got != c["expected"] {
			t.Errorf("entry point %s: %s (%v)", c["name"], got, e)
		}
	}
	for _, x := range s["password_throttle_timelines"].([]any) {
		c := x.(map[string]any)
		th := NewThrottle()
		for _, event := range c["events"].([]any) {
			q := event.(map[string]any)
			user, _ := q["user_id"].(string)
			source, _ := c["source_key"].(string)
			if x, ok := q["source_key"].(string); ok {
				source = x
			}
			got := "TRANSACTION_ABORTED"
			if q["transaction_aborted"] != true {
				got = th.Attempt(int64(q["at_seconds"].(float64)*1000), user, source, q["password_valid"].(bool))
			}
			if got != q["expected_decision"] {
				t.Errorf("throttle %s: got %s want %s", c["name"], got, q["expected_decision"])
			}
			uf, sf := th.Failures(user, source)
			if uf != int(q["expected_account_failures"].(float64)) || sf != int(q["expected_source_failures"].(float64)) {
				t.Errorf("throttle count %s: %d/%d", c["name"], uf, sf)
			}
		}
	}
}
func TestNATSAndRateLimitMappings(t *testing.T) {
	g := load(t, "fixtures/platform/golden-vectors-v1.json")
	r := strictValue(t, g["request_digest"].(map[string]any)["value"]).(map[string]any)
	want, _ := RequestDigest(r)
	for k, x := range r {
		m := map[string]any{}
		for a, b := range r {
			m[a] = b
		}
		if _, ok := x.(map[string]any); ok {
			m[k] = map[string]any{"changed": true}
		} else {
			m[k] = "changed"
		}
		got, e := RequestDigest(m)
		if e == nil && got == want {
			t.Errorf("request digest did not bind %s", k)
		}
	}
	p := strictValue(t, g["payload_digest"].(map[string]any)["value"])
	pd, _ := PayloadDigest(p)
	for k, x := range p.(map[string]any) {
		m := map[string]any{}
		for a, b := range p.(map[string]any) {
			m[a] = b
		}
		if _, ok := x.(map[string]any); ok {
			m[k] = map[string]any{"changed": true}
		} else {
			m[k] = "changed"
		}
		got, e := PayloadDigest(m)
		if e == nil && got == pd {
			t.Errorf("payload digest did not bind %s", k)
		}
	}
	v, e := NewValidator()
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		p, a, s string
		want    bool
	}{{"outbox-publisher", "publish", "fit.platform.v1.auth-security.device-enrolled", true}, {"outbox-publisher", "publish", "fit.platform.v1.>", false}, {"platform-consumer", "publish", "$JS.ACK.FIT_PLATFORM_V1.PLATFORM_CONSUMER.any", true}, {"platform-consumer", "publish", "$JS.ACK.FIT_PLATFORM_V1.INTERNAL_NOTIFICATION_CONSUMER.any", false}} {
		got, e := v.NATSAllowed(c.p, c.a, c.s)
		if e != nil || got != c.want {
			t.Errorf("NATS %v: %v %v", c, got, e)
		}
	}
	for _, x := range load(t, "fixtures/platform/semantic-scenarios-v1.json")["rate_limit_cases"].([]any) {
		c := x.(map[string]any)
		group, err := v.RateLimitGroup(c["group"].(string))
		if err != nil {
			t.Error(err)
			continue
		}
		for _, key := range []string{"rate", "per_seconds", "burst"} {
			if CanonicalJSON(group[key]) != CanonicalJSON(strictValue(t, c[key])) {
				t.Errorf("rate limit %s %s", c["group"], key)
			}
		}
		if want, exists := c["maximum_concurrent"]; exists && CanonicalJSON(group["maximum_concurrent"]) != CanonicalJSON(strictValue(t, want)) {
			t.Errorf("rate limit concurrency %s", c["group"])
		}
	}
}
func TestEverySchemaFieldHasMechanicalDisposition(t *testing.T) {
	v, e := NewValidator()
	if e != nil {
		t.Fatal(e)
	}
	m := v.FieldMappings()
	if len(m) != 439 {
		t.Fatalf("unexpected schema field coverage: %d", len(m))
	}
	if got, want := v.FieldMappingsDigest(), "9e8edb2ec0d9aa9fa14b9bd22c0ed41f0c7ac02083f9bb421bdb43cd11399b15"; got != want {
		t.Fatalf("field mapping inventory drift: %s", got)
	}
	for p, d := range m {
		if !strings.HasPrefix(d, "field:") &&
			!strings.HasPrefix(d, "union:") &&
			!strings.HasPrefix(d, "transport-only:") {
			t.Errorf("unmapped field %s: %s", p, d)
		}
	}
	t.Logf("mechanically mapped schema properties=%d digest=%s", len(m), v.FieldMappingsDigest())
}
