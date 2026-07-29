// Command gentypes generates the sealed, field-level Go contract and
// immutable manifest views from the byte-frozen platform inputs. It performs
// no network access.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var schemaFiles = []schemaFile{
	{
		key:          "schemas/platform-v1.schema.json",
		filename:     "schemas/platform-v1.schema.json",
		rootName:     "",
		localPrefix:  "",
		platformDefs: true,
	},
	{
		key:         "schemas/remediation-proposal-v1.schema.json",
		filename:    "schemas/remediation-proposal-v1.schema.json",
		rootName:    "RemediationProposal",
		localPrefix: "Remediation",
	},
	{
		key:         "schemas/recovery-evidence-v1.schema.json",
		filename:    "schemas/recovery-evidence-v1.schema.json",
		rootName:    "RecoveryEvidence",
		localPrefix: "Recovery",
	},
}

var manifestFiles = []manifestFile{
	{key: "failure-injection", filename: "manifests/failure-injection-v1.json", goName: "FailureInjectionManifest"},
	{key: "http-rate-limits", filename: "manifests/http-rate-limits-v1.json", goName: "HTTPRateLimitsManifest"},
	{key: "migration-policy", filename: "manifests/migration-policy-v1.json", goName: "MigrationPolicyManifest"},
	{key: "nats-permissions", filename: "manifests/nats-permissions-v1.json", goName: "NATSPermissionsManifest"},
	{key: "phase0-file-manifest", filename: "manifests/phase0-file-manifest-v1.json", goName: "Phase0FileManifest"},
	{key: "recovery-policy", filename: "manifests/recovery-policy-v1.json", goName: "RecoveryPolicyManifest"},
	{key: "security-values", filename: "manifests/security-values-v1.json", goName: "SecurityValuesManifest"},
	{key: "state-machines", filename: "manifests/state-machines-v1.json", goName: "StateMachinesManifest"},
	{key: "transaction-boundaries", filename: "manifests/transaction-boundaries-v1.json", goName: "TransactionBoundariesManifest"},
	{key: "websocket-protocol", filename: "manifests/websocket-protocol-v1.json", goName: "WebSocketProtocolManifest"},
}

func main() {
	check := flag.Bool("check", false, "verify generated output is current")
	rootFlag := flag.String("root", "", "override contracts/platform input directory")
	outputFlag := flag.String("output", "", "override platformcontract output directory")
	flag.Parse()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("gentypes: cannot locate generator source")
	}
	sourceDir := filepath.Dir(sourceFile)
	root := *rootFlag
	if root == "" {
		root = filepath.Clean(filepath.Join(sourceDir, "../../../../../contracts/platform"))
	}
	output := *outputFlag
	if output == "" {
		output = filepath.Clean(filepath.Join(sourceDir, "../.."))
	}

	generator, err := loadGenerator(root)
	if err != nil {
		panic(err)
	}
	outputs := map[string][]byte{}
	contractSource, err := generator.renderContracts()
	if err != nil {
		panic(err)
	}
	manifestSource, err := generator.renderManifests()
	if err != nil {
		panic(err)
	}
	outputs["contract_types_gen.go"] = formatted(contractSource)
	outputs["manifest_types_gen.go"] = formatted(manifestSource)

	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := filepath.Join(output, name)
		if *check {
			current, err := os.ReadFile(target)
			if err != nil {
				panic(fmt.Sprintf("gentypes: read %s: %v", target, err))
			}
			if !bytes.Equal(current, outputs[name]) {
				panic(fmt.Sprintf("gentypes: %s is stale; run go run ./cmd/gentypes", name))
			}
			continue
		}
		if err := os.WriteFile(target, outputs[name], 0o644); err != nil {
			panic(err)
		}
	}
}

func formatted(source []byte) []byte {
	result, err := format.Source(source)
	if err != nil {
		lines := strings.Split(string(source), "\n")
		for index, line := range lines {
			fmt.Fprintf(os.Stderr, "%5d %s\n", index+1, line)
		}
		panic(fmt.Sprintf("gentypes: format generated source: %v", err))
	}
	return result
}

func decodeJSONFile(filename string) (map[string]any, []byte, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, nil, fmt.Errorf("%s: trailing JSON value", filename)
	}
	return value, raw, nil
}

func inputDigest(inputs map[string][]byte) string {
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		_, _ = hash.Write([]byte(key))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(inputs[key])
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
