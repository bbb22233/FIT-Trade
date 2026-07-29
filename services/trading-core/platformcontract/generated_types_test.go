package platformcontract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestGeneratedAll59Dispatches(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	definitions := schemaMap(validator.platform["$defs"])
	names := make([]string, 0, len(definitions)+2)
	for name := range definitions {
		names = append(names, name)
	}
	names = append(names, "RecoveryEvidence", "RemediationProposal")
	sort.Strings(names)
	if len(names) != GeneratedSchemaTypeCount {
		t.Fatalf("generated schema dispatch count=%d, frozen schema count=%d", GeneratedSchemaTypeCount, len(names))
	}

	candidates := generatedCandidateCorpus(t)
	built := make(map[string]string, len(names))
	for _, name := range names {
		for index, candidate := range candidates {
			if validator.validate(name, candidate) != nil {
				continue
			}
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("%s candidate %d marshal: %v", name, index, err)
			}
			value, err := validator.DecodeTyped(name, raw)
			if err != nil {
				t.Fatalf("%s candidate %d validates but generated construction failed: %v", name, index, err)
			}
			if value.SchemaName() != name {
				t.Fatalf("%s generated value reports schema %q", name, value.SchemaName())
			}
			built[name] = reflect.TypeOf(value).Name()
			break
		}
		if built[name] == "" {
			t.Errorf("%s has no directly usable representative in the complete frozen fixture corpus", name)
		}
	}
	if len(built) != GeneratedSchemaTypeCount {
		t.Fatalf("generated dispatches exercised=%d, want %d", len(built), GeneratedSchemaTypeCount)
	}
	if _, err := validator.DecodeTyped("not-a-frozen-schema", []byte(`{}`)); err == nil {
		t.Fatal("unknown generated schema dispatch was accepted")
	}
}

func TestGeneratedFieldAccessorsAndTaggedValues(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}

	userRaw := generatedValidCaseRaw(t, "active synthetic user")
	user, err := validator.DecodeUser(userRaw)
	if err != nil {
		t.Fatal(err)
	}
	if user.SchemaVersion() == "" || user.UserID().String() == "" || !user.Status().Valid() {
		t.Fatal("generated user fields are not available through typed accessors")
	}

	scope, err := validator.DecodeScope([]byte(`{"type":"SYSTEM"}`))
	if err != nil {
		t.Fatal(err)
	}
	if scope.Kind() != ScopeKindSystemScope {
		t.Fatalf("scope tag=%v, want system branch", scope.Kind())
	}
	system, ok := scope.SystemScope()
	if !ok || system.Type() != "SYSTEM" {
		t.Fatal("scope system branch view is unavailable")
	}
	if _, ok := scope.OwnerScope(); ok {
		t.Fatal("scope exposes a losing union branch")
	}

	value, err := validator.DecodeJsonValue([]byte(`{"array":[null,true,1,"text"]}`))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.Object()
	if !ok {
		t.Fatal("recursive JSON value lost its object tag")
	}
	arrayValue, ok := object.Lookup("array")
	if !ok {
		t.Fatal("recursive JSON object lookup failed")
	}
	array, ok := arrayValue.Array()
	if !ok || len(array) != 4 {
		t.Fatal("recursive JSON array view failed")
	}
	wantKinds := []JsonValueKind{JsonValueNull, JsonValueBoolean, JsonValueNumber, JsonValueString}
	for index, want := range wantKinds {
		if array[index].Kind() != want {
			t.Errorf("recursive JSON item %d tag=%v, want %v", index, array[index].Kind(), want)
		}
	}

	if _, err := validator.DecodeUser([]byte(`{"schema_version":"fit.platform.user.v1"}`)); err == nil {
		t.Fatal("generated user construction bypassed required-field validation")
	}
	if _, err := validator.DecodeJsonValue([]byte(`{"x":1,"x":2}`)); err == nil {
		t.Fatal("generated JSON value construction bypassed strict duplicate-key decoding")
	}
}

