package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

type fixtureEnvelope struct {
	Schema string          `json:"schema"`
	Reason string          `json:"reason,omitempty"`
	Value  json.RawMessage `json:"value"`
}

// ValidateFixture validates the shared fixture object types consumed by this
// module. It is intentionally not a second general-purpose JSON Schema engine.
func ValidateFixture(data []byte, now time.Time) error {
	var fixture fixtureEnvelope
	if err := StrictDecode(data, &fixture); err != nil {
		return err
	}
	switch fixture.Schema {
	case "TradeIntent":
		if err := rejectExplicitNull(fixture.Value, "stop"); err != nil {
			return err
		}
		var value TradeIntent
		if err := StrictDecode(fixture.Value, &value); err != nil {
			return err
		}
		return value.Validate()
	case "ConfirmationTicket":
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(fixture.Value, &properties); err != nil {
			return err
		}
		if intent, present := properties["intent"]; present {
			if err := rejectExplicitNull(intent, "stop"); err != nil {
				return err
			}
		}
		var value ConfirmationTicket
		if err := StrictDecode(fixture.Value, &value); err != nil {
			return err
		}
		return value.Validate(now)
	case "Operation":
		if err := rejectExplicitNull(fixture.Value, "confirmation_id", "rejection_code"); err != nil {
			return err
		}
		var value struct {
			SchemaVersion  string         `json:"schema_version"`
			OperationID    string         `json:"operation_id"`
			IntentID       string         `json:"intent_id"`
			ConfirmationID string         `json:"confirmation_id,omitempty"`
			State          OperationState `json:"state"`
			StateVersion   *int           `json:"state_version"`
			RejectionCode  string         `json:"rejection_code,omitempty"`
			CreatedAt      string         `json:"created_at"`
			UpdatedAt      string         `json:"updated_at"`
		}
		if err := StrictDecode(fixture.Value, &value); err != nil {
			return err
		}
		if value.SchemaVersion != SchemaVersion || !uuidPattern.MatchString(value.OperationID) ||
			!uuidPattern.MatchString(value.IntentID) || value.StateVersion == nil || *value.StateVersion < 0 {
			return errors.New("invalid operation")
		}
		if value.ConfirmationID != "" && !uuidPattern.MatchString(value.ConfirmationID) {
			return errors.New("invalid confirmation id")
		}
		if !knownOperationState(value.State) {
			return errors.New("unknown operation state")
		}
		if utf8.RuneCountInString(value.RejectionCode) > 96 {
			return errors.New("rejection_code exceeds max length")
		}
		if _, err := time.Parse(time.RFC3339, value.CreatedAt); err != nil {
			return errors.New("invalid operation created_at")
		}
		if _, err := time.Parse(time.RFC3339, value.UpdatedAt); err != nil {
			return errors.New("invalid operation updated_at")
		}
		return nil
	case "ModelProposal":
		if err := rejectExplicitNull(fixture.Value, "intent"); err != nil {
			return err
		}
		var value struct {
			SchemaVersion string          `json:"schema_version"`
			ProposalID    string          `json:"proposal_id"`
			ModelVersion  *string         `json:"model_version"`
			Decision      string          `json:"decision"`
			Intent        json.RawMessage `json:"intent,omitempty"`
			CreatedAt     string          `json:"created_at"`
		}
		if err := StrictDecode(fixture.Value, &value); err != nil {
			return err
		}
		if value.SchemaVersion != SchemaVersion || !uuidPattern.MatchString(value.ProposalID) ||
			value.ModelVersion == nil || utf8.RuneCountInString(*value.ModelVersion) > 64 ||
			(value.Decision != "PROPOSE_TRADE" && value.Decision != "NO_TRADE") {
			return errors.New("invalid model proposal")
		}
		hasIntent := len(bytes.TrimSpace(value.Intent)) != 0
		if (value.Decision == "PROPOSE_TRADE") != hasIntent {
			return errors.New("proposal decision/intent mismatch")
		}
		if _, err := time.Parse(time.RFC3339, value.CreatedAt); err != nil {
			return errors.New("invalid model proposal created_at")
		}
		if hasIntent {
			if err := rejectExplicitNull(value.Intent, "stop"); err != nil {
				return err
			}
			var intent TradeIntent
			if err := StrictDecode(value.Intent, &intent); err != nil {
				return err
			}
			if err := intent.Validate(); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("fixture schema is outside trading-core scope")
	}
}

func rejectExplicitNull(data []byte, propertyNames ...string) error {
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(data, &properties); err != nil {
		return err
	}
	for _, name := range propertyNames {
		raw, present := properties[name]
		if present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s must not be null", name)
		}
	}
	return nil
}

func knownOperationState(state OperationState) bool {
	switch state {
	case OperationDraft, OperationAwaiting, OperationExpired, OperationConfirmed,
		OperationRiskValidating, OperationRejected, OperationAdmitted,
		OperationDispatchPending, OperationDispatched, OperationAcknowledged,
		OperationUnknown, OperationManual, OperationFinal:
		return true
	default:
		return false
	}
}
