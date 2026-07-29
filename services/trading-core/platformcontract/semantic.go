package platformcontract

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

func (v *Validator) semantic(name string, value any) error {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	switch name {
	case "UntrustedMutationInput", "ConfirmationConsumeMutationInput":
		if containsAuthority(m, authorityNames(v, "UntrustedJsonObject")) {
			return fmt.Errorf("platformcontract: untrusted authority field")
		}
	case "ModelToolMutationInput":
		if containsAuthority(m, append(authorityNames(v, "UntrustedJsonObject"), "confirmation_id", "confirmation_hash")) {
			return fmt.Errorf("platformcontract: model authority field")
		}
	case "RequestEnvelope":
		d, e := RequestDigest(m)
		if e != nil || m["canonical_request_digest"] != d {
			return fmt.Errorf("platformcontract: request digest mismatch")
		}
	case "EnrollmentChallengeState":
		return enrollmentState(m)
	case "RefreshTokenRecord":
		return refreshToken(m)
	case "RefreshFamilyState":
		return refreshFamily(m)
	case "WebSocketReauthRequired":
		return websocketDeadline(m)
	case "EventPayload":
		return v.eventPayload(m)
	case "EventEnvelope":
		return v.eventEnvelope(m)
	case "OutboxRecord":
		return outboxCausality(m)
	case "AggregateEventHistory":
		return v.aggregateHistory(m)
	case "AuditEvent", "Notification":
		d, e := DurableRecordDigest(m)
		if e != nil || m["payload_digest"] != d {
			return fmt.Errorf("platformcontract: durable digest mismatch")
		}
	case "RecoveryEvidence":
		return recoveryConsistency(m)
	}
	return nil
}
func authorityNames(v *Validator, name string) []string {
	d := v.definition(name)
	pn := schemaMap(d["propertyNames"])
	not := schemaMap(pn["not"])
	return strSlice(not["enum"])
}
func authorityAlias(s string) string { return unicodeAuthorityAlias(s) }
func containsAuthority(x any, names []string) bool {
	denied := map[string]bool{}
	for _, n := range names {
		denied[authorityAlias(n)] = true
	}
	var walk func(any) bool
	walk = func(x any) bool {
		switch z := x.(type) {
		case []any:
			for _, a := range z {
				if walk(a) {
					return true
				}
			}
		case map[string]any:
			for k, a := range z {
				if denied[authorityAlias(k)] || walk(a) {
					return true
				}
			}
		}
		return false
	}
	return walk(x)
}
func millis(x any) (int64, bool) {
	s, ok := x.(string)
	if !ok || !validTimestamp(s) {
		return 0, false
	}
	layout := "2006-01-02T15:04:05Z07:00"
	if strings.Contains(s, ".") {
		layout = "2006-01-02T15:04:05.000Z07:00"
	}
	t, e := time.Parse(layout, s)
	return t.UnixMilli(), e == nil
}
func pair(m map[string]any, a, b string) (int64, int64, bool) {
	x, ok := millis(m[a])
	if !ok {
		return 0, 0, false
	}
	y, ok := millis(m[b])
	return x, y, ok
}
func enrollmentState(m map[string]any) error {
	c, _ := m["challenge"].(map[string]any)
	h, _ := c["subject_handle"].(string)
	if fmt.Sprintf("%x", sha256.Sum256([]byte(h))) != m["subject_handle_digest"] {
		return fmt.Errorf("platformcontract: challenge handle binding")
	}
	// The frozen authority binds the wire representation, not merely the
	// represented instant. This prevents a digest/state record from silently
	// changing between the whole-second and millisecond timestamp spellings.
	if m["created_at"] != c["issued_at"] {
		return fmt.Errorf("platformcontract: challenge creation time")
	}
	status, _ := m["status"].(string)
	if status == "CONSUMED" {
		x, ok := millis(m["consumed_at"])
		c, ok2 := millis(m["created_at"])
		if !ok || !ok2 || x < c {
			return fmt.Errorf("platformcontract: consumed time")
		}
	}
	if status == "EXPIRED" {
		x, ok := millis(m["expired_at"])
		e, ok2 := millis(c["expires_at"])
		if !ok || !ok2 || x < e {
			return fmt.Errorf("platformcontract: expired time")
		}
	}
	return nil
}
func refreshToken(m map[string]any) error {
	fc, iss, ok := pair(m, "family_created_at", "issued_at")
	if !ok {
		return fmt.Errorf("platformcontract: refresh time")
	}
	fd, ok := millis(m["family_deadline"])
	if !ok || fd-fc != 2592000000 || iss < fc || iss >= fd {
		return fmt.Errorf("platformcontract: refresh family deadline")
	}
	exp, ok := millis(m["expires_at"])
	want := iss + 604800000
	if fd < want {
		want = fd
	}
	if !ok || exp != want {
		return fmt.Errorf("platformcontract: refresh expiry")
	}
	if r, yes := m["rotated_to_digest"]; yes && r == m["token_digest"] {
		return fmt.Errorf("platformcontract: self rotation")
	}
	if r, yes := m["revoked_at"]; yes {
		x, ok := millis(r)
		if !ok || x < iss || x > fd {
			return fmt.Errorf("platformcontract: revoked time")
		}
	}
	return nil
}
func refreshFamily(m map[string]any) error {
	tokens, _ := m["tokens"].([]any)
	by := map[string]map[string]any{}
	child := map[string]bool{}
	for _, x := range tokens {
		t, _ := x.(map[string]any)
		d, _ := t["token_digest"].(string)
		if by[d] != nil {
			return fmt.Errorf("platformcontract: duplicate token")
		}
		by[d] = t
		for _, k := range []string{"family_id", "session_id", "family_created_at", "family_deadline"} {
			if t[k] != m[k] {
				return fmt.Errorf("platformcontract: lineage scope")
			}
		}
		if m["status"] == "REVOKED" && t["status"] != "REVOKED" {
			return fmt.Errorf("platformcontract: revoked descendant")
		}
	}
	for d, t := range by {
		if n, ok := t["rotated_to_digest"].(string); ok {
			next := by[n]
			if next == nil || child[n] {
				return fmt.Errorf("platformcontract: lineage descendant")
			}
			child[n] = true
			a, _ := millis(t["issued_at"])
			b, _ := millis(next["issued_at"])
			e, _ := millis(t["expires_at"])
			deadline, _ := millis(m["family_deadline"])
			if b < a || b >= e || b >= deadline {
				return fmt.Errorf("platformcontract: lineage ordering")
			}
			seen := map[string]bool{}
			for cur := d; cur != ""; {
				if seen[cur] {
					return fmt.Errorf("platformcontract: lineage cycle")
				}
				seen[cur] = true
				n, _ := by[cur]["rotated_to_digest"].(string)
				cur = n
			}
		}
	}
	roots := 0
	for d := range by {
		if !child[d] {
			roots++
		}
	}
	if roots != 1 {
		return fmt.Errorf("platformcontract: lineage roots")
	}
	if m["status"] == "ACTIVE" {
		active := 0
		for _, t := range by {
			if t["status"] == "ACTIVE" {
				active++
			}
			if t["status"] == "REVOKED" {
				return fmt.Errorf("platformcontract: active family revoked")
			}
		}
		if active != 1 {
			return fmt.Errorf("platformcontract: active lineage")
		}
	}
	return nil
}
func websocketDeadline(m map[string]any) error {
	i, ok := millis(m["issued_at"])
	if !ok {
		return fmt.Errorf("platformcontract: websocket issue")
	}
	c, ok := millis(m["connection_established_at"])
	e, ok2 := millis(m["current_access_expires_at"])
	d, ok3 := millis(m["deadline"])
	if !ok || !ok2 || !ok3 || c >= e {
		return fmt.Errorf("platformcontract: websocket time")
	}
	want := e - 60000
	if c > want {
		want = c
	}
	if i != want || i >= e {
		return fmt.Errorf("platformcontract: websocket issuance")
	}
	want = i + 60000
	if e < want {
		want = e
	}
	if d != want {
		return fmt.Errorf("platformcontract: websocket deadline")
	}
	return nil
}
func (v *Validator) eventPayload(m map[string]any) error {
	data, _ := m["data"].(map[string]any)
	record, _ := data["record"].(map[string]any)
	if record == nil {
		return fmt.Errorf("platformcontract: event record")
	}
	kind, _ := m["event_kind"].(string)
	bindings := v.docs["manifests/nats-permissions-v1.json"]
	matched := false
	for _, x := range slice(bindings["subject_bindings"]) {
		b := schemaMap(x)
		if b["subject"] == m["subject"] && b["event_kind"] == kind {
			scope := schemaMap(m["scope"])
			if b["scope"] == scope["type"] {
				matched = true
				break
			}
		}
	}
	if !matched {
		return fmt.Errorf("platformcontract: event subject binding")
	}
	identity, _ := m["event_identity"].(map[string]any)
	if identity["event_id"] != data["record_id"] || identity["previous_event_id"] == identity["event_id"] {
		return fmt.Errorf("platformcontract: event identity")
	}
	if record["kind"] != kind {
		return fmt.Errorf("platformcontract: event kind")
	}
	if CanonicalJSON(record["scope"]) != CanonicalJSON(m["scope"]) {
		return fmt.Errorf("platformcontract: event scope")
	}
	typ, _ := data["record_type"].(string)
	id := data["record_id"]
	subject, _ := m["subject"].(string)
	switch typ {
	case "audit":
		if strings.Contains(subject, ".notification.") || record["event_id"] != id {
			return fmt.Errorf("platformcontract: audit linkage")
		}
	case "notification":
		if !strings.Contains(subject, ".notification.") || record["notification_id"] != id {
			return fmt.Errorf("platformcontract: notification linkage")
		}
	default:
		return fmt.Errorf("platformcontract: event record type")
	}
	return nil
}
func (v *Validator) eventEnvelope(m map[string]any) error {
	p, _ := m["payload"].(map[string]any)
	if p == nil {
		return fmt.Errorf("platformcontract: payload")
	}
	if p["schema_version"] != "fit.platform.event-payload.v1" || p["schema_version"] != m["payload_schema_version"] {
		return fmt.Errorf("platformcontract: payload schema version")
	}
	d, e := PayloadDigest(p)
	if e != nil || m["payload_digest"] != d {
		return fmt.Errorf("platformcontract: payload digest")
	}
	for _, k := range []string{"subject", "event_kind", "scope"} {
		if CanonicalJSON(p[k]) != CanonicalJSON(m[k]) {
			return fmt.Errorf("platformcontract: payload %s binding", k)
		}
	}
	id, _ := p["event_identity"].(map[string]any)
	for _, k := range []string{"event_id", "aggregate_type", "aggregate_id", "aggregate_version"} {
		if CanonicalJSON(id[k]) != CanonicalJSON(m[k]) {
			return fmt.Errorf("platformcontract: identity binding")
		}
	}
	if _, ok := m["previous_event_id"]; ok {
		if id["previous_event_id"] != m["previous_event_id"] {
			return fmt.Errorf("platformcontract: predecessor")
		}
	} else if _, ok := id["previous_event_id"]; ok {
		return fmt.Errorf("platformcontract: unexpected predecessor")
	}
	if e := v.eventPayload(p); e != nil {
		return e
	}
	data, _ := p["data"].(map[string]any)
	record, _ := data["record"].(map[string]any)
	d, e = DurableRecordDigest(record)
	if e != nil || record["payload_digest"] != d {
		return fmt.Errorf("platformcontract: durable payload digest")
	}
	for _, k := range []string{"causation_id", "correlation_id", "occurred_at"} {
		if record[k] != m[k] {
			return fmt.Errorf("platformcontract: record %s linkage", k)
		}
	}
	return nil
}
func outboxCausality(m map[string]any) error {
	e, _ := m["event"].(map[string]any)
	a, ok := millis(e["occurred_at"])
	b, ok2 := millis(m["created_at"])
	if !ok || !ok2 || a > b {
		return fmt.Errorf("platformcontract: outbox causality")
	}
	if p, yes := m["published_at"]; yes {
		x, ok := millis(p)
		if !ok || b > x {
			return fmt.Errorf("platformcontract: publication order")
		}
	}
	return nil
}
func (v *Validator) aggregateHistory(m map[string]any) error {
	a, _ := m["events"].([]any)
	var prior map[string]any
	seen := map[string]bool{}
	for i, x := range a {
		e, _ := x.(map[string]any)
		if e["aggregate_type"] != m["aggregate_type"] || e["aggregate_id"] != m["aggregate_id"] {
			return fmt.Errorf("platformcontract: aggregate identity")
		}
		ver, ok := numberInt(e["aggregate_version"])
		start, ok2 := numberInt(m["starting_version"])
		if !ok || !ok2 || ver != start+int64(i) || seen[fmt.Sprint(e["event_id"])] {
			return fmt.Errorf("platformcontract: aggregate version")
		}
		seen[fmt.Sprint(e["event_id"])] = true
		if prior == nil {
			if _, exists := e["previous_event_id"]; exists {
				return fmt.Errorf("platformcontract: aggregate initial predecessor")
			}
		} else {
			if e["previous_event_id"] != prior["event_id"] || e["correlation_id"] != prior["correlation_id"] {
				return fmt.Errorf("platformcontract: aggregate linkage")
			}
			x, _ := millis(e["occurred_at"])
			y, _ := millis(prior["occurred_at"])
			if x < y {
				return fmt.Errorf("platformcontract: aggregate time")
			}
		}
		prior = e
	}
	return nil
}
func recoveryConsistency(m map[string]any) error { // complete arithmetic is enforced without accepting operational action.
	last, _ := m["last_committed"].(map[string]any)
	rec, _ := m["last_recovered"].(map[string]any)
	lc, ok := numberInt(last["sequence"])
	lr, ok2 := numberInt(rec["sequence"])
	rpo, _ := m["observed_rpo"].(map[string]any)
	gap, ok3 := numberInt(rpo["sequence_gap"])
	committedAt, ok4 := millis(last["committed_at"])
	recoveredAt, ok5 := millis(rec["committed_at"])
	failure, _ := m["failure"].(map[string]any)
	injectedAt, ok6 := millis(failure["injected_at"])
	restore, _ := m["restore"].(map[string]any)
	invokedAt, ok7 := millis(restore["invoked_at"])
	result, _ := m["result"].(string)
	terminalKey, terminalID := "verification_stopped_at", "stop_event_id"
	if result == "PASS" {
		terminalKey, terminalID = "verification_completed_at", "completion_event_id"
	}
	terminalAt, ok8 := millis(restore[terminalKey])
	timeGap, ok9 := numberInt(rpo["time_gap_ms"])
	if !ok || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 || !ok9 || lc != 6000 || lr > lc || gap != lc-lr || recoveredAt > committedAt || committedAt != injectedAt || injectedAt > invokedAt || invokedAt > terminalAt || timeGap != committedAt-recoveredAt || restore["start_event_id"] == restore[terminalID] {
		return fmt.Errorf("platformcontract: recovery sequence")
	}
	if result == "PASS" {
		rto, valid := numberInt(m["observed_rto_ms"])
		if !valid || rto != terminalAt-invokedAt {
			return fmt.Errorf("platformcontract: recovery rto")
		}
	} else if _, exists := m["observed_rto_ms"]; exists {
		return fmt.Errorf("platformcontract: fail closed rto")
	}
	components, _ := m["components"].([]any)
	names := map[string]bool{}
	for _, x := range components {
		names[fmt.Sprint(schemaMap(x)["name"])] = true
	}
	if len(names) != 2 || !names["postgresql"] || !names["recovery-tool"] {
		return fmt.Errorf("platformcontract: recovery components")
	}
	gates, _ := m["verification_gates"].([]any)
	hasFail := false
	for _, x := range gates {
		status := schemaMap(x)["status"]
		if result == "PASS" && status != "PASS" {
			return fmt.Errorf("platformcontract: recovery gate")
		}
		if status == "FAIL" {
			hasFail = true
		}
	}
	if result == "FAIL_CLOSED" && (!hasFail || fmt.Sprint(restore["failure_reason_code"]) == "") {
		return fmt.Errorf("platformcontract: fail closed gate")
	}
	for _, kind := range []string{"BACKUP_CORRUPTION", "MISSING_WAL", "INCOMPATIBLE_MIGRATION", "PARTIAL_RESTORE", "MISSING_CREDENTIAL", "EXTERNAL_EGRESS"} {
		if failure["kind"] == kind && result != "FAIL_CLOSED" {
			return fmt.Errorf("platformcontract: recovery failure result")
		}
	}
	return nil
}