func TestGeneratedFieldDispositionInventory(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	public := validator.FieldMappings()
	detailed := GeneratedFieldDispositions()
	if len(public) != GeneratedSchemaPropertyCount || len(detailed) != GeneratedSchemaPropertyCount {
		t.Fatalf("field inventories public=%d generated=%d expected=%d", len(public), len(detailed), GeneratedSchemaPropertyCount)
	}
	if !reflect.DeepEqual(public, detailed) {
		t.Fatal("public field disposition inventory is not the exact generated inventory")
	}
	transportCount := 0
	fieldCount := 0
	for pointer, disposition := range detailed {
		switch {
		case strings.HasPrefix(disposition, "field:"):
			fieldCount++
			if _, ok := generatedFieldAccessors[pointer]; !ok {
				t.Errorf("%s field disposition has no compile-time checked accessor", pointer)
			}
		case strings.HasPrefix(disposition, "union:"):
			if _, ok := generatedFieldAccessors[pointer]; ok {
				t.Errorf("%s union disposition unexpectedly has a field accessor", pointer)
			}
		case strings.HasPrefix(disposition, "transport-only:"):
			transportCount++
			if _, ok := generatedFieldAccessors[pointer]; ok {
				t.Errorf("%s transport-only disposition unexpectedly has a field accessor", pointer)
			}
		default:
			t.Errorf("%s has unknown generated disposition %q", pointer, disposition)
		}
	}
	if len(generatedFieldAccessors) != fieldCount {
		t.Fatalf("compile-time field accessors=%d, field dispositions=%d", len(generatedFieldAccessors), fieldCount)
	}
	if transportCount != 1 {
		t.Fatalf("transport-only generated fields=%d, want 1", transportCount)
	}

	canonical := make(map[string]any, len(detailed))
	for pointer, disposition := range detailed {
		canonical[pointer] = disposition
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(CanonicalJSON(canonical))))
	const wantGeneratedDispositionDigest = "9e8edb2ec0d9aa9fa14b9bd22c0ed41f0c7ac02083f9bb421bdb43cd11399b15"
	if got != wantGeneratedDispositionDigest {
		t.Fatalf("generated field disposition digest=%s, want %s", got, wantGeneratedDispositionDigest)
	}
	if GeneratedFieldDispositionSHA256 != wantGeneratedDispositionDigest {
		t.Fatalf("generated disposition constant=%s, want %s", GeneratedFieldDispositionSHA256, wantGeneratedDispositionDigest)
	}
	if got := validator.FieldMappingsDigest(); got != wantGeneratedDispositionDigest {
		t.Fatalf("public field disposition digest=%s, want %s", got, wantGeneratedDispositionDigest)
	}
}

