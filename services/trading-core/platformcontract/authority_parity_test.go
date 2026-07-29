package platformcontract

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const frozenNodeAuthoritySHA256 = "2d5a5cf52af2d35cd7d285450e3bc18db32d4ea29db21b881f36ac4224ac5ea5"

type nodeAuthorityRunner struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Scanner
	stderr  bytes.Buffer
	mutex   sync.Mutex
}

type nodeAuthorityResponse struct {
	Ready      bool                 `json:"ready"`
	Valid      bool                 `json:"valid"`
	Error      string               `json:"error"`
	Unicode    string               `json:"unicode"`
	Node       string               `json:"node"`
	SchemaName string               `json:"schema"`
	Issues     []nodeAuthorityIssue `json:"issues"`
}

type nodeAuthorityIssue struct {
	Keyword      string `json:"keyword"`
	InstancePath string `json:"instancePath"`
	SchemaPath   string `json:"schemaPath"`
}

type frozenNodeAuthorityProgram struct {
	nodePath      string
	contractsRoot string
	source        string
}

func loadFrozenNodeAuthorityProgram(t *testing.T) frozenNodeAuthorityProgram {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("frozen Node authority unavailable: node executable not found: %v", err)
	}
	contractsRoot, err := filepath.Abs(contractRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(contractsRoot, "scripts", "verify-platform.mjs")
	authoritySource, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatalf("frozen Node authority unavailable: %v", err)
	}
	if got := fmtHash(sha256.Sum256(authoritySource)); got != frozenNodeAuthoritySHA256 {
		t.Fatalf("frozen Node authority drift: got %s want %s", got, frozenNodeAuthoritySHA256)
	}
	nodeModules := os.Getenv("FIT_CONTRACT_NODE_MODULES")
	if nodeModules == "" {
		nodeModules = filepath.Join(contractsRoot, "node_modules")
	}
	nodeModules, err = filepath.Abs(nodeModules)
	if err != nil {
		t.Fatal(err)
	}
	ajvModule := filepath.Join(nodeModules, "ajv", "dist", "2020.js")
	formatsModule := filepath.Join(nodeModules, "ajv-formats", "dist", "index.js")
	for _, required := range []string{ajvModule, formatsModule} {
		if _, err := os.Stat(required); err != nil {
			t.Fatalf("frozen Node authority unavailable: %s is missing; run the pinned contracts npm install or set FIT_CONTRACT_NODE_MODULES: %v", required, err)
		}
	}
	assertNodePackageVersion(t, filepath.Join(nodeModules, "ajv", "package.json"), "8.20.0")
	assertNodePackageVersion(t, filepath.Join(nodeModules, "ajv-formats", "package.json"), "3.0.1")

	source := string(authoritySource)
	source = strings.Replace(source,
		`import Ajv2020 from "ajv/dist/2020.js";`,
		`import Ajv2020 from `+strconv.Quote(fileURL(ajvModule))+`;`, 1)
	source = strings.Replace(source,
		`import addFormats from "ajv-formats";`,
		`import addFormats from `+strconv.Quote(fileURL(formatsModule))+`;`, 1)
	const directoryLine = `const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));`
	source = strings.Replace(source, directoryLine,
		`const scriptDirectory = `+strconv.Quote(filepath.Join(contractsRoot, "scripts"))+`;`, 1)
	return frozenNodeAuthorityProgram{
		nodePath:      nodePath,
		contractsRoot: contractsRoot,
		source:        source,
	}
}

func startNodeAuthority(t *testing.T) *nodeAuthorityRunner {
	t.Helper()
	program := loadFrozenNodeAuthorityProgram(t)
	source := program.source
	const marker = "for (const fixture of validFixtureSet.cases) {"
	if strings.Count(source, marker) != 1 {
		t.Fatalf("frozen Node authority runner injection marker changed")
	}
	const protocol = `
const __fitReadline = await import("node:readline");
const __fitInput = __fitReadline.createInterface({
  input: process.stdin,
  crlfDelay: Infinity,
  terminal: false,
});
process.stdout.write(JSON.stringify({
  ready: true,
  node: process.version,
  unicode: process.versions.unicode,
}) + "\n");
for await (const __fitLine of __fitInput) {
  let __fitResponse;
  try {
    const __fitRequest = JSON.parse(__fitLine);
    // Frozen schema fixtures are loaded with readJson/JSON.parse before they
    // reach validatorFor.  Use that same authority path here.  The script's
    // separate parseStrictJson entrypoint deliberately creates null-prototype
    // objects; ajv's deep-equality helper cannot evaluate object-valued
    // const/enum constraints on those objects (it calls a missing valueOf).
    // Go's strict byte parser is exercised independently below and in the raw
    // entrypoint fixtures.
    const __fitValue = JSON.parse(__fitRequest.raw);
    const __fitValidate = validatorFor(__fitRequest.schema);
	    __fitResponse = {
	      valid: Boolean(__fitValidate(__fitValue)),
	      schema: __fitRequest.schema,
	      issues: (__fitValidate.errors || []).map((issue) => ({
	        keyword: issue.keyword,
	        instancePath: issue.instancePath,
	        schemaPath: issue.schemaPath,
	      })),
	    };
  } catch (__fitError) {
    __fitResponse = {
      valid: false,
      error: String(__fitError && __fitError.message || __fitError),
    };
  }
  process.stdout.write(JSON.stringify(__fitResponse) + "\n");
}
process.exit(0);

`
	source = strings.Replace(source, marker, protocol+marker, 1)
	tempScript := filepath.Join(t.TempDir(), "frozen-node-authority.mjs")
	if err := os.WriteFile(tempScript, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(program.nodePath, tempScript)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	runner := &nodeAuthorityRunner{
		command: command,
		stdin:   stdin,
		stdout:  bufio.NewScanner(stdoutPipe),
	}
	runner.stdout.Buffer(make([]byte, 4096), 1024*1024)
	command.Stderr = &runner.stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start frozen Node authority: %v", err)
	}
	if !runner.stdout.Scan() {
		_ = stdin.Close()
		_ = command.Wait()
		t.Fatalf("frozen Node authority did not become ready: %s", runner.stderr.String())
	}
	var ready nodeAuthorityResponse
	if err := json.Unmarshal(runner.stdout.Bytes(), &ready); err != nil || !ready.Ready {
		_ = stdin.Close()
		_ = command.Wait()
		t.Fatalf("frozen Node authority invalid ready response %q: %v; stderr=%s", runner.stdout.Text(), err, runner.stderr.String())
	}
	if ready.Unicode == "" || ready.Node == "" {
		t.Fatalf("frozen Node authority omitted runtime provenance")
	}
	t.Logf("frozen Node authority ready: node=%s unicode=%s sha256=%s", ready.Node, ready.Unicode, frozenNodeAuthoritySHA256)
	t.Cleanup(func() {
		_ = runner.stdin.Close()
		if err := runner.command.Wait(); err != nil {
			t.Errorf("frozen Node authority exit: %v; stderr=%s", err, runner.stderr.String())
		}
	})
	return runner
}

