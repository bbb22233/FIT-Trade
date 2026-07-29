package domain

import (
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const contractsRoot = "../../../contracts"

func readFixture(t *testing.T, relative string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(contractsRoot, relative))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ticketFromFixture(t *testing.T) ConfirmationTicket {
	t.Helper()
	var fixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/confirmation-ticket-open.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	var ticket ConfirmationTicket
	if err := StrictDecode(fixture.Value, &ticket); err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestSharedFixtures(t *testing.T) {
	now := time.Date(2026, 7, 29, 4, 5, 0, 0, time.UTC)
	for _, set := range []struct {
		name      string
		wantValid bool
	}{{"valid", true}, {"invalid", false}} {
		dir := filepath.Join(contractsRoot, "fixtures", set.name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Fatalf("%s fixture directory is empty", set.name)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				t.Fatalf("unexpected fixture directory entry %s", filepath.Join(dir, entry.Name()))
			}
			t.Run(set.name+"/"+entry.Name(), func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				err = ValidateFixture(data, now)
				if set.wantValid && err != nil {
					t.Fatalf("valid fixture rejected: %v", err)
				}
				if !set.wantValid && err == nil {
					t.Fatal("invalid fixture accepted")
				}
			})
		}
	}
}

func TestModelProposalProposeTradeValidatesNestedIntent(t *testing.T) {
	var intentFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/trade-intent-open.json"), &intentFixture); err != nil {
		t.Fatal(err)
	}
	proposal := map[string]any{
		"schema_version": SchemaVersion,
		"proposal_id":    "018f4f1c-9f9a-7c21-a06f-2f4d3d25d701",
		"model_version":  "model-hermes-v1",
		"decision":       "PROPOSE_TRADE",
		"created_at":     "2026-07-29T04:00:00Z",
	}
	var intent any
	if err := json.Unmarshal(intentFixture.Value, &intent); err != nil {
		t.Fatal(err)
	}
	proposal["intent"] = intent
	fixture := marshalFixture(t, "ModelProposal", proposal)
	if err := ValidateFixture(fixture, time.Time{}); err != nil {
		t.Fatalf("valid PROPOSE_TRADE rejected: %v", err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"missing intent": func(p map[string]any) { delete(p, "intent") },
		"unknown nested field": func(p map[string]any) {
			p["intent"].(map[string]any)["unexpected"] = true
		},
		"invalid nested version": func(p map[string]any) {
			p["intent"].(map[string]any)["model_version"] = "hermes-v1"
		},
		"invalid nested stop": func(p map[string]any) {
			p["intent"].(map[string]any)["stop"].(map[string]any)["reduce_only"] = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			var copy map[string]any
			raw, _ := json.Marshal(proposal)
			if err := json.Unmarshal(raw, &copy); err != nil {
				t.Fatal(err)
			}
			mutate(copy)
			if err := ValidateFixture(marshalFixture(t, "ModelProposal", copy), time.Time{}); err == nil {
				t.Fatal("invalid nested intent accepted")
			}
		})
	}
}

