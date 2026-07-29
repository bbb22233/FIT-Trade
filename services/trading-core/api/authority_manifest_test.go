package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type authorityManifest struct {
	ServerOwnedFields []string `json:"server_owned_fields"`
}

type securityManifest struct {
	ServerAuthoredIdentity struct {
		Fields []string `json:"fields"`
	} `json:"server_authored_identity"`
}

func TestFrozenAuthorityManifestParityAndAliases(t *testing.T) {
	transaction := authorityManifest{}
	readFrozenJSON(t, "transaction-boundaries-v1.json", &transaction)
	security := securityManifest{}
	readFrozenJSON(t, "security-values-v1.json", &security)

	if len(transaction.ServerOwnedFields) != 18 {
		t.Fatalf("frozen server_owned_fields count = %d, want 18", len(transaction.ServerOwnedFields))
	}
	if !sameStrings(transaction.ServerOwnedFields, mapKeys(serverOwnedFields)) {
		t.Fatalf("production server-owned fields differ from frozen manifest: manifest=%v production=%v", sorted(transaction.ServerOwnedFields), sorted(mapKeys(serverOwnedFields)))
	}
	if !sameStrings(transaction.ServerOwnedFields, security.ServerAuthoredIdentity.Fields) {
		t.Fatalf("security server_authored_identity differs from transaction manifest: transaction=%v security=%v", sorted(transaction.ServerOwnedFields), sorted(security.ServerAuthoredIdentity.Fields))
	}

	values := []string{"null", "true", "7", `"client"`, `{}`, `[]`}
	for _, field := range transaction.ServerOwnedFields {
		for _, alias := range authorityAliasMatrix(field) {
			t.Run(field+"/"+alias, func(t *testing.T) {
				for _, value := range values {
					for _, raw := range [][]byte{
						[]byte(`{"` + alias + `":` + value + `}`),
						[]byte(`{"outer":[{"inner":{"` + alias + `":` + value + `}}]}`),
						[]byte(`[{"outer":{"` + alias + `":` + value + `}}]`),
					} {
						if !hasAuthorityField(raw) {
							t.Fatalf("alias %q with value %s bypassed recursive authority rejection", alias, value)
						}
					}
				}
			})
		}
	}
}

func TestAuthorityLegalNearMissesRemainEndpointDependent(t *testing.T) {
	for _, field := range []string{"scope", "account_id", "authorization", "created_at", "family", "signature_verified_client"} {
		raw := []byte(`{"payload":{"` + field + `":"client"}}`)
		if hasAuthorityField(raw) {
			t.Fatalf("nonmanifest client field %q was authority-rejected", field)
		}
		request := newJSONRequest(raw)
		var destination struct {
			Payload json.RawMessage `json:"payload"`
		}
		if !decodeMutation(httptest.NewRecorder(), request, &destination) {
			t.Fatalf("generic client payload containing %q was rejected", field)
		}
	}

	request := newJSONRequest([]byte(`{"scope":"client","payload":{}}`))
	var destination struct {
		Payload json.RawMessage `json:"payload"`
	}
	if decodeMutation(httptest.NewRecorder(), request, &destination) {
		t.Fatal("endpoint unknown root field scope was accepted")
	}
}

func authorityAliasMatrix(field string) []string {
	parts := strings.Split(field, "_")
	camel := parts[0]
	pascal := ""
	for _, part := range parts {
		pascal += strings.ToUpper(part[:1]) + part[1:]
	}
	for _, part := range parts[1:] {
		camel += strings.ToUpper(part[:1]) + part[1:]
	}
	aliases := []string{field, strings.ToUpper(field), camel, pascal, strings.ReplaceAll(field, "_", "-"), strings.ReplaceAll(field, "_", ""), strings.ReplaceAll(field, "_", "__")}
	for index := range field {
		if field[index] >= 'a' && field[index] <= 'z' {
			aliases = append(aliases, field[:index]+strings.ToUpper(field[index:index+1])+field[index+1:])
		}
	}
	return uniqueStrings(aliases)
}

func newJSONRequest(raw []byte) *http.Request {
	request := httptest.NewRequest("POST", "http://loopback.test/ignored", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func readFrozenJSON(t *testing.T, name string, destination any) {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(directory, "contracts", "platform", "manifests", name)
		if raw, err := os.ReadFile(candidate); err == nil {
			if err := json.Unmarshal(raw, destination); err != nil {
				t.Fatal(err)
			}
			return
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("could not locate frozen manifest %s", name)
		}
		directory = parent
	}
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	return keys
}
func sameStrings(left, right []string) bool {
	return strings.Join(sorted(left), "\x00") == strings.Join(sorted(right), "\x00")
}
func sorted(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}
func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, found := seen[value]; !found {
			seen[value] = struct{}{}
			unique = append(unique, value)
		}
	}
	return unique
}