func assertNodePackageVersion(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frozen Node authority package %s: %v", path, err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode frozen Node authority package %s: %v", path, err)
	}
	if manifest.Version != want {
		t.Fatalf("frozen Node authority package %s version=%s want=%s", path, manifest.Version, want)
	}
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func (r *nodeAuthorityRunner) validateDetailed(schemaName string, raw []byte) (nodeAuthorityResponse, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	request, err := json.Marshal(map[string]string{"schema": schemaName, "raw": string(raw)})
	if err != nil {
		return nodeAuthorityResponse{}, err
	}
	if _, err := r.stdin.Write(append(request, '\n')); err != nil {
		return nodeAuthorityResponse{}, fmt.Errorf("write Node authority request: %w; stderr=%s", err, r.stderr.String())
	}
	if !r.stdout.Scan() {
		return nodeAuthorityResponse{}, fmt.Errorf("read Node authority response: %v; stderr=%s", r.stdout.Err(), r.stderr.String())
	}
	var response nodeAuthorityResponse
	if err := json.Unmarshal(r.stdout.Bytes(), &response); err != nil {
		return nodeAuthorityResponse{}, fmt.Errorf("decode Node authority response %q: %w", r.stdout.Text(), err)
	}
	if response.Error != "" {
		return nodeAuthorityResponse{}, fmt.Errorf("Node authority: %s", response.Error)
	}
	if response.SchemaName != schemaName {
		return nodeAuthorityResponse{}, fmt.Errorf("Node authority schema response=%q want=%q", response.SchemaName, schemaName)
	}
	return response, nil
}

func (r *nodeAuthorityRunner) validate(schemaName string, raw []byte) (bool, error) {
	response, err := r.validateDetailed(schemaName, raw)
	return response.Valid, err
}

func assertFrozenFixtureNodeParity(t *testing.T, valid, invalid []any) {
	t.Helper()
	authority := startNodeAuthority(t)
	for _, group := range []struct {
		name  string
		cases []any
		want  bool
	}{
		{name: "valid", cases: valid, want: true},
		{name: "invalid", cases: invalid, want: false},
	} {
		for _, rawCase := range group.cases {
			fixture := rawCase.(map[string]any)
			raw, err := json.Marshal(strictValue(t, fixture["value"]))
			if err != nil {
				t.Fatal(err)
			}
			got, err := authority.validate(fixture["schema"].(string), raw)
			if err != nil {
				t.Fatalf("Node %s fixture %q: %v", group.name, fixture["name"], err)
			}
			if got != group.want {
				t.Errorf("Node authority disagrees with frozen %s fixture %q: got valid=%v", group.name, fixture["name"], got)
			}
		}
	}
}

func TestTargetedFrozenAuthorityBoundaries(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	authority := startNodeAuthority(t)
	assertParity := func(schema string, value any, want bool) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		_, goErr := validator.Decode(schema, raw)
		goValid := goErr == nil
		nodeValid, nodeErr := authority.validate(schema, raw)
		if nodeErr != nil {
			t.Fatal(nodeErr)
		}
		if goValid != nodeValid || goValid != want {
			t.Fatalf("%s value=%s: Go=%v (%v), Node=%v, want=%v",
				schema, raw, goValid, goErr, nodeValid, want)
		}
	}

	enrollment := fixtureCase(t, "server-owned enrollment challenge state")
	enrollment["created_at"] = "2026-07-29T10:00:00.000Z"
	assertParity("EnrollmentChallengeState", enrollment, false)

	for _, test := range []struct {
		timestamp string
		valid     bool
	}{
		{"0000-02-29T23:59:60Z", true},
		{"1900-02-29T23:59:59Z", false},
		{"2000-02-29T23:59:59.000Z", true},
		{"2026-07-29T23:59:60Z", true},
		{"2026-07-29T24:00:00Z", false},
		{"2026-07-29T10:00:00.00Z", false},
	} {
		assertParity("Timestamp", test.timestamp, test.valid)
	}
}