func TestSharedFixtureFieldBoundaries(t *testing.T) {
	base := ticketFromFixture(t).Intent
	tests := map[string]func(*TradeIntent){
		"bad risk version":     func(v *TradeIntent) { v.RiskPolicyVersion = "risk-UPPER" },
		"bad strategy version": func(v *TradeIntent) { v.StrategyVersion = "strategy-" },
		"bad model version":    func(v *TradeIntent) { v.ModelVersion = "model-a!" },
		"bad created_at":       func(v *TradeIntent) { v.CreatedAt = "2026-07-29" },
		"reducing bad stop type": func(v *TradeIntent) {
			v.PositionEffect = "REDUCE"
			v.Stop.Type = "LIMIT"
		},
		"reducing non-reduce-only stop": func(v *TradeIntent) {
			v.PositionEffect = "CLOSE"
			v.Stop.ReduceOnly = false
		},
	}
	for name, mutate := range tests {
		t.Run("TradeIntent/"+name, func(t *testing.T) {
			value := base
			stop := *base.Stop
			value.Stop = &stop
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("schema-invalid intent accepted")
			}
		})
	}

	var operationFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/operation-awaiting-confirmation.json"), &operationFixture); err != nil {
		t.Fatal(err)
	}
	var operation map[string]any
	if err := json.Unmarshal(operationFixture.Value, &operation); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing state_version": func(v map[string]any) { delete(v, "state_version") },
		"bad created_at":        func(v map[string]any) { v["created_at"] = "not-a-time" },
		"bad updated_at":        func(v map[string]any) { v["updated_at"] = "not-a-time" },
		"long rejection_code":   func(v map[string]any) { v["rejection_code"] = strings.Repeat("x", 97) },
	} {
		t.Run("Operation/"+name, func(t *testing.T) {
			copy := cloneMap(t, operation)
			mutate(copy)
			if err := ValidateFixture(marshalFixture(t, "Operation", copy), time.Time{}); err == nil {
				t.Fatal("schema-invalid operation accepted")
			}
		})
	}

	validProposal := map[string]any{
		"schema_version": SchemaVersion,
		"proposal_id":    "018f4f1c-9f9a-7c21-a06f-2f4d3d25d701",
		"model_version":  "",
		"decision":       "NO_TRADE",
		"created_at":     "2026-07-29T04:00:00Z",
	}
	if err := ValidateFixture(marshalFixture(t, "ModelProposal", validProposal), time.Time{}); err != nil {
		t.Fatalf("schema-valid empty model_version rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing model_version": func(v map[string]any) { delete(v, "model_version") },
		"long model_version":    func(v map[string]any) { v["model_version"] = strings.Repeat("é", 65) },
		"bad created_at":        func(v map[string]any) { v["created_at"] = "not-a-time" },
		"obsolete TRADE":        func(v map[string]any) { v["decision"] = "TRADE" },
	} {
		t.Run("ModelProposal/"+name, func(t *testing.T) {
			copy := cloneMap(t, validProposal)
			mutate(copy)
			if err := ValidateFixture(marshalFixture(t, "ModelProposal", copy), time.Time{}); err == nil {
				t.Fatal("schema-invalid proposal accepted")
			}
		})
	}

	ticket := ticketFromFixture(t)
	ticket.CreatedAt = "not-a-time"
	if err := ticket.Validate(time.Date(2026, 7, 29, 4, 5, 0, 0, time.UTC)); err == nil {
		t.Fatal("ConfirmationTicket with invalid created_at accepted")
	}
}

func TestFixtureOptionalPropertiesRejectExplicitNull(t *testing.T) {
	var intentFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/trade-intent-open.json"), &intentFixture); err != nil {
		t.Fatal(err)
	}
	var intent map[string]any
	if err := json.Unmarshal(intentFixture.Value, &intent); err != nil {
		t.Fatal(err)
	}
	intent["position_effect"] = "REDUCE"
	intent["stop"] = nil
	assertInvalidFixture(t, "TradeIntent", intent)

	var operationFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/operation-awaiting-confirmation.json"), &operationFixture); err != nil {
		t.Fatal(err)
	}
	var operation map[string]any
	if err := json.Unmarshal(operationFixture.Value, &operation); err != nil {
		t.Fatal(err)
	}
	for _, property := range []string{"confirmation_id", "rejection_code"} {
		t.Run("Operation/"+property, func(t *testing.T) {
			copy := cloneMap(t, operation)
			copy[property] = nil
			assertInvalidFixture(t, "Operation", copy)
		})
	}

	proposal := map[string]any{
		"schema_version": SchemaVersion,
		"proposal_id":    "018f4f1c-9f9a-7c21-a06f-2f4d3d25d701",
		"model_version":  "model-hermes-v1",
		"decision":       "PROPOSE_TRADE",
		"intent":         nil,
		"created_at":     "2026-07-29T04:00:00Z",
	}
	assertInvalidFixture(t, "ModelProposal", proposal)
}