func TestGeneratedTransportFieldIsEphemeral(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	raw := generatedValidCaseRaw(t, "websocket client reauthorization response")
	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	secret, _ := fixture["access_token_transport"].(string)
	if secret == "" {
		t.Fatal("reauth fixture has no transport credential")
	}
	reauth, err := validator.DecodeWebSocketReauth(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reflect.TypeOf(reauth).MethodByName("AccessTokenTransport"); ok {
		t.Fatal("writeOnly transport credential gained a public accessor")
	}
	transportField, ok := reflect.TypeOf(reauth).FieldByName("accessTokenTransport")
	if !ok || transportField.Type.Kind() != reflect.Struct || transportField.Type.NumField() != 0 {
		t.Fatal("transport credential marker retained storage")
	}
	for index := 0; index < reflect.TypeOf(reauth).NumField(); index++ {
		if reflect.TypeOf(reauth).Field(index).IsExported() {
			t.Fatalf("WebSocketReauth field %q is exported", reflect.TypeOf(reauth).Field(index).Name)
		}
	}
	for _, rendered := range []string{
		fmt.Sprint(reauth),
		fmt.Sprintf("%+v", reauth),
		fmt.Sprintf("%#v", reauth),
		fmt.Sprintf("%q", reauth),
	} {
		if strings.Contains(rendered, secret) {
			t.Fatal("transport credential leaked through formatting")
		}
		if !strings.Contains(rendered, "[REDACTED]") {
			t.Fatalf("transport formatter did not visibly redact: %q", rendered)
		}
	}
	if encoded, err := json.Marshal(reauth); err == nil {
		t.Fatalf("transport-only reauth unexpectedly marshaled: %s", encoded)
	}
	if encoded, err := json.Marshal(&reauth); err == nil {
		t.Fatalf("transport-only reauth pointer unexpectedly marshaled: %s", encoded)
	}
	textMarshaler := any(reauth).(interface {
		MarshalText() ([]byte, error)
	})
	if encoded, err := textMarshaler.MarshalText(); err == nil {
		t.Fatalf("transport-only reauth unexpectedly text-marshaled: %s", encoded)
	}
	if strings.Contains(fmt.Sprintf("%#v", reauth), secret) {
		t.Fatal("transport credential remains in the generated value")
	}
}

func TestGeneratedManifestViews(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := map[string]reflect.Type{
		"failure-injection":      reflect.TypeOf(FailureInjectionManifest{}),
		"http-rate-limits":       reflect.TypeOf(HTTPRateLimitsManifest{}),
		"migration-policy":       reflect.TypeOf(MigrationPolicyManifest{}),
		"nats-permissions":       reflect.TypeOf(NATSPermissionsManifest{}),
		"phase0-file-manifest":   reflect.TypeOf(Phase0FileManifest{}),
		"recovery-policy":        reflect.TypeOf(RecoveryPolicyManifest{}),
		"security-values":        reflect.TypeOf(SecurityValuesManifest{}),
		"state-machines":         reflect.TypeOf(StateMachinesManifest{}),
		"transaction-boundaries": reflect.TypeOf(TransactionBoundariesManifest{}),
		"websocket-protocol":     reflect.TypeOf(WebSocketProtocolManifest{}),
	}
	if len(wantTypes) != GeneratedManifestCount {
		t.Fatalf("manifest type inventory=%d, generated=%d", len(wantTypes), GeneratedManifestCount)
	}
	for name, wantType := range wantTypes {
		manifest, err := validator.Manifest(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if manifest.ManifestName() != name {
			t.Errorf("%s reports manifest name %q", name, manifest.ManifestName())
		}
		if reflect.TypeOf(manifest) != wantType {
			t.Errorf("%s generated type=%v, want %v", name, reflect.TypeOf(manifest), wantType)
		}
		versioned, ok := manifest.(interface{ SchemaVersion() string })
		if !ok || versioned.SchemaVersion() == "" {
			t.Errorf("%s has no typed schema-version view", name)
		}
	}
	if _, err := validator.Manifest("not-a-frozen-manifest"); err == nil {
		t.Fatal("unknown generated manifest was accepted")
	}

	manifest, err := validator.Manifest("failure-injection")
	if err != nil {
		t.Fatal(err)
	}
	failure := manifest.(FailureInjectionManifest)
	methods := failure.InjectionMethods()
	if len(methods) == 0 {
		t.Fatal("failure-injection manifest has no generated array data")
	}
	original := methods[0]
	methods[0] = "mutated-outside-view"
	if failure.InjectionMethods()[0] != original {
		t.Fatal("manifest array accessor exposed mutable backing storage")
	}

	requireKinds := func(name string, values []uint8, wantItems, wantKinds int) {
		t.Helper()
		if len(values) != wantItems {
			t.Fatalf("%s items=%d, want %d", name, len(values), wantItems)
		}
		kinds := make(map[uint8]bool)
		for index, kind := range values {
			if kind == 0 {
				t.Fatalf("%s item %d has invalid generated union tag", name, index)
			}
			kinds[kind] = true
		}
		if len(kinds) != wantKinds {
			t.Fatalf("%s generated union kinds=%d, want %d", name, len(kinds), wantKinds)
		}
	}
	requirementKinds := make([]uint8, 0, len(failure.WorkflowRequirements()))
	for _, requirement := range failure.WorkflowRequirements() {
		requirementKinds = append(requirementKinds, uint8(requirement.Kind()))
	}
	requireKinds("failure-injection workflow requirements", requirementKinds, 14, 3)

	natsValue, err := validator.Manifest("nats-permissions")
	if err != nil {
		t.Fatal(err)
	}
	principals := natsValue.(NATSPermissionsManifest).Principals()
	principalKinds := make([]uint8, 0, len(principals))
	for _, principal := range principals {
		principalKinds = append(principalKinds, uint8(principal.Kind()))
	}
	requireKinds("NATS principals", principalKinds, 3, 2)

	transactionValue, err := validator.Manifest("transaction-boundaries")
	if err != nil {
		t.Fatal(err)
	}
	workflows := transactionValue.(TransactionBoundariesManifest).Workflows()
	workflowKinds := make([]uint8, 0, len(workflows))
	for _, workflow := range workflows {
		workflowKinds = append(workflowKinds, uint8(workflow.Kind()))
	}
	requireKinds("transaction workflows", workflowKinds, 14, 12)
}

func TestGeneratedManifestFieldDispositionInventory(t *testing.T) {
	dispositions := GeneratedManifestFieldDispositions()
	if len(dispositions) != GeneratedManifestPropertyCount {
		t.Fatalf(
			"manifest field dispositions=%d, generated count=%d",
			len(dispositions),
			GeneratedManifestPropertyCount,
		)
	}
	if GeneratedManifestPropertyCount != 596 {
		t.Fatalf("manifest property paths=%d, want exact frozen count 596", GeneratedManifestPropertyCount)
	}
	if GeneratedManifestStructuralFieldCount != 677 {
		t.Fatalf(
			"manifest concrete structural fields=%d, want exact generated count 677",
			GeneratedManifestStructuralFieldCount,
		)
	}
	fieldCount := 0
	unionCount := 0
	for pointer, disposition := range dispositions {
		switch {
		case strings.HasPrefix(disposition, "field:"):
			fieldCount++
			if _, ok := generatedManifestFieldAccessors[pointer]; !ok {
				t.Errorf("%s field disposition has no compile-time checked accessor", pointer)
			}
		case strings.HasPrefix(disposition, "union:"):
			unionCount++
			if _, ok := generatedManifestFieldAccessors[pointer]; ok {
				t.Errorf("%s union disposition unexpectedly has a direct accessor", pointer)
			}
			if _, ok := generatedManifestUnionTags[pointer]; !ok {
				t.Errorf("%s union disposition has no compile-time checked tag", pointer)
			}
		default:
			t.Errorf("%s has unknown manifest disposition %q", pointer, disposition)
		}
	}
	if fieldCount != 547 || unionCount != 49 {
		t.Fatalf(
			"manifest dispositions fields=%d unions=%d, want fields=547 unions=49",
			fieldCount,
			unionCount,
		)
	}
	if len(generatedManifestFieldAccessors) != fieldCount {
		t.Fatalf(
			"manifest compile-time accessors=%d, field dispositions=%d",
			len(generatedManifestFieldAccessors),
			fieldCount,
		)
	}
	if len(generatedManifestUnionTags) != unionCount {
		t.Fatalf(
			"manifest compile-time union tags=%d, union dispositions=%d",
			len(generatedManifestUnionTags),
			unionCount,
		)
	}

	canonical := make(map[string]any, len(dispositions))
	for pointer, disposition := range dispositions {
		canonical[pointer] = disposition
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(CanonicalJSON(canonical))))
	const wantManifestDispositionDigest = "7fadeadd2bc911403bdc091a6206b1281f60ff2d9b759c2df86de0e7a7d5bdb0"
	if got != wantManifestDispositionDigest {
		t.Fatalf(
			"generated manifest field disposition digest=%s, want %s",
			got,
			wantManifestDispositionDigest,
		)
	}
	if GeneratedManifestFieldDispositionSHA256 != wantManifestDispositionDigest {
		t.Fatalf(
			"generated manifest disposition constant=%s, want %s",
			GeneratedManifestFieldDispositionSHA256,
			wantManifestDispositionDigest,
		)
	}
}

func TestGeneratedTypeDrift(t *testing.T) {
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBinary); err != nil {
		t.Fatalf("generated type authority runner unavailable at %s: %v", goBinary, err)
	}
	command := exec.Command(goBinary, "run", "./cmd/gentypes", "-check")
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated type drift check failed: %v\n%s", err, output)
	}
}