func TestFrozenAuthorityExecutableHarness(t *testing.T) {
	program := loadFrozenNodeAuthorityProgram(t)
	const marker = "assert.equal(phase0Manifest.schema_version, acceptedPhase0ManifestVersion);"
	if strings.Count(program.source, marker) != 1 {
		t.Fatal("frozen full-authority completion marker changed")
	}
	const completion = `
const __fitSemanticCount = Object.values(scenarios)
  .filter((value) => Array.isArray(value))
  .reduce((count, value) => count + value.length, 0);
const __fitExecutableCount = Object.values(executableSecurity)
  .filter((value) => Array.isArray(value))
  .reduce((count, value) => count + value.length, 0);
console.log("FIT_FULL_AUTHORITY " + JSON.stringify({
  schemas: platformValidators.size + 2,
  valid: validFixtureSet.cases.length,
  invalid: invalidFixtureSet.cases.length,
  semantic: __fitSemanticCount,
  semanticSingletons: [
    "cross_route_throttle_case",
    "forwarding_header_case",
    "recovery_calculation_case",
  ].filter((key) => Object.hasOwn(scenarios, key)).length,
  executable: __fitExecutableCount,
  executableGroups: Object.keys(executableSecurity)
    .filter((key) => key !== "schema_version").length,
  linkages: linkageChains.chains.length,
  redactions: executableSecurity.websocket_redaction_cases.length,
  argon: argonVectorExecution,
}));
process.exit(0);

`
	source := strings.Replace(program.source, marker, completion+marker, 1)
	tempScript := filepath.Join(t.TempDir(), "frozen-node-full-authority.mjs")
	if err := os.WriteFile(tempScript, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(program.nodePath, tempScript)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("full frozen Node authority failed: %v\n%s", err, output)
	}
	const prefix = "FIT_FULL_AUTHORITY "
	var encoded string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, prefix) {
			encoded = strings.TrimPrefix(line, prefix)
		}
	}
	if encoded == "" {
		t.Fatalf("full frozen Node authority omitted completion report:\n%s", output)
	}
	var report struct {
		Schemas            int    `json:"schemas"`
		Valid              int    `json:"valid"`
		Invalid            int    `json:"invalid"`
		Semantic           int    `json:"semantic"`
		SemanticSingletons int    `json:"semanticSingletons"`
		Executable         int    `json:"executable"`
		ExecutableGroups   int    `json:"executableGroups"`
		Linkages           int    `json:"linkages"`
		Redactions         int    `json:"redactions"`
		Argon              string `json:"argon"`
	}
	if err := json.Unmarshal([]byte(encoded), &report); err != nil {
		t.Fatalf("decode full frozen Node authority report %q: %v", encoded, err)
	}
	if report.Schemas != 59 || report.Valid != 35 || report.Invalid != 28 ||
		report.Semantic != 88 || report.SemanticSingletons != 3 ||
		report.Executable != 49 || report.ExecutableGroups != 8 ||
		report.Linkages != 4 || report.Redactions != 3 {
		t.Fatalf("full frozen Node authority inventory drift: %+v", report)
	}
	if report.Argon != "VERIFIED_BY_NODE_CRYPTO_ARGON2" {
		t.Fatalf("full frozen Node authority did not recompute Argon2id: %s", report.Argon)
	}
	t.Logf("full frozen authority schemas=%d fixtures=%d/%d semantic=%d+%d executable=%d/%d linkages=%d Argon2=%s",
		report.Schemas, report.Valid, report.Invalid, report.Semantic,
		report.SemanticSingletons, report.Executable, report.ExecutableGroups,
		report.Linkages, report.Argon)
}

type mutationPathElement struct {
	key     string
	index   int
	isIndex bool
}

type contractMutation struct {
	fixture string
	schema  string
	path    string
	class   string
	value   any
	target  mutationTarget
}

type mutationTarget struct {
	document      string
	schemaPointer string
	keyword       string
}

func (target mutationTarget) key() string {
	if target.keyword == "" {
		return ""
	}
	return target.document + target.schemaPointer + "/" + target.keyword
}

func (target mutationTarget) schemaPathSuffix() string {
	if target.schemaPointer == "#" {
		return "#/" + target.keyword
	}
	return target.schemaPointer + "/" + target.keyword
}

func (target mutationTarget) matches(issue nodeAuthorityIssue) bool {
	if issue.Keyword != target.keyword {
		return false
	}
	candidates := []string{target.schemaPathSuffix()}
	const definitions = "#/$defs/"
	if strings.HasPrefix(target.schemaPointer, definitions) {
		remainder := strings.TrimPrefix(target.schemaPointer, definitions)
		if separator := strings.IndexByte(remainder, '/'); separator >= 0 {
			candidates = append(candidates, "#"+remainder[separator:]+"/"+target.keyword)
		} else {
			candidates = append(candidates, "#/"+target.keyword)
		}
	}
	for _, candidate := range candidates {
		if strings.HasSuffix(issue.SchemaPath, candidate) {
			return true
		}
	}
	return false
}

type mutationSchemaOccurrence struct {
	schema  map[string]any
	pointer string
}