func TestFixtureOptionalPropertiesMayBeOmitted(t *testing.T) {
	var intentFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/trade-intent-open.json"), &intentFixture); err != nil {
		t.Fatal(err)
	}
	var intent map[string]any
	if err := json.Unmarshal(intentFixture.Value, &intent); err != nil {
		t.Fatal(err)
	}
	intent["position_effect"] = "REDUCE"
	delete(intent, "stop")
	if err := ValidateFixture(marshalFixture(t, "TradeIntent", intent), time.Time{}); err != nil {
		t.Fatalf("omitted optional stop rejected: %v", err)
	}

	var operationFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/operation-awaiting-confirmation.json"), &operationFixture); err != nil {
		t.Fatal(err)
	}
	var operation map[string]any
	if err := json.Unmarshal(operationFixture.Value, &operation); err != nil {
		t.Fatal(err)
	}
	delete(operation, "confirmation_id")
	delete(operation, "rejection_code")
	if err := ValidateFixture(marshalFixture(t, "Operation", operation), time.Time{}); err != nil {
		t.Fatalf("omitted optional operation properties rejected: %v", err)
	}

	proposal := map[string]any{
		"schema_version": SchemaVersion,
		"proposal_id":    "018f4f1c-9f9a-7c21-a06f-2f4d3d25d701",
		"model_version":  "",
		"decision":       "NO_TRADE",
		"created_at":     "2026-07-29T04:00:00Z",
	}
	if err := ValidateFixture(marshalFixture(t, "ModelProposal", proposal), time.Time{}); err != nil {
		t.Fatalf("omitted optional intent rejected: %v", err)
	}
}

func TestNestedTradeIntentRejectsExplicitNullStop(t *testing.T) {
	var intentFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/trade-intent-open.json"), &intentFixture); err != nil {
		t.Fatal(err)
	}
	var intent map[string]any
	if err := json.Unmarshal(intentFixture.Value, &intent); err != nil {
		t.Fatal(err)
	}
	intent["position_effect"] = "REDUCE"
	intent["stop"] = nil

	proposal := map[string]any{
		"schema_version": SchemaVersion,
		"proposal_id":    "018f4f1c-9f9a-7c21-a06f-2f4d3d25d701",
		"model_version":  "model-hermes-v1",
		"decision":       "PROPOSE_TRADE",
		"intent":         intent,
		"created_at":     "2026-07-29T04:00:00Z",
	}
	assertInvalidFixture(t, "ModelProposal", proposal)

	var ticketFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/confirmation-ticket-open.json"), &ticketFixture); err != nil {
		t.Fatal(err)
	}
	var ticket map[string]any
	if err := json.Unmarshal(ticketFixture.Value, &ticket); err != nil {
		t.Fatal(err)
	}
	ticket["intent"] = intent
	assertInvalidFixture(t, "ConfirmationTicket", ticket)
}

func TestRequiredZeroValuePropertiesTrackPresence(t *testing.T) {
	var operationFixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/operation-awaiting-confirmation.json"), &operationFixture); err != nil {
		t.Fatal(err)
	}
	var operation map[string]any
	if err := json.Unmarshal(operationFixture.Value, &operation); err != nil {
		t.Fatal(err)
	}
	operation["state_version"] = 0
	if err := ValidateFixture(marshalFixture(t, "Operation", operation), time.Time{}); err != nil {
		t.Fatalf("schema-valid zero state_version rejected: %v", err)
	}
	for _, value := range []any{nil, "omitted"} {
		copy := cloneMap(t, operation)
		if value == "omitted" {
			delete(copy, "state_version")
		} else {
			copy["state_version"] = nil
		}
		assertInvalidFixture(t, "Operation", copy)
	}

	proposal := map[string]any{
		"schema_version": SchemaVersion,
		"proposal_id":    "018f4f1c-9f9a-7c21-a06f-2f4d3d25d701",
		"model_version":  "",
		"decision":       "NO_TRADE",
		"created_at":     "2026-07-29T04:00:00Z",
	}
	if err := ValidateFixture(marshalFixture(t, "ModelProposal", proposal), time.Time{}); err != nil {
		t.Fatalf("schema-valid empty model_version rejected: %v", err)
	}
	for _, value := range []any{nil, "omitted"} {
		copy := cloneMap(t, proposal)
		if value == "omitted" {
			delete(copy, "model_version")
		} else {
			copy["model_version"] = nil
		}
		assertInvalidFixture(t, "ModelProposal", copy)
	}
}

func assertInvalidFixture(t *testing.T, schema string, value any) {
	t.Helper()
	if err := ValidateFixture(marshalFixture(t, schema, value), time.Time{}); err == nil {
		t.Fatalf("schema-invalid %s accepted", schema)
	}
}