func generatedCandidateCorpus(t *testing.T) []any {
	t.Helper()
	candidates := []any{
		nil,
		true,
		json.Number("0"),
		"",
		[]any{},
		map[string]any{},
		"00000000-0000-4000-8000-000000000001",
		"2026-01-01T00:00:00.000Z",
		strings.Repeat("0", 64),
		map[string]any{
			"session_id": "00000000-0000-4000-8000-000000000001",
		},
	}
	files := []string{
		"fixtures/platform/valid-schema-cases-v1.json",
		"fixtures/platform/semantic-scenarios-v1.json",
		"fixtures/platform/executable-security-scenarios-v1.json",
		"fixtures/platform/linkage-chains-v1.json",
		"fixtures/platform/golden-vectors-v1.json",
	}
	var walk func(any)
	walk = func(value any) {
		candidates = append(candidates, value)
		switch item := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(item))
			for key := range item {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(item[key])
			}
		case []any:
			for _, child := range item {
				walk(child)
			}
		}
	}
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(generatedContractsRoot(), name))
		if err != nil {
			t.Fatal(err)
		}
		value, err := ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		walk(value)
	}
	for _, candidate := range candidates {
		event, ok := candidate.(map[string]any)
		if !ok || event["schema_version"] != "fit.platform.event-envelope.v1" {
			continue
		}
		candidates = append(candidates, map[string]any{
			"schema_version":   "fit.platform.aggregate-event-history.v1",
			"aggregate_type":   event["aggregate_type"],
			"aggregate_id":     event["aggregate_id"],
			"starting_version": json.Number("1"),
			"events":           []any{event},
		})
		break
	}
	return candidates
}

func generatedValidCaseRaw(t *testing.T, caseName string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(generatedContractsRoot(), "fixtures/platform/valid-schema-cases-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := ParseStrictJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	root := value.(map[string]any)
	for _, item := range root["cases"].([]any) {
		fixture := item.(map[string]any)
		if fixture["name"] != caseName {
			continue
		}
		raw, err := json.Marshal(fixture["value"])
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	t.Fatalf("valid fixture %q not found", caseName)
	return nil
}

func generatedContractsRoot() string {
	return filepath.Join("..", "..", "..", "contracts")
}