func runRecursiveMutationParity(t *testing.T) {
	t.Helper()
	validator, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	authority := startNodeAuthority(t)
	fixtures := load(t, "fixtures/platform/valid-schema-cases-v1.json")["cases"].([]any)
	all := make([]contractMutation, 0, 4096)
	classCounts := map[string]int{}
	expectedTargets := map[string]mutationTarget{}
	coveredTargets := map[string]int{}
	baselineSchemas := map[string]int{}
	schemaBaselineValues := map[string]string{}
	baselinePasses := 0
	addCase := func(name, schemaName string, value any) {
		t.Helper()
		baseline, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := validator.DecodeTyped(schemaName, baseline); err != nil {
			t.Fatalf("Go baseline %s: %v", name, err)
		}
		nodeValid, err := authority.validate(schemaName, baseline)
		if err != nil || !nodeValid {
			t.Fatalf("Node baseline %s: valid=%v err=%v", name, nodeValid, err)
		}
		if _, exists := schemaBaselineValues[schemaName]; !exists {
			schemaBaselineValues[schemaName] = CanonicalJSON(value)
		}
		baselineSchemas[schemaName]++
		baselinePasses++
		docName, schema := validator.definitionDocument(schemaName)
		mutations, targets := generateContractMutations(validator, docName, schemaName, name, value, schema)
		if len(mutations) == 0 {
			t.Fatalf("no recursive mutations generated for %s", name)
		}
		all = append(all, mutations...)
		for key, target := range targets {
			expectedTargets[key] = target
		}
	}
	for _, rawFixture := range fixtures {
		fixture := rawFixture.(map[string]any)
		name := fixture["name"].(string)
		schemaName := fixture["schema"].(string)
		value := strictValue(t, fixture["value"])
		addCase(name, schemaName, value)
	}
	definitions := schemaMap(validator.platform["$defs"])
	schemaNames := make([]string, 0, len(definitions)+2)
	for name := range definitions {
		schemaNames = append(schemaNames, name)
	}
	schemaNames = append(schemaNames, "RecoveryEvidence", "RemediationProposal")
	sort.Strings(schemaNames)
	if len(schemaNames) != GeneratedSchemaTypeCount {
		t.Fatalf("recursive schema inventory=%d, generated schema type count=%d", len(schemaNames), GeneratedSchemaTypeCount)
	}
	candidates := generatedCandidateCorpus(t)
	for _, schemaName := range schemaNames {
		if baselineSchemas[schemaName] != 0 {
			continue
		}
		found := false
		docName, schema := validator.definitionDocument(schemaName)
		for index, candidate := range candidates {
			if validator.validate(schemaName, candidate) != nil {
				continue
			}
			mutations, _ := generateContractMutations(
				validator,
				docName,
				schemaName,
				"candidate preflight",
				candidate,
				schema,
			)
			if len(mutations) == 0 {
				continue
			}
			addCase(
				fmt.Sprintf("generated candidate %d for %s", index, schemaName),
				schemaName,
				cloneJSON(candidate),
			)
			found = true
			break
		}
		if !found {
			t.Fatalf("schema %s has no directly usable valid baseline in the complete frozen candidate corpus", schemaName)
		}
	}
	if len(baselineSchemas) != GeneratedSchemaTypeCount {
		t.Fatalf("recursive schema baselines=%d, want=%d", len(baselineSchemas), GeneratedSchemaTypeCount)
	}
	schemaBaselineKeys := make([]string, 0, len(schemaNames))
	for _, schemaName := range schemaNames {
		schemaBaselineKeys = append(schemaBaselineKeys, schemaName+"\x00"+schemaBaselineValues[schemaName])
	}
	schemaBaselineDigest := sha256Hex(strings.Join(schemaBaselineKeys, "\n"))
	const frozenSchemaBaselineDigest = "7331f057ac02733b50098eddd1041d5310ac16a8dfa924ca42b35543fdba9915"
	if baselinePasses != 65 || schemaBaselineDigest != frozenSchemaBaselineDigest {
		t.Errorf("recursive schema baseline drift: passes=%d schemas=%d digest=%s want passes=65 schemas=%d digest=%s",
			baselinePasses, len(baselineSchemas), schemaBaselineDigest, GeneratedSchemaTypeCount, frozenSchemaBaselineDigest)
	}
	sort.Slice(all, func(i, j int) bool {
		left := all[i].fixture + "\x00" + all[i].path + "\x00" + all[i].class + "\x00" + all[i].target.key() + "\x00" + CanonicalJSON(all[i].value)
		right := all[j].fixture + "\x00" + all[j].path + "\x00" + all[j].class + "\x00" + all[j].target.key() + "\x00" + CanonicalJSON(all[j].value)
		return left < right
	})
	for _, mutation := range all {
		raw, err := json.Marshal(mutation.value)
		if err != nil {
			t.Fatalf("%s %s %s marshal: %v", mutation.fixture, mutation.path, mutation.class, err)
		}
		_, goErr := validator.DecodeTyped(mutation.schema, raw)
		goValid := goErr == nil
		nodeResponse, err := authority.validateDetailed(mutation.schema, raw)
		if err != nil {
			t.Fatalf("%s %s %s authority: %v", mutation.fixture, mutation.path, mutation.class, err)
		}
		nodeValid := nodeResponse.Valid
		if goValid != nodeValid {
			t.Errorf("recursive mutation mismatch fixture=%q schema=%s path=%s class=%s Go=%v Node=%v GoError=%v raw=%s",
				mutation.fixture, mutation.schema, mutation.path, mutation.class, goValid, nodeValid, goErr, raw)
		}
		if key := mutation.target.key(); key != "" {
			matched := false
			for _, issue := range nodeResponse.Issues {
				if mutation.target.matches(issue) {
					matched = true
					break
				}
			}
			if matched {
				coveredTargets[key]++
			} else {
				t.Errorf("targeted mutation did not produce its pinned Node issue fixture=%q schema=%s path=%s class=%s target=%s issues=%v raw=%s",
					mutation.fixture, mutation.schema, mutation.path, mutation.class, key, nodeResponse.Issues, raw)
			}
		}
		classCounts[mutation.class]++
	}
	mutationKeys := make([]string, 0, len(all))
	for _, mutation := range all {
		mutationKeys = append(mutationKeys,
			mutation.fixture+"\x00"+
				mutation.schema+"\x00"+
				mutation.path+"\x00"+
				mutation.class+"\x00"+
				mutation.target.key()+"\x00"+
				CanonicalJSON(mutation.value),
		)
	}
	mutationDigest := sha256Hex(strings.Join(mutationKeys, "\n"))
	for _, required := range []string{
		"array-bound", "const", "contains", "digest-bound", "enum",
		"explicit-null", "missing", "numeric-bound", "string-bound",
		"unknown-field", "wrong-type", "x-fit",
	} {
		if classCounts[required] == 0 {
			t.Errorf("recursive mutation class %q executed zero cases", required)
		}
	}
	wantClassCounts := map[string]int{
		"array-bound":   37,
		"const":         166,
		"contains":      14,
		"digest-bound":  40,
		"enum":          96,
		"explicit-null": 713,
		"missing":       1379,
		"numeric-bound": 15,
		"string-bound":  672,
		"unknown-field": 292,
		"wrong-type":    1259,
		"x-fit":         45,
	}
	const frozenMutationDigest = "a7e11838c7e6ad4d0929d385c39ff2bb7600a09b2370bd24b55aed31d4717448"
	if len(all) != 4728 || len(classCounts) != len(wantClassCounts) || mutationDigest != frozenMutationDigest {
		t.Errorf("recursive mutation inventory drift: mutations=%d digest=%s classes=%v want mutations=4728 digest=%s",
			len(all), mutationDigest, classCounts, frozenMutationDigest)
	}
	for class, want := range wantClassCounts {
		if got := classCounts[class]; got != want {
			t.Errorf("recursive mutation class %q count=%d want=%d", class, got, want)
		}
	}
	for key := range expectedTargets {
		if coveredTargets[key] == 0 {
			t.Errorf("frozen constraint occurrence was not differentially exercised: %s", key)
		}
	}
	targetKeys := make([]string, 0, len(expectedTargets))
	targetKeywords := map[string]int{}
	for key, target := range expectedTargets {
		targetKeys = append(targetKeys, key)
		targetKeywords[target.keyword]++
	}
	sort.Strings(targetKeys)
	targetDigest := sha256Hex(strings.Join(targetKeys, "\n"))
	const frozenTargetDigest = "bcbf74d60c60b524c9ade03c7c4cb498fa95c2ddc36c6dc700854c30f9a61f79"
	if len(expectedTargets) != 469 || targetDigest != frozenTargetDigest {
		t.Errorf("targeted frozen constraint inventory drift: count=%d digest=%s want count=469 digest=%s",
			len(expectedTargets), targetDigest, frozenTargetDigest)
	}
	frozenXFit := frozenXFitMutationTargets(validator)
	if len(frozenXFit) != 20 {
		t.Errorf("frozen x-fit occurrence inventory=%d want=20", len(frozenXFit))
	}
	for key := range frozenXFit {
		if _, exists := expectedTargets[key]; !exists {
			t.Errorf("frozen x-fit occurrence has no recursive differential mutation: %s", key)
		}
	}
	for key, target := range expectedTargets {
		if strings.HasPrefix(target.keyword, "x-fit-") {
			if _, exists := frozenXFit[key]; !exists {
				t.Errorf("recursive differential mutation targets unknown x-fit occurrence: %s", key)
			}
		}
	}
	t.Logf("recursive differential mutations=%d mutation-digest=%s fixtures=%d baseline-passes=%d schema-baselines=%d schema-digest=%s classes=%v targeted-constraints=%d target-digest=%s keywords=%v",
		len(all), mutationDigest, len(fixtures), baselinePasses, len(baselineSchemas), schemaBaselineDigest, classCounts, len(coveredTargets), targetDigest, targetKeywords)
}