func marshalFixture(t *testing.T, schema string, value any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"schema": schema, "value": value})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestConfirmationGoldenAndExpiry(t *testing.T) {
	ticket := ticketFromFixture(t)
	got, err := ConfirmationDigest(ticket)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(readFixture(t, "fixtures/golden/confirmation-open.sha256")))
	if got != want || got != ticket.ConfirmationHash {
		t.Fatalf("digest got %s want %s", got, want)
	}
	if err := ticket.Validate(time.Date(2026, 7, 29, 4, 5, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Validate(time.Date(2026, 7, 29, 4, 10, 0, 0, time.UTC)); err == nil {
		t.Fatal("expiry boundary accepted")
	}
}

func TestEveryConfirmationBoundFieldMutationChangesDigest(t *testing.T) {
	var fixture fixtureEnvelope
	if err := StrictDecode(readFixture(t, "fixtures/valid/confirmation-ticket-open.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(fixture.Value)))
	decoder.UseNumber()
	if err := decoder.Decode(&original); err != nil {
		t.Fatal(err)
	}
	base := digestBindingMap(t, original)
	for _, path := range confirmationFields {
		t.Run(path, func(t *testing.T) {
			cloned := cloneMap(t, original)
			segments := strings.Split(path, ".")
			current := cloned
			for _, segment := range segments[:len(segments)-1] {
				current = current[segment].(map[string]any)
			}
			last := segments[len(segments)-1]
			current[last] = mutate(current[last])
			if got := digestBindingMap(t, cloned); got == base {
				t.Fatal("mutation did not change digest")
			}
		})
	}
}

func TestConfirmationFieldsMatchFrozenContract(t *testing.T) {
	var contract struct {
		SchemaVersion    string   `json:"schema_version"`
		Algorithm        string   `json:"algorithm"`
		Canonicalization string   `json:"canonicalization"`
		Fields           []string `json:"fields"`
	}
	if err := StrictDecode(readFixture(t, "confirmation-fields.json"), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.SchemaVersion != "fit.confirmation-hash.v1" || contract.Algorithm != "sha256" ||
		contract.Canonicalization != "RFC8785-JCS" {
		t.Fatal("unsupported frozen confirmation contract")
	}
	if !reflect.DeepEqual(ConfirmationFields(), contract.Fields) {
		t.Fatalf("Go confirmation fields drifted from frozen contract\ngot:  %q\nwant: %q", ConfirmationFields(), contract.Fields)
	}
}

func cloneMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(value)
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func digestBindingMap(t *testing.T, root map[string]any) string {
	t.Helper()
	binding := make(map[string]any)
	for _, path := range confirmationFields {
		value, err := atPath(root, path)
		if err != nil {
			t.Fatal(err)
		}
		binding[path] = value
	}
	raw, err := canonicalJSON(binding)
	if err != nil {
		t.Fatal(err)
	}
	// Reuse the public implementation by comparing canonical material through a
	// tiny deterministic string. This test only needs equality/non-equality.
	return string(raw)
}

func mutate(value any) any {
	switch x := value.(type) {
	case string:
		return x + "x"
	case bool:
		return !x
	case json.Number:
		return json.Number("6")
	case []any:
		return append(x, map[string]any{"mutation": true})
	default:
		return "mutation"
	}
}

func TestAllStateMachineEdges(t *testing.T) {
	operationStates := []OperationState{
		OperationDraft, OperationAwaiting, OperationExpired, OperationConfirmed,
		OperationRiskValidating, OperationRejected, OperationAdmitted,
		OperationDispatchPending, OperationDispatched, OperationAcknowledged,
		OperationUnknown, OperationManual, OperationFinal,
	}
	for _, from := range operationStates {
		for _, to := range operationStates {
			err := ValidateOperationTransition(from, to)
			_, allowed := operationEdges[[2]OperationState{from, to}]
			if (err == nil) != allowed {
				t.Fatalf("Operation %s -> %s allowed=%v err=%v", from, to, allowed, err)
			}
		}
	}
	protectionStates := []ProtectionState{
		ProtectionNoPosition, ProtectionEntryPending, ProtectionPartiallyFilled,
		ProtectionFilled, ProtectionPending, ProtectionProtected,
		ProtectionAdjusting, ProtectionFailed, ProtectionEmergencyClosing,
		ProtectionManual, ProtectionClosed,
	}
	for _, from := range protectionStates {
		for _, to := range protectionStates {
			err := ValidateProtectionTransition(from, to)
			_, allowed := protectionEdges[[2]ProtectionState{from, to}]
			if (err == nil) != allowed {
				t.Fatalf("protection %s -> %s allowed=%v err=%v", from, to, allowed, err)
			}
		}
	}
}

func TestStateMachineValidatorsMatchFrozenGraphs(t *testing.T) {
	assertFrozenGraph(t, "state-machines/operation.json", "Operation",
		func(from, to string) error {
			return ValidateOperationTransition(OperationState(from), OperationState(to))
		}, stringOperationEdges())
	assertFrozenGraph(t, "state-machines/protection.json", "PositionProtection",
		func(from, to string) error {
			return ValidateProtectionTransition(ProtectionState(from), ProtectionState(to))
		}, stringProtectionEdges())
}

func assertFrozenGraph(t *testing.T, path, name string, validate func(string, string) error, implemented map[[2]string]struct{}) {
	t.Helper()
	var graph struct {
		Name        string      `json:"name"`
		Initial     string      `json:"initial"`
		Terminal    []string    `json:"terminal"`
		States      []string    `json:"states"`
		Transitions [][2]string `json:"transitions"`
		InvariantID string      `json:"invariant_id,omitempty"`
	}
	if err := StrictDecode(readFixture(t, path), &graph); err != nil {
		t.Fatal(err)
	}
	if graph.Name != name || graph.Initial == "" || len(graph.States) == 0 {
		t.Fatalf("invalid frozen %s graph metadata", name)
	}
	frozen := make(map[[2]string]struct{}, len(graph.Transitions))
	for _, edge := range graph.Transitions {
		if _, duplicate := frozen[edge]; duplicate {
			t.Fatalf("%s frozen graph has duplicate transition %s -> %s", name, edge[0], edge[1])
		}
		frozen[edge] = struct{}{}
	}
	if !reflect.DeepEqual(implemented, frozen) {
		t.Fatalf("%s Go transition table drifted from frozen graph\ngot:  %v\nwant: %v", name, implemented, frozen)
	}
	for _, from := range graph.States {
		for _, to := range graph.States {
			_, allowed := frozen[[2]string{from, to}]
			if err := validate(from, to); (err == nil) != allowed {
				t.Fatalf("%s %s -> %s: frozen allowed=%v, validator error=%v", name, from, to, allowed, err)
			}
		}
	}
	for _, unknown := range [][2]string{{"UNKNOWN_FROZEN_STATE", graph.Initial}, {graph.Initial, "UNKNOWN_FROZEN_STATE"}} {
		if err := validate(unknown[0], unknown[1]); err == nil {
			t.Fatalf("%s validator accepted unknown state transition %s -> %s", name, unknown[0], unknown[1])
		}
	}
}

func stringOperationEdges() map[[2]string]struct{} {
	result := make(map[[2]string]struct{}, len(operationEdges))
	for edge := range operationEdges {
		result[[2]string{string(edge[0]), string(edge[1])}] = struct{}{}
	}
	return result
}

func stringProtectionEdges() map[[2]string]struct{} {
	result := make(map[[2]string]struct{}, len(protectionEdges))
	for edge := range protectionEdges {
		result[[2]string{string(edge[0]), string(edge[1])}] = struct{}{}
	}
	return result
}

func TestTakeProfitPlanSchemaSemantics(t *testing.T) {
	base := ticketFromFixture(t).Intent
	base.TakeProfitPlan = []TakeProfitLeg{}
	if err := base.Validate(); err != nil {
		t.Fatalf("required empty take_profit_plan rejected despite absent minItems: %v", err)
	}
	base.TakeProfitPlan = nil
	if err := base.Validate(); err == nil {
		t.Fatal("missing/null take_profit_plan accepted despite required array schema")
	}
	base.TakeProfitPlan = make([]TakeProfitLeg, 9)
	if err := base.Validate(); err == nil {
		t.Fatal("take_profit_plan above maxItems accepted")
	}
}

func TestExactProtectionCoverage(t *testing.T) {
	position := PositionSnapshot{"position", "-0.025", "protection", ProtectionProtected}
	protection := ProtectionStatus{"protection", "position", ProtectionProtected, "0.025", []string{"stop"}, strings.Repeat("a", 64)}
	if err := ValidateProtectionCoverage(position, protection); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*ProtectionStatus){
		func(p *ProtectionStatus) { p.State = ProtectionPending },
		func(p *ProtectionStatus) { p.ProtectionStatusID = "other" },
		func(p *ProtectionStatus) { p.PositionID = "other" },
		func(p *ProtectionStatus) { p.AbsoluteLivePositionAmount = "0.024999999999999999" },
		func(p *ProtectionStatus) { p.ActiveStopOrderIDs = nil },
		func(p *ProtectionStatus) { p.CoverageEvidenceHash = "BAD" },
	}
	for i, mutation := range mutations {
		copy := protection
		mutation(&copy)
		if err := ValidateProtectionCoverage(position, copy); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
	for _, id := range []string{"", " ", "\t\r\n"} {
		copy := protection
		copy.ActiveStopOrderIDs = []string{id}
		if err := ValidateProtectionCoverage(position, copy); err == nil {
			t.Fatalf("blank stop order identifier %q accepted", id)
		}
	}
}

func TestExactDecimalComparisonsDoNotMutateOperands(t *testing.T) {
	value, err := ParseDecimal("1.000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	before := new(big.Int).Set(&value.coefficient)
	for range 3 {
		if !decimalGreaterThanOne(value) {
			t.Fatal("greater-than comparison returned false")
		}
		if value.coefficient.Cmp(before) != 0 || value.String() != "1.000000000000000001" {
			t.Fatal("greater-than comparison mutated operand")
		}
	}
	negative, _ := ParseDecimal("-0.025")
	positive, _ := ParseDecimal("0.025")
	negativeBefore := new(big.Int).Set(&negative.coefficient)
	positiveBefore := new(big.Int).Set(&positive.coefficient)
	if !negative.EqualAbs(positive) || negative.coefficient.Cmp(negativeBefore) != 0 ||
		positive.coefficient.Cmp(positiveBefore) != 0 {
		t.Fatal("absolute equality comparison mutated operand")
	}
}

func TestIdempotencyConcurrentDuplicateAndRestart(t *testing.T) {
	for _, kind := range []IdentifierKind{RequestID, AttemptID, EventID, ClientOrderID} {
		store := NewIdempotencyStore()
		var wg sync.WaitGroup
		results := make(chan bool, 32)
		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, duplicate, err := store.Apply(kind, "same-id", "existing-result")
				if err != nil {
					t.Error(err)
				}
				results <- duplicate
			}()
		}
		wg.Wait()
		close(results)
		first := 0
		for duplicate := range results {
			if !duplicate {
				first++
			}
		}
		if first != 1 {
			t.Fatalf("%s produced %d first effects", kind, first)
		}
		restored, err := RestoreIdempotencyStore(store.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		if got, duplicate, err := restored.Apply(kind, "same-id", "existing-result"); err != nil || !duplicate || got != "existing-result" {
			t.Fatalf("restart did not preserve result: %q %v %v", got, duplicate, err)
		}
		if _, _, err := restored.Apply(kind, "same-id", "different"); !errors.Is(err, ErrIdentifierConflict) {
			t.Fatal("identifier rebinding accepted")
		}
	}
}

func TestIdempotencyRejectsUnknownIdentifierKinds(t *testing.T) {
	store := NewIdempotencyStore()
	if _, _, err := store.Apply(IdentifierKind("future_kind"), "id", "result"); !errors.Is(err, ErrUnknownIdentifierKind) {
		t.Fatalf("unknown identifier kind error = %v", err)
	}
	if len(store.Snapshot()) != 0 {
		t.Fatal("unknown identifier kind mutated store")
	}
	if _, err := RestoreIdempotencyStore([]Record{{Kind: IdentifierKind("future_kind"), ID: "id", Result: "result"}}); !errors.Is(err, ErrUnknownIdentifierKind) {
		t.Fatalf("restore unknown identifier kind error = %v", err)
	}
}

func TestFailClosedCrashTimeoutAndReconnectScenarios(t *testing.T) {
	tests := []struct {
		name     string
		sent     bool
		recorded bool
		want     DispatchOutcome
	}{
		{"timeout_before_send", false, false, NotDispatched},
		{"crash_before_signing", false, false, NotDispatched},
		{"crash_after_signing_before_submit", false, false, NotDispatched},
		{"timeout_after_send", true, false, UnknownRequiresReconciliation},
		{"crash_after_submit_before_record", true, false, UnknownRequiresReconciliation},
		{"crash_after_exchange_success", true, false, UnknownRequiresReconciliation},
		{"acknowledged", true, true, Acknowledged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := OutcomeAfterDispatch(test.sent, test.recorded); got != test.want {
				t.Fatalf("got %s want %s", got, test.want)
			}
		})
	}
	if err := ValidateOperationTransition(OperationUnknown, OperationDispatchPending); err == nil {
		t.Fatal("UNKNOWN allowed blind retry")
	}
	for _, edge := range [][2]ProtectionState{
		{ProtectionEntryPending, ProtectionPartiallyFilled},
		{ProtectionPartiallyFilled, ProtectionPending},
		{ProtectionPending, ProtectionPartiallyFilled},
		{ProtectionPending, ProtectionProtected},
		{ProtectionPending, ProtectionFailed},
		{ProtectionFailed, ProtectionEmergencyClosing},
		{ProtectionEmergencyClosing, ProtectionManual},
	} {
		if err := ValidateProtectionTransition(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryFrozenFakeScenario(t *testing.T) {
	var contract struct {
		SchemaVersion string `json:"schema_version"`
		NetworkAccess bool   `json:"network_access"`
		SyntheticOnly bool   `json:"synthetic_only"`
		Scenarios     []struct {
			ID     string `json:"id"`
			Effect string `json:"effect"`
			Result string `json:"result"`
		} `json:"scenarios"`
	}
	if err := StrictDecode(readFixture(t, "fake-hyperliquid-scenarios-v1.json"), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.SchemaVersion != "fit.fake-hyperliquid.v1" || contract.NetworkAccess || !contract.SyntheticOnly {
		t.Fatal("fake scenarios do not fail closed to synthetic-only")
	}
	if len(contract.Scenarios) != len(frozenScenarioResults) {
		t.Fatalf("scenario implementation has %d entries, contract has %d", len(frozenScenarioResults), len(contract.Scenarios))
	}
	for _, scenario := range contract.Scenarios {
		if scenario.Effect == "" {
			t.Errorf("%s has empty frozen effect", scenario.ID)
		}
		got, ok := FrozenScenarioResult(scenario.ID)
		if !ok || got != scenario.Result {
			t.Errorf("%s got %q, %v; want %q", scenario.ID, got, ok, scenario.Result)
		}
	}
	if _, ok := FrozenScenarioResult("unknown"); ok {
		t.Fatal("unknown scenario accepted")
	}
}

func TestCanonicalJSONStringsAreAlwaysValidJSON(t *testing.T) {
	inputs := []string{
		"plain",
		"quote\"slash\\control\n",
		"\x00\x1f\x7f",
		"\u2028\u2029",
		string([]byte{0xff, 'x'}),
	}
	for _, input := range inputs {
		got, err := canonicalJSON(map[string]any{input: input})
		if err != nil {
			t.Fatalf("canonicalJSON(%q): %v", input, err)
		}
		if !json.Valid(got) {
			t.Fatalf("canonicalJSON(%q) emitted invalid JSON: %q", input, got)
		}
		var decoded map[string]string
		if err := json.Unmarshal(got, &decoded); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNoFinancialFloatFields(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(TradeIntent{}), reflect.TypeOf(ConfirmationTicket{}), reflect.TypeOf(PositionSnapshot{}), reflect.TypeOf(ProtectionStatus{})} {
		for i := 0; i < typ.NumField(); i++ {
			kind := typ.Field(i).Type.Kind()
			if kind == reflect.Float32 || kind == reflect.Float64 {
				t.Fatalf("%s.%s is floating point", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
