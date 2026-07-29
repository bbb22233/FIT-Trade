package platformcontract

import (
	"encoding/json"
	"testing"
)

func TestValidatorConstructionAuditsEverySchemaNode(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	defs := schemaMap(v.platform["$defs"])
	unusedScalar := schemaMap(defs["Uuid"])
	unusedScalar["unsupported_optional_keyword"] = true
	if err := v.auditDocument("schemas/platform-v1.schema.json"); err == nil {
		t.Fatal("constructor audit ignored an unsupported keyword in an otherwise unused definition")
	}
	delete(unusedScalar, "unsupported_optional_keyword")

	optional := schemaMap(schemaMap(schemaMap(defs["User"])["properties"])["created_at"])
	optional["pattern"] = "["
	if err := v.auditDocument("schemas/platform-v1.schema.json"); err == nil {
		t.Fatal("constructor audit ignored an invalid pattern in an optional property")
	}
	delete(optional, "pattern")

	optional["items"] = true
	if err := v.auditDocument("schemas/platform-v1.schema.json"); err == nil {
		t.Fatal("constructor audit silently accepted an unsupported boolean items schema")
	}
	delete(optional, "items")
	if err := v.auditDocument("schemas/platform-v1.schema.json"); err != nil {
		t.Fatalf("restored frozen schema did not compile: %v", err)
	}
}

func TestDocumentLocalReferencesAndJSONSchemaAnnotations(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	recovery := v.docs["schemas/recovery-evidence-v1.schema.json"]
	recoveryTimestamp := schemaMap(schemaMap(recovery["$defs"])["Timestamp"])
	recoveryTimestamp["pattern"] = "^never-a-frozen-timestamp$"
	if err := v.validate("RecoveryEvidence", fixtureCase(t, "measured recovery evidence")); err == nil {
		t.Fatal("RecoveryEvidence resolved its local Timestamp ref through another document")
	}

	integerSchema := map[string]any{"type": "integer", "minimum": json.Number("0")}
	for _, number := range []json.Number{"1.0", "1e0", "-0", "-1e-400"} {
		if err := v.checkDocument("schemas/platform-v1.schema.json", integerSchema, number, "$"); err != nil {
			t.Errorf("Node/AJV integer %s rejected: %v", number, err)
		}
	}

	conditional := map[string]any{
		"type": "object",
		"anyOf": []any{
			map[string]any{
				"properties": map[string]any{"a": map[string]any{"type": "string"}},
				"required":   []any{"a"},
			},
			map[string]any{
				"properties": map[string]any{"b": map[string]any{"type": "string"}},
				"required":   []any{"b"},
			},
		},
		"unevaluatedProperties": false,
	}
	value := map[string]any{"a": "selected", "b": json.Number("1")}
	if err := v.checkDocument("schemas/platform-v1.schema.json", conditional, value, "$"); err == nil {
		t.Fatal("property from a losing anyOf branch was incorrectly treated as evaluated")
	}
}