func frozenXFitMutationTargets(v *Validator) map[string]mutationTarget {
	result := map[string]mutationTarget{}
	for _, document := range []string{
		"schemas/platform-v1.schema.json",
		"schemas/remediation-proposal-v1.schema.json",
		"schemas/recovery-evidence-v1.schema.json",
	} {
		var walk func(map[string]any, string)
		walk = func(schema map[string]any, pointer string) {
			if schema == nil {
				return
			}
			for keyword := range schema {
				if strings.HasPrefix(keyword, "x-fit-") {
					target := mutationTarget{
						document:      document,
						schemaPointer: pointer,
						keyword:       keyword,
					}
					result[target.key()] = target
				}
			}
			for _, keyword := range []string{"$defs", "properties"} {
				for name, raw := range schemaMap(schema[keyword]) {
					walk(schemaMap(raw), pointer+"/"+keyword+"/"+escapeJSONPointer(name))
				}
			}
			for _, keyword := range []string{
				"propertyNames", "additionalProperties", "unevaluatedProperties",
				"items", "contains", "not", "if", "then", "else",
			} {
				if child := schemaMap(schema[keyword]); child != nil {
					walk(child, pointer+"/"+keyword)
				}
			}
			for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
				for index, raw := range slice(schema[keyword]) {
					walk(schemaMap(raw), fmt.Sprintf("%s/%s/%d", pointer, keyword, index))
				}
			}
		}
		walk(v.docs[document], "#")
	}
	return result
}

func generateContractMutations(v *Validator, docName, schemaName, fixtureName string, root any, rootSchema map[string]any) ([]contractMutation, map[string]mutationTarget) {
	result := make([]contractMutation, 0, 128)
	expectedTargets := map[string]mutationTarget{}
	seen := map[string]bool{}
	addWithTarget := func(path []mutationPathElement, displayPath, class string, replacement any, remove bool, target mutationTarget) {
		mutated, ok := mutateJSON(root, path, replacement, remove)
		if !ok {
			return
		}
		key := class + "\x00" + displayPath + "\x00" + target.key() + "\x00" + CanonicalJSON(mutated)
		if seen[key] {
			return
		}
		seen[key] = true
		result = append(result, contractMutation{
			fixture: fixtureName,
			schema:  schemaName,
			path:    displayPath,
			class:   class,
			value:   mutated,
			target:  target,
		})
	}
	add := func(path []mutationPathElement, displayPath, class string, replacement any, remove bool) {
		addWithTarget(path, displayPath, class, replacement, remove, mutationTarget{})
	}
	addTarget := func(path []mutationPathElement, displayPath, class string, replacement any, remove bool, occurrence mutationSchemaOccurrence, keyword string) {
		mutated, ok := mutateJSON(root, path, replacement, remove)
		if !ok {
			return
		}
		// A mutation aimed at one occurrence can leave its enclosing oneOf or
		// conditional valid through another branch. Keep that mutation in the
		// differential corpus, but only pin an issue that the enclosing Go
		// schema expects to reject. Either Go/Node decision disagreement still
		// fails when the mutation executes.
		if v.validate(schemaName, mutated) == nil {
			addWithTarget(path, displayPath, class, replacement, remove, mutationTarget{})
			return
		}
		target := mutationTarget{document: docName, schemaPointer: occurrence.pointer, keyword: keyword}
		expectedTargets[target.key()] = target
		addWithTarget(path, displayPath, class, replacement, remove, target)
	}
	var walk func(any, []mutationSchemaOccurrence, []mutationPathElement, string)
	walk = func(value any, schemas []mutationSchemaOccurrence, path []mutationPathElement, displayPath string) {
		expanded := applicableSchemas(v, docName, schemas, value, map[string]bool{})
		addConstraintMutations(v, docName, value, expanded, path, displayPath, addTarget)
		switch current := value.(type) {
		case map[string]any:
			unknown := cloneJSON(current).(map[string]any)
			unknown["__fit_contract_unknown__"] = true
			add(path, displayPath, "unknown-field", unknown, false)
			for _, occurrence := range expanded {
				for _, key := range strSlice(occurrence.schema["required"]) {
					if _, exists := current[key]; !exists {
						continue
					}
					childPath := appendPath(path, mutationPathElement{key: key})
					addTarget(childPath, displayPath+"."+key, "missing", nil, true, occurrence, "required")
				}
				if occurrence.schema["additionalProperties"] == false {
					addTarget(path, displayPath, "unknown-field", unknown, false, occurrence, "additionalProperties")
				}
				if occurrence.schema["unevaluatedProperties"] == false {
					addTarget(path, displayPath, "unknown-field", unknown, false, occurrence, "unevaluatedProperties")
				}
				if schemaMap(occurrence.schema["propertyNames"]) != nil {
					forbidden := cloneJSON(current).(map[string]any)
					forbidden["server_time"] = true
					addTarget(path, displayPath, "unknown-field", forbidden, false, occurrence, "propertyNames")
				}
			}
			keys := sortedKeys(current)
			for _, key := range keys {
				childPath := appendPath(path, mutationPathElement{key: key})
				childDisplay := displayPath + "." + key
				add(childPath, childDisplay, "missing", nil, true)
				if current[key] != nil {
					add(childPath, childDisplay, "explicit-null", nil, false)
				}
				add(childPath, childDisplay, "wrong-type", wrongType(current[key]), false)
				if text, ok := current[key].(string); ok && isDigestProperty(key) {
					add(childPath, childDisplay, "digest-bound", changedDigest(text), false)
				}
				childSchemas := propertySchemaSet(v, docName, expanded, key)
				walk(current[key], childSchemas, childPath, childDisplay)
			}
		case []any:
			if len(current) > 0 {
				add(path, displayPath, "array-bound", []any{}, false)
				shorter := cloneJSON(current).([]any)[:len(current)-1]
				add(path, displayPath, "array-bound", shorter, false)
				duplicate := cloneJSON(current).([]any)
				duplicate = append(duplicate, cloneJSON(current[0]))
				add(path, displayPath, "array-bound", duplicate, false)
			}
			itemSchemas := itemSchemaSet(v, docName, expanded)
			for index, item := range current {
				childPath := appendPath(path, mutationPathElement{index: index, isIndex: true})
				walk(item, itemSchemas, childPath, fmt.Sprintf("%s[%d]", displayPath, index))
			}
		}
	}
	rootPointer := "#"
	if docName == "schemas/platform-v1.schema.json" {
		rootPointer = "#/$defs/" + escapeJSONPointer(schemaName)
	}
	walk(root, []mutationSchemaOccurrence{{schema: rootSchema, pointer: rootPointer}}, nil, "$")
	return result, expectedTargets
}

func applicableSchemas(v *Validator, docName string, schemas []mutationSchemaOccurrence, value any, seenRefs map[string]bool) []mutationSchemaOccurrence {
	result := make([]mutationSchemaOccurrence, 0, len(schemas)*2)
	for _, occurrence := range schemas {
		schema := occurrence.schema
		if schema == nil {
			continue
		}
		result = append(result, occurrence)
		if ref, _ := schema["$ref"].(string); ref != "" && !seenRefs[ref] {
			nextSeen := cloneStringSet(seenRefs)
			nextSeen[ref] = true
			result = append(result, applicableSchemas(v, docName, []mutationSchemaOccurrence{{
				schema:  v.resolve(docName, schema),
				pointer: ref,
			}}, value, nextSeen)...)
		}
		for index, raw := range slice(schema["allOf"]) {
			result = append(result, applicableSchemas(v, docName, []mutationSchemaOccurrence{{
				schema:  schemaMap(raw),
				pointer: fmt.Sprintf("%s/allOf/%d", occurrence.pointer, index),
			}}, value, cloneStringSet(seenRefs))...)
		}
		for _, keyword := range []string{"anyOf", "oneOf"} {
			for index, raw := range slice(schema[keyword]) {
				child := schemaMap(raw)
				if child != nil && v.checkDocument(docName, child, value, "$mutation") == nil {
					result = append(result, applicableSchemas(v, docName, []mutationSchemaOccurrence{{
						schema:  child,
						pointer: fmt.Sprintf("%s/%s/%d", occurrence.pointer, keyword, index),
					}}, value, cloneStringSet(seenRefs))...)
				}
			}
		}
		if condition := schemaMap(schema["if"]); condition != nil {
			keyword := "else"
			if v.checkDocument(docName, condition, value, "$mutation") == nil {
				keyword = "then"
			}
			if child := schemaMap(schema[keyword]); child != nil {
				result = append(result, applicableSchemas(v, docName, []mutationSchemaOccurrence{{
					schema:  child,
					pointer: occurrence.pointer + "/" + keyword,
				}}, value, cloneStringSet(seenRefs))...)
			}
		}
	}
	return result
}

func propertySchemaSet(v *Validator, docName string, schemas []mutationSchemaOccurrence, key string) []mutationSchemaOccurrence {
	result := []mutationSchemaOccurrence{}
	for _, occurrence := range schemas {
		schema := occurrence.schema
		if child := schemaMap(schemaMap(schema["properties"])[key]); child != nil {
			result = append(result, mutationSchemaOccurrence{
				schema:  child,
				pointer: occurrence.pointer + "/properties/" + escapeJSONPointer(key),
			})
			continue
		}
		if child := schemaMap(schema["additionalProperties"]); child != nil {
			result = append(result, mutationSchemaOccurrence{
				schema:  child,
				pointer: occurrence.pointer + "/additionalProperties",
			})
		}
	}
	return result
}

func itemSchemaSet(v *Validator, docName string, schemas []mutationSchemaOccurrence) []mutationSchemaOccurrence {
	result := []mutationSchemaOccurrence{}
	for _, occurrence := range schemas {
		schema := occurrence.schema
		if child := schemaMap(schema["items"]); child != nil {
			result = append(result, mutationSchemaOccurrence{
				schema:  child,
				pointer: occurrence.pointer + "/items",
			})
		}
	}
	return result
}

func addConstraintMutations(
	v *Validator,
	docName string,
	value any,
	schemas []mutationSchemaOccurrence,
	path []mutationPathElement,
	displayPath string,
	addTarget func([]mutationPathElement, string, string, any, bool, mutationSchemaOccurrence, string),
) {
	for _, occurrence := range schemas {
		schema := occurrence.schema
		if _, exists := schema["type"]; exists &&
			!strings.Contains(occurrence.pointer, "JsonValue/oneOf/") {
			addTarget(path, displayPath, "wrong-type", wrongType(value), false, occurrence, "type")
		}
		if _, exists := schema["const"]; exists {
			addTarget(path, displayPath, "const", constDrift(value), false, occurrence, "const")
		}
		if _, exists := schema["enum"]; exists {
			addTarget(path, displayPath, "enum", enumDrift(value), false, occurrence, "enum")
		}
		if text, ok := value.(string); ok {
			if min, ok := schemaNonNegativeInteger(schema["minLength"]); ok {
				length := int(min) - 1
				if length < 0 {
					length = 0
				}
				addTarget(path, displayPath, "string-bound", repeatFirstRune(text, length), false, occurrence, "minLength")
			}
			if max, ok := schemaNonNegativeInteger(schema["maxLength"]); ok && max < 10000 {
				addTarget(path, displayPath, "string-bound", text+strings.Repeat("a", int(max)+1-len([]rune(text))), false, occurrence, "maxLength")
			}
			if _, exists := schema["pattern"]; exists {
				addTarget(path, displayPath, "string-bound", patternDrift(text), false, occurrence, "pattern")
			}
			if _, exists := schema["format"]; exists {
				addTarget(path, displayPath, "string-bound", formatDrift(text), false, occurrence, "format")
			}
		}
		if _, ok := value.(json.Number); ok {
			for _, keyword := range []string{"minimum", "exclusiveMinimum"} {
				if bound, ok := binary64(schema[keyword]); ok {
					replacement := bound - 1
					if keyword == "exclusiveMinimum" {
						replacement = bound
					}
					addTarget(path, displayPath, "numeric-bound", json.Number(strconv.FormatFloat(replacement, 'g', -1, 64)), false, occurrence, keyword)
				}
			}
			for _, keyword := range []string{"maximum", "exclusiveMaximum"} {
				if bound, ok := binary64(schema[keyword]); ok {
					replacement := bound + 1
					if keyword == "exclusiveMaximum" {
						replacement = bound
					}
					addTarget(path, displayPath, "numeric-bound", json.Number(strconv.FormatFloat(replacement, 'g', -1, 64)), false, occurrence, keyword)
				}
			}
			if multiple, ok := binary64(schema["multipleOf"]); ok {
				addTarget(path, displayPath, "numeric-bound", json.Number(strconv.FormatFloat(multiple/2, 'g', -1, 64)), false, occurrence, "multipleOf")
			}
		}
		if array, ok := value.([]any); ok {
			if min, ok := schemaNonNegativeInteger(schema["minItems"]); ok && int64(len(array)) >= min && min > 0 {
				addTarget(path, displayPath, "array-bound", cloneJSON(array[:min-1]), false, occurrence, "minItems")
			}
			if max, ok := schemaNonNegativeInteger(schema["maxItems"]); ok && max < 1000 {
				expanded := cloneJSON(array).([]any)
				for int64(len(expanded)) <= max {
					var repeated any = nil
					if len(array) > 0 {
						repeated = cloneJSON(array[len(array)-1])
					}
					expanded = append(expanded, repeated)
				}
				addTarget(path, displayPath, "array-bound", expanded, false, occurrence, "maxItems")
			}
			if schema["uniqueItems"] == true && len(array) > 0 {
				duplicate := cloneJSON(array).([]any)
				duplicate = append(duplicate, cloneJSON(array[0]))
				addTarget(path, displayPath, "array-bound", duplicate, false, occurrence, "uniqueItems")
			}
			if contains := schemaMap(schema["contains"]); contains != nil {
				filtered := []any{}
				matching := []any{}
				for _, item := range array {
					if v.checkDocument(docName, contains, item, "$contains") != nil {
						filtered = append(filtered, cloneJSON(item))
					} else {
						matching = append(matching, cloneJSON(item))
					}
				}
				addTarget(path, displayPath, "contains", filtered, false, occurrence, "contains")
				if max, ok := schemaNonNegativeInteger(schema["maxContains"]); ok && len(matching) > 0 {
					overmatching := cloneJSON(array).([]any)
					for int64(len(matching)) <= max {
						item := cloneJSON(matching[0])
						overmatching = append(overmatching, item)
						matching = append(matching, item)
					}
					addTarget(path, displayPath, "contains", overmatching, false, occurrence, "contains")
				}
			}
		}
		if object, ok := value.(map[string]any); ok {
			if min, ok := schemaNonNegativeInteger(schema["minProperties"]); ok && min > 0 {
				reduced := cloneJSON(object).(map[string]any)
				for _, key := range sortedKeys(reduced) {
					if int64(len(reduced)) < min {
						break
					}
					delete(reduced, key)
				}
				addTarget(path, displayPath, "unknown-field", reduced, false, occurrence, "minProperties")
			}
			if max, ok := schemaNonNegativeInteger(schema["maxProperties"]); ok && max < 1000 {
				expanded := cloneJSON(object).(map[string]any)
				for index := 0; int64(len(expanded)) <= max; index++ {
					expanded[fmt.Sprintf("__fit_property_%d", index)] = true
				}
				addTarget(path, displayPath, "unknown-field", expanded, false, occurrence, "maxProperties")
			}
		}
		for keyword := range schema {
			if !strings.HasPrefix(keyword, "x-fit-") {
				continue
			}
			if replacement, ok := xFitDrift(value, keyword, schema); ok {
				addTarget(path, displayPath, "x-fit", replacement, false, occurrence, keyword)
			}
		}
	}
}

func xFitDrift(value any, keyword string, schema map[string]any) (any, bool) {
	object, ok := cloneJSON(value).(map[string]any)
	if !ok {
		return nil, false
	}
	switch keyword {
	case "x-fit-time-order":
		for _, raw := range slice(schema[keyword]) {
			rule := schemaMap(raw)
			start, _ := rule["start"].(string)
			end, _ := rule["end"].(string)
			if _, exists := object[start]; exists {
				object[end] = "0000-01-01T00:00:00Z"
				return object, true
			}
		}
	case "x-fit-time-window":
		rule := schemaMap(schema[keyword])
		start, _ := rule["start"].(string)
		end, _ := rule["end"].(string)
		if startValue, exists := object[start]; exists {
			object[end] = cloneJSON(startValue)
			return object, true
		}
	case "x-fit-refresh-window", "x-fit-refresh-family-lineage":
		object["family_deadline"] = "1970-01-01T00:00:00Z"
		return object, true
	case "x-fit-enrollment-challenge-state":
		object["subject_handle_digest"] = strings.Repeat("0", 64)
		return object, true
	case "x-fit-request-digest":
		object["canonical_request_digest"] = strings.Repeat("0", 64)
		return object, true
	case "x-fit-websocket-deadline":
		object["deadline"] = "1970-01-01T00:00:00Z"
		return object, true
	case "x-fit-event-payload":
		object["event_kind"] = "FIT_DRIFT"
		return object, true
	case "x-fit-payload-integrity":
		object["payload_digest"] = strings.Repeat("0", 64)
		return object, true
	case "x-fit-outbox-causality":
		object["created_at"] = "1970-01-01T00:00:00Z"
		return object, true
	case "x-fit-aggregate-history":
		if events, ok := object["events"].([]any); ok && len(events) > 0 {
			first := schemaMap(events[0])
			first["aggregate_id"] = "ffffffff-ffff-4fff-8fff-ffffffffffff"
			return object, true
		}
	case "x-fit-durable-record-digest":
		object["payload_digest"] = strings.Repeat("0", 64)
		return object, true
	case "x-fit-recovery-consistency":
		object["observed_rto_ms"] = json.Number("1")
		return object, true
	case "x-fit-authority-field-names":
		object["ｓｅｒｖｅｒ＿ｔｉｍｅ"] = true
		return object, true
	case "x-fit-model-authority-fields":
		object["ｃｏｎｆｉｒｍａｔｉｏｎ＿ｉｄ"] = true
		return object, true
	}
	return nil, false
}

func mutateJSON(root any, path []mutationPathElement, replacement any, remove bool) (any, bool) {
	if len(path) == 0 {
		if remove {
			return nil, false
		}
		return cloneJSON(replacement), true
	}
	result := cloneJSON(root)
	var current any = result
	for _, element := range path[:len(path)-1] {
		if element.isIndex {
			array, ok := current.([]any)
			if !ok || element.index < 0 || element.index >= len(array) {
				return nil, false
			}
			current = array[element.index]
		} else {
			object, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			current = object[element.key]
		}
	}
	last := path[len(path)-1]
	if last.isIndex {
		array, ok := current.([]any)
		if !ok || last.index < 0 || last.index >= len(array) {
			return nil, false
		}
		if remove {
			array = append(array[:last.index], array[last.index+1:]...)
			if len(path) == 1 {
				result = array
			} else if !assignJSON(result, path[:len(path)-1], array) {
				return nil, false
			}
		} else {
			array[last.index] = cloneJSON(replacement)
		}
		return result, true
	}
	object, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}
	if remove {
		if _, exists := object[last.key]; !exists {
			return nil, false
		}
		delete(object, last.key)
	} else {
		object[last.key] = cloneJSON(replacement)
	}
	return result, true
}

func assignJSON(root any, path []mutationPathElement, replacement any) bool {
	if len(path) == 0 {
		return false
	}
	var current any = root
	for _, element := range path[:len(path)-1] {
		if element.isIndex {
			current = current.([]any)[element.index]
		} else {
			current = current.(map[string]any)[element.key]
		}
	}
	last := path[len(path)-1]
	if last.isIndex {
		current.([]any)[last.index] = replacement
	} else {
		current.(map[string]any)[last.key] = replacement
	}
	return true
}

func appendPath(path []mutationPathElement, element mutationPathElement) []mutationPathElement {
	out := make([]mutationPathElement, len(path)+1)
	copy(out, path)
	out[len(path)] = element
	return out
}

func cloneJSON(value any) any {
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, item := range current {
			out[key] = cloneJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for index, item := range current {
			out[index] = cloneJSON(item)
		}
		return out
	default:
		return current
	}
}

func wrongType(value any) any {
	switch value.(type) {
	case string:
		return json.Number("1")
	case json.Number:
		return "1"
	case bool:
		return "true"
	case map[string]any:
		return []any{}
	case []any:
		return map[string]any{}
	case nil:
		return true
	default:
		return nil
	}
}

func constDrift(value any) any {
	switch current := value.(type) {
	case string:
		return current + "__FIT_CONST_DRIFT__"
	case json.Number:
		return json.Number("9007199254740991")
	case bool:
		return !current
	case nil:
		return true
	default:
		return wrongType(value)
	}
}

func enumDrift(value any) any {
	switch value.(type) {
	case string:
		return "__FIT_ENUM_DRIFT__"
	case json.Number:
		return json.Number("9007199254740991")
	case bool:
		return "not-a-boolean"
	case nil:
		return true
	default:
		return wrongType(value)
	}
}

func repeatFirstRune(value string, count int) string {
	if count <= 0 {
		return ""
	}
	first := 'x'
	for _, current := range value {
		first = current
		break
	}
	return strings.Repeat(string(first), count)
}

func patternDrift(value string) string {
	return value + "\n__FIT_PATTERN_DRIFT__"
}

func formatDrift(value string) string {
	if strings.Contains(value, "T") {
		return "2026-02-30T00:00:00Z"
	}
	return "not-a-valid-format"
}

func isDigestProperty(key string) bool {
	return strings.Contains(key, "digest") || strings.Contains(key, "_hash") || strings.HasSuffix(key, "sha256")
}

func changedDigest(value string) string {
	if value == "" {
		return "0"
	}
	first := byte('0')
	if value[0] == first {
		first = '1'
	}
	return string(first) + value[1:]
}
