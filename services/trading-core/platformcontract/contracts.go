package platformcontract

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Validator constructs sealed named values from strict raw JSON. Intermediate
// maps are package-internal and never form a public validated representation.
type Validator struct {
	docs     map[string]map[string]any
	platform map[string]any
}

// NewValidator loads only generated, byte-pinned contract data.
func NewValidator() (*Validator, error) {
	docs := map[string]map[string]any{}
	for n, b64 := range frozenContractBase64 {
		b, e := base64.StdEncoding.DecodeString(b64)
		if e != nil {
			return nil, e
		}
		var d map[string]any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if e = dec.Decode(&d); e != nil {
			return nil, e
		}
		docs[n] = d
	}
	p := docs["schemas/platform-v1.schema.json"]
	if p == nil {
		return nil, fmt.Errorf("platformcontract: frozen platform schema missing")
	}
	v := &Validator{docs: docs, platform: p}
	for _, name := range []string{
		"schemas/platform-v1.schema.json",
		"schemas/remediation-proposal-v1.schema.json",
		"schemas/recovery-evidence-v1.schema.json",
	} {
		if err := v.auditDocument(name); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// Decode is the untrusted transport construction entry point. It returns only
// a sealed named contract value after strict parsing and complete validation;
// it never exposes the intermediate map as a validated representation.
func (v *Validator) Decode(name string, raw []byte) (TypedContract, error) {
	return v.DecodeTyped(name, raw)
}
func (v *Validator) validate(name string, value any) error {
	doc, schema := v.definitionDocument(name)
	if schema == nil {
		return fmt.Errorf("platformcontract: unknown schema %q", name)
	}
	if e := v.checkDocument(doc, schema, value, "$"); e != nil {
		return e
	}
	return v.semantic(name, value)
}
func (v *Validator) definition(name string) map[string]any {
	_, schema := v.definitionDocument(name)
	return schema
}
func (v *Validator) definitionDocument(name string) (string, map[string]any) {
	if name == "RemediationProposal" {
		const doc = "schemas/remediation-proposal-v1.schema.json"
		return doc, v.docs[doc]
	}
	if name == "RecoveryEvidence" {
		const doc = "schemas/recovery-evidence-v1.schema.json"
		return doc, v.docs[doc]
	}
	d, _ := v.platform["$defs"].(map[string]any)
	x, _ := d[name].(map[string]any)
	return "schemas/platform-v1.schema.json", x
}
func (v *Validator) resolve(docName string, s map[string]any) map[string]any {
	r, _ := s["$ref"].(string)
	if r == "" {
		return s
	}
	const p = "#/$defs/"
	if strings.HasPrefix(r, p) {
		name := strings.TrimPrefix(r, p)
		doc := v.docs[docName]
		return schemaMap(schemaMap(doc["$defs"])[name])
	}
	return nil
}
func schemaMap(x any) map[string]any { m, _ := x.(map[string]any); return m }
func slice(x any) []any              { a, _ := x.([]any); return a }
func strSlice(x any) []string {
	a := slice(x)
	o := make([]string, 0, len(a))
	for _, x := range a {
		if s, ok := x.(string); ok {
			o = append(o, s)
		}
	}
	return o
}
func (v *Validator) check(schema map[string]any, value any, path string) error {
	return v.checkDocument("schemas/platform-v1.schema.json", schema, value, path)
}
func (v *Validator) checkDocument(docName string, schema map[string]any, value any, path string) error {
	if schema == nil {
		return fmt.Errorf("%s: invalid schema", path)
	}
	if e := knownKeywords(schema, path); e != nil {
		return e
	}
	// A $ref may have sibling constraints in draft 2020-12.  Validate the
	// target first, then continue with the local keywords rather than dropping
	// those siblings by replacing the complete schema map.
	if _, hasRef := schema["$ref"]; hasRef {
		target := v.resolve(docName, schema)
		if target == nil {
			return fmt.Errorf("%s: unresolved schema", path)
		}
		if e := v.checkDocument(docName, target, value, path); e != nil {
			return e
		}
		// Keep $ref on the local schema after validating its target:
		// evaluatedProperties must retain the target's annotations when a
		// sibling unevaluatedProperties keyword is present.
	}
	if x, ok := schema["type"]; ok {
		if !matchesType(x, value) {
			return fmt.Errorf("%s: expected %v", path, x)
		}
	}
	if c, ok := schema["const"]; ok && !equalJSON(c, value) {
		return fmt.Errorf("%s: const mismatch", path)
	}
	if es, ok := schema["enum"]; ok {
		found := false
		for _, e := range slice(es) {
			if equalJSON(e, value) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s: enum mismatch", path)
		}
	}
	if s, ok := value.(string); ok {
		if n, ok := numberInt(schema["minLength"]); ok && int64(len([]rune(s))) < n {
			return fmt.Errorf("%s: short string", path)
		}
		if n, ok := numberInt(schema["maxLength"]); ok && int64(len([]rune(s))) > n {
			return fmt.Errorf("%s: long string", path)
		}
		if p, ok := schema["pattern"].(string); ok {
			r, e := regexp.Compile(p)
			if e != nil || !r.MatchString(s) {
				return fmt.Errorf("%s: pattern mismatch", path)
			}
		}
		if f, _ := schema["format"].(string); f != "" && !validFormat(f, s) {
			return fmt.Errorf("%s: invalid %s", path, f)
		}
	}
	if _, ok := value.(json.Number); ok {
		if e := checkNumberBounds(schema, value, path); e != nil {
			return e
		}
	}
	if m, ok := value.(map[string]any); ok {
		if n, ok := numberInt(schema["minProperties"]); ok && int64(len(m)) < n {
			return fmt.Errorf("%s: too few properties", path)
		}
		if n, ok := numberInt(schema["maxProperties"]); ok && int64(len(m)) > n {
			return fmt.Errorf("%s: too many properties", path)
		}
		for _, k := range strSlice(schema["required"]) {
			if _, ok := m[k]; !ok {
				return fmt.Errorf("%s: missing %s", path, k)
			}
		}
		props := schemaMap(schema["properties"])
		if ap, exists := schema["additionalProperties"]; exists && ap == false {
			for k := range m {
				if _, ok := props[k]; !ok {
					return fmt.Errorf("%s: unknown field %s", path, k)
				}
			}
		}
		if pn := schemaMap(schema["propertyNames"]); pn != nil {
			for k := range m {
				if e := v.checkDocument(docName, pn, k, path+".<property>"); e != nil {
					return e
				}
			}
		}
		for k, sub := range props {
			if x, ok := m[k]; ok {
				if e := v.checkDocument(docName, schemaMap(sub), x, path+"."+k); e != nil {
					return e
				}
			}
		}
		if ap, exists := schema["additionalProperties"]; exists {
			if sub := schemaMap(ap); sub != nil {
				for k, x := range m {
					if _, declared := props[k]; !declared {
						if e := v.checkDocument(docName, sub, x, path+"."+k); e != nil {
							return e
						}
					}
				}
			}
		}
	}
	if a, ok := value.([]any); ok {
		if n, ok := numberInt(schema["minItems"]); ok && int64(len(a)) < n {
			return fmt.Errorf("%s: too few items", path)
		}
		if n, ok := numberInt(schema["maxItems"]); ok && int64(len(a)) > n {
			return fmt.Errorf("%s: too many items", path)
		}
		if item := schemaMap(schema["items"]); item != nil {
			for i, x := range a {
				if e := v.checkDocument(docName, item, x, fmt.Sprintf("%s[%d]", path, i)); e != nil {
					return e
				}
			}
		}
		if schema["uniqueItems"] == true {
			for i := range a {
				for j := 0; j < i; j++ {
					if equalJSON(a[i], a[j]) {
						return fmt.Errorf("%s: duplicate item", path)
					}
				}
			}
		}
		if contains := schemaMap(schema["contains"]); contains != nil {
			count := int64(0)
			for _, x := range a {
				if v.checkDocument(docName, contains, x, path+"[contains]") == nil {
					count++
				}
			}
			min, ok := numberInt(schema["minContains"])
			if !ok {
				min = 1
			}
			max, hasMax := numberInt(schema["maxContains"])
			if count < min || (hasMax && count > max) {
				return fmt.Errorf("%s: contains bounds", path)
			}
		}
	}
	for _, a := range slice(schema["allOf"]) {
		if e := v.checkDocument(docName, schemaMap(a), value, path); e != nil {
			return e
		}
	}
	if any := slice(schema["anyOf"]); len(any) > 0 {
		ok := false
		for _, a := range any {
			if v.checkDocument(docName, schemaMap(a), value, path) == nil {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s: no anyOf branch", path)
		}
	}
	if one := slice(schema["oneOf"]); len(one) > 0 {
		n := 0
		for _, a := range one {
			if v.checkDocument(docName, schemaMap(a), value, path) == nil {
				n++
			}
		}
		if n != 1 {
			return fmt.Errorf("%s: expected exactly one branch", path)
		}
	}
	if not := schemaMap(schema["not"]); not != nil && v.checkDocument(docName, not, value, path) == nil {
		return fmt.Errorf("%s: prohibited branch", path)
	}
	if iff := schemaMap(schema["if"]); iff != nil {
		if v.checkDocument(docName, iff, value, path) == nil {
			if then := schemaMap(schema["then"]); then != nil {
				if e := v.checkDocument(docName, then, value, path); e != nil {
					return e
				}
			}
		} else if els := schemaMap(schema["else"]); els != nil {
			if e := v.checkDocument(docName, els, value, path); e != nil {
				return e
			}
		}
	}
	if m, ok := value.(map[string]any); ok {
		if raw, exists := schema["unevaluatedProperties"]; exists {
			evaluated := v.evaluatedProperties(docName, schema, m, map[string]bool{})
			for key, item := range m {
				if evaluated[key] {
					continue
				}
				switch rule := raw.(type) {
				case bool:
					if !rule {
						return fmt.Errorf("%s: unevaluated field %s", path, key)
					}
				case map[string]any:
					if err := v.checkDocument(docName, rule, item, path+"."+key); err != nil {
						return err
					}
				default:
					return fmt.Errorf("%s: invalid unevaluatedProperties rule", path)
				}
			}
		}
	}
	return v.checkFit(schema, value, path)
}

// evaluatedProperties computes draft-2020-12 object annotations from the
// subschemas that actually succeeded. It is intentionally separate from a
// declaration inventory: properties in losing union branches or an unselected
// conditional branch do not make an unknown field evaluated.
func (v *Validator) evaluatedProperties(docName string, schema map[string]any, value map[string]any, seen map[string]bool) map[string]bool {
	out := map[string]bool{}
	merge := func(other map[string]bool) {
		for key := range other {
			out[key] = true
		}
	}
	if ref, _ := schema["$ref"].(string); ref != "" && !seen[ref] {
		nextSeen := cloneStringSet(seen)
		nextSeen[ref] = true
		if target := v.resolve(docName, schema); target != nil {
			merge(v.evaluatedProperties(docName, target, value, nextSeen))
		}
	}
	properties := schemaMap(schema["properties"])
	for key := range properties {
		if _, exists := value[key]; exists {
			out[key] = true
		}
	}
	if raw, exists := schema["additionalProperties"]; exists {
		for key := range value {
			if _, declared := properties[key]; declared {
				continue
			}
			switch rule := raw.(type) {
			case bool:
				if rule {
					out[key] = true
				}
			case map[string]any:
				if v.checkDocument(docName, rule, value[key], "$annotation."+key) == nil {
					out[key] = true
				}
			}
		}
	}
	for _, raw := range slice(schema["allOf"]) {
		child := schemaMap(raw)
		if child != nil {
			merge(v.evaluatedProperties(docName, child, value, cloneStringSet(seen)))
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		for _, raw := range slice(schema[keyword]) {
			child := schemaMap(raw)
			if child != nil && v.checkDocument(docName, child, value, "$annotation") == nil {
				merge(v.evaluatedProperties(docName, child, value, cloneStringSet(seen)))
			}
		}
	}
	if condition := schemaMap(schema["if"]); condition != nil {
		keyword := "else"
		if v.checkDocument(docName, condition, value, "$annotation") == nil {
			keyword = "then"
		}
		if child := schemaMap(schema[keyword]); child != nil {
			merge(v.evaluatedProperties(docName, child, value, cloneStringSet(seen)))
		}
	}
	if raw, exists := schema["unevaluatedProperties"]; exists {
		for key := range value {
			if out[key] {
				continue
			}
			switch rule := raw.(type) {
			case bool:
				if rule {
					out[key] = true
				}
			case map[string]any:
				if v.checkDocument(docName, rule, value[key], "$annotation."+key) == nil {
					out[key] = true
				}
			}
		}
	}
	return out
}

func cloneStringSet(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (v *Validator) declaredProperty(docName string, schema map[string]any, key string, seen map[string]bool) bool {
	if schema == nil {
		return false
	}
	if r, _ := schema["$ref"].(string); r != "" {
		if seen[r] {
			return false
		}
		seen[r] = true
		if v.declaredProperty(docName, v.resolve(docName, schema), key, seen) {
			return true
		}
	}
	if _, ok := schemaMap(schema["properties"])[key]; ok {
		return true
	}
	for _, a := range slice(schema["allOf"]) {
		if v.declaredProperty(docName, schemaMap(a), key, seen) {
			return true
		}
	}
	for _, k := range []string{"anyOf", "oneOf"} {
		for _, a := range slice(schema[k]) {
			if v.declaredProperty(docName, schemaMap(a), key, seen) {
				return true
			}
		}
	}
	for _, k := range []string{"if", "then", "else"} {
		if v.declaredProperty(docName, schemaMap(schema[k]), key, seen) {
			return true
		}
	}
	return false
}

var supportedKeywords = map[string]bool{
	"$schema": true, "$id": true, "$defs": true, "$ref": true, "title": true,
	"type": true, "const": true, "enum": true, "format": true, "pattern": true,
	"minLength": true, "maxLength": true, "minimum": true, "maximum": true,
	"exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"required": true, "properties": true, "propertyNames": true,
	"additionalProperties": true, "unevaluatedProperties": true,
	"minProperties": true, "maxProperties": true, "items": true, "contains": true,
	"minContains": true, "maxContains": true, "minItems": true, "maxItems": true,
	"uniqueItems": true, "allOf": true, "anyOf": true, "oneOf": true, "not": true,
	"if": true, "then": true, "else": true, "writeOnly": true,
	"x-fit-time-order": true, "x-fit-time-window": true, "x-fit-refresh-window": true,
	"x-fit-refresh-family-lineage": true, "x-fit-enrollment-challenge-state": true,
	"x-fit-model-authority-fields": true, "x-fit-authority-field-names": true,
	"x-fit-request-digest": true, "x-fit-websocket-deadline": true,
	"x-fit-event-payload": true, "x-fit-payload-integrity": true,
	"x-fit-outbox-causality": true, "x-fit-aggregate-history": true,
	"x-fit-durable-record-digest": true, "x-fit-recovery-consistency": true,
}

func knownKeywords(schema map[string]any, path string) error {
	for key := range schema {
		if !supportedKeywords[key] {
			return fmt.Errorf("%s: unsupported schema keyword %q (fail closed)", path, key)
		}
	}
	return nil
}

func (v *Validator) auditDocument(docName string) error {
	doc := v.docs[docName]
	if doc == nil {
		return fmt.Errorf("platformcontract: frozen schema document %q missing", docName)
	}
	return v.auditSchema(docName, doc, "#")
}

func (v *Validator) auditSchema(docName string, schema map[string]any, path string) error {
	if schema == nil {
		return fmt.Errorf("platformcontract: %s%s is not a schema object", docName, path)
	}
	if err := knownKeywords(schema, docName+path); err != nil {
		return err
	}
	stringKeyword := func(key string) (string, error) {
		raw, exists := schema[key]
		if !exists {
			return "", nil
		}
		value, ok := raw.(string)
		if !ok {
			return "", fmt.Errorf("platformcontract: %s%s/%s must be a string", docName, path, key)
		}
		return value, nil
	}
	for _, key := range []string{"$schema", "$id", "title"} {
		if _, err := stringKeyword(key); err != nil {
			return err
		}
	}
	if raw, exists := schema["type"]; exists {
		types := []string{}
		switch x := raw.(type) {
		case string:
			types = []string{x}
		case []any:
			if len(x) == 0 {
				return fmt.Errorf("platformcontract: %s%s/type is empty", docName, path)
			}
			for _, item := range x {
				value, ok := item.(string)
				if !ok {
					return fmt.Errorf("platformcontract: %s%s/type contains a non-string", docName, path)
				}
				types = append(types, value)
			}
		default:
			return fmt.Errorf("platformcontract: %s%s/type has unsupported shape", docName, path)
		}
		seen := map[string]bool{}
		for _, value := range types {
			if seen[value] {
				return fmt.Errorf("platformcontract: %s%s/type repeats %q", docName, path, value)
			}
			seen[value] = true
			switch value {
			case "null", "boolean", "object", "array", "number", "string", "integer":
			default:
				return fmt.Errorf("platformcontract: %s%s/type contains unknown type %q", docName, path, value)
			}
		}
	}
	if raw, exists := schema["enum"]; exists {
		values, ok := raw.([]any)
		if !ok || len(values) == 0 {
			return fmt.Errorf("platformcontract: %s%s/enum must be a non-empty array", docName, path)
		}
	}
	if format, err := stringKeyword("format"); err != nil {
		return err
	} else {
		switch format {
		case "", "uuid", "date-time":
		default:
			return fmt.Errorf("platformcontract: %s%s has unsupported format %q", docName, path, format)
		}
	}
	if pattern, err := stringKeyword("pattern"); err != nil {
		return err
	} else if pattern != "" {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("platformcontract: %s%s has invalid pattern: %w", docName, path, err)
		}
	}
	for _, key := range []string{
		"minLength", "maxLength", "minProperties", "maxProperties",
		"minItems", "maxItems", "minContains", "maxContains",
	} {
		if raw, exists := schema[key]; exists {
			value, ok := schemaNonNegativeInteger(raw)
			if !ok {
				return fmt.Errorf("platformcontract: %s%s/%s must be a non-negative integer", docName, path, key)
			}
			if (key == "minContains" || key == "maxContains") && schemaMap(schema["contains"]) == nil {
				return fmt.Errorf("platformcontract: %s%s/%s requires contains", docName, path, key)
			}
			_ = value
		}
	}
	for _, pair := range [][2]string{
		{"minLength", "maxLength"}, {"minProperties", "maxProperties"},
		{"minItems", "maxItems"}, {"minContains", "maxContains"},
	} {
		min, hasMin := schemaNonNegativeInteger(schema[pair[0]])
		max, hasMax := schemaNonNegativeInteger(schema[pair[1]])
		if hasMin && hasMax && min > max {
			return fmt.Errorf("platformcontract: %s%s has %s greater than %s", docName, path, pair[0], pair[1])
		}
	}
	for _, key := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf"} {
		if raw, exists := schema[key]; exists {
			value, ok := schemaFiniteNumber(raw)
			if !ok || (key == "multipleOf" && value <= 0) {
				return fmt.Errorf("platformcontract: %s%s/%s must be a valid schema number", docName, path, key)
			}
		}
	}
	for _, key := range []string{
		"additionalProperties", "unevaluatedProperties", "uniqueItems", "writeOnly",
		"x-fit-refresh-window", "x-fit-refresh-family-lineage",
		"x-fit-enrollment-challenge-state", "x-fit-authority-field-names",
		"x-fit-request-digest", "x-fit-websocket-deadline", "x-fit-event-payload",
		"x-fit-payload-integrity", "x-fit-outbox-causality", "x-fit-aggregate-history",
		"x-fit-durable-record-digest", "x-fit-recovery-consistency",
	} {
		raw, exists := schema[key]
		if !exists {
			continue
		}
		if (key == "additionalProperties" || key == "unevaluatedProperties") && schemaMap(raw) != nil {
			continue
		}
		if _, ok := raw.(bool); !ok {
			return fmt.Errorf("platformcontract: %s%s/%s must be boolean or schema", docName, path, key)
		}
	}
	if raw, exists := schema["required"]; exists {
		values, ok := stringArray(raw)
		if !ok || hasDuplicateString(values) {
			return fmt.Errorf("platformcontract: %s%s/required must contain unique strings", docName, path)
		}
	}
	if raw, exists := schema["x-fit-model-authority-fields"]; exists {
		values, ok := stringArray(raw)
		if !ok || len(values) == 0 || hasDuplicateString(values) {
			return fmt.Errorf("platformcontract: %s%s/x-fit-model-authority-fields must contain unique strings", docName, path)
		}
	}
	if raw, exists := schema["x-fit-time-window"]; exists {
		rule := schemaMap(raw)
		seconds, ok := schemaNonNegativeInteger(rule["seconds"])
		if rule == nil || seconds == 0 || !isNonEmptyString(rule["start"]) || !isNonEmptyString(rule["end"]) || len(rule) != 3 || !ok {
			return fmt.Errorf("platformcontract: %s%s/x-fit-time-window has invalid shape", docName, path)
		}
	}
	if raw, exists := schema["x-fit-time-order"]; exists {
		rules, ok := raw.([]any)
		if !ok || len(rules) == 0 {
			return fmt.Errorf("platformcontract: %s%s/x-fit-time-order must be a non-empty array", docName, path)
		}
		for _, item := range rules {
			rule := schemaMap(item)
			if len(rule) != 2 || !isNonEmptyString(rule["start"]) || !isNonEmptyString(rule["end"]) {
				return fmt.Errorf("platformcontract: %s%s/x-fit-time-order contains an invalid rule", docName, path)
			}
		}
	}
	if ref, err := stringKeyword("$ref"); err != nil {
		return err
	} else if ref != "" {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(ref, prefix) || strings.Contains(strings.TrimPrefix(ref, prefix), "/") || v.resolve(docName, schema) == nil {
			return fmt.Errorf("platformcontract: %s%s has unresolved or non-local ref %q", docName, path, ref)
		}
	}
	for _, key := range []string{"$defs", "properties"} {
		raw, exists := schema[key]
		if !exists {
			continue
		}
		children, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("platformcontract: %s%s/%s must be an object", docName, path, key)
		}
		for _, name := range sortedKeys(children) {
			child := schemaMap(children[name])
			if child == nil {
				return fmt.Errorf("platformcontract: %s%s/%s/%s is not a schema object", docName, path, key, name)
			}
			if err := v.auditSchema(docName, child, path+"/"+key+"/"+escapeJSONPointer(name)); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"propertyNames", "items", "contains", "not", "if", "then", "else"} {
		raw, exists := schema[key]
		if !exists {
			continue
		}
		child := schemaMap(raw)
		if child == nil {
			return fmt.Errorf("platformcontract: %s%s/%s is not a supported schema object", docName, path, key)
		}
		if err := v.auditSchema(docName, child, path+"/"+key); err != nil {
			return err
		}
	}
	for _, key := range []string{"additionalProperties", "unevaluatedProperties"} {
		if child := schemaMap(schema[key]); child != nil {
			if err := v.auditSchema(docName, child, path+"/"+key); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		raw, exists := schema[key]
		if !exists {
			continue
		}
		children, ok := raw.([]any)
		if !ok || len(children) == 0 {
			return fmt.Errorf("platformcontract: %s%s/%s must be a non-empty schema array", docName, path, key)
		}
		for index, rawChild := range children {
			child := schemaMap(rawChild)
			if child == nil {
				return fmt.Errorf("platformcontract: %s%s/%s/%d is not a schema object", docName, path, key, index)
			}
			if err := v.auditSchema(docName, child, fmt.Sprintf("%s/%s/%d", path, key, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func schemaFiniteNumber(raw any) (float64, bool) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Float64()
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func schemaNonNegativeInteger(raw any) (int64, bool) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(string(number), 10, 64)
	return value, err == nil && value >= 0
}

func stringArray(raw any) ([]string, bool) {
	values, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, len(values))
	for index, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok {
			return nil, false
		}
		result[index] = value
	}
	return result, true
}

func hasDuplicateString(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

func isNonEmptyString(raw any) bool {
	value, ok := raw.(string)
	return ok && value != ""
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func checkNumberBounds(schema map[string]any, value any, path string) error {
	v, ok := binary64(value)
	if !ok {
		return fmt.Errorf("%s: invalid number", path)
	}
	for _, rule := range []struct {
		key     string
		compare func(float64, float64) bool
	}{
		{"minimum", func(value, bound float64) bool { return value < bound }},
		{"maximum", func(value, bound float64) bool { return value > bound }},
		{"exclusiveMinimum", func(value, bound float64) bool { return value <= bound }},
		{"exclusiveMaximum", func(value, bound float64) bool { return value >= bound }},
	} {
		if raw, exists := schema[rule.key]; exists {
			b, good := binary64(raw)
			if !good || rule.compare(v, b) {
				return fmt.Errorf("%s: %s bound", path, rule.key)
			}
		}
	}
	if raw, exists := schema["multipleOf"]; exists {
		d, good := binary64(raw)
		if !good || d <= 0 {
			return fmt.Errorf("%s: invalid multipleOf", path)
		}
		q := v / d
		if math.Trunc(q) != q {
			return fmt.Errorf("%s: multipleOf", path)
		}
	}
	return nil
}

// binary64 mirrors JSON.parse's numeric domain. ParseStrictJSON preserves the
// original lexeme for canonicalization, but JSON Schema decisions are made on
// the finite IEEE-754 value used by the frozen Node/AJV authority. Numeric
// underflow therefore remains a valid signed zero; overflow remains invalid.
func binary64(raw any) (float64, bool) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseFloat(string(number), 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// checkFit executes every frozen extension at the point where its schema is
// visited, including refs, array items, and conditional branches.
func (v *Validator) checkFit(schema map[string]any, value any, path string) error {
	m, isObject := value.(map[string]any)
	if x, ok := schema["x-fit-time-order"]; ok {
		if !isObject {
			return fmt.Errorf("%s: time order object", path)
		}
		for _, raw := range slice(x) {
			pairDef := schemaMap(raw)
			a, hasA := m[pairDef["start"].(string)]
			b, hasB := m[pairDef["end"].(string)]
			if hasB {
				am, okA := millis(a)
				bm, okB := millis(b)
				if !hasA || !okA || !okB || bm < am {
					return fmt.Errorf("%s: x-fit-time-order", path)
				}
			}
		}
	}
	if x, ok := schema["x-fit-time-window"]; ok {
		if !isObject {
			return fmt.Errorf("%s: time window object", path)
		}
		d := schemaMap(x)
		a, okA := millis(m[fmt.Sprint(d["start"])])
		b, okB := millis(m[fmt.Sprint(d["end"])])
		seconds, okS := numberInt(d["seconds"])
		if !okA || !okB || !okS || b-a != seconds*1000 {
			return fmt.Errorf("%s: x-fit-time-window", path)
		}
		if revoked, exists := m["revoked_at"]; exists {
			r, ok := millis(revoked)
			if !ok || r < a {
				return fmt.Errorf("%s: x-fit-time-window revocation", path)
			}
		}
	}
	if _, ok := schema["x-fit-refresh-window"]; ok {
		if !isObject {
			return fmt.Errorf("%s: refresh object", path)
		}
		if e := refreshToken(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-refresh-family-lineage"]; ok {
		if !isObject {
			return fmt.Errorf("%s: family object", path)
		}
		if e := refreshFamily(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-enrollment-challenge-state"]; ok {
		if !isObject {
			return fmt.Errorf("%s: challenge state object", path)
		}
		if e := enrollmentState(m); e != nil {
			return e
		}
	}
	if x, ok := schema["x-fit-model-authority-fields"]; ok {
		if containsAuthority(value, append(authorityNames(v, "UntrustedJsonObject"), strSlice(x)...)) {
			return fmt.Errorf("%s: model authority field", path)
		}
	}
	if _, ok := schema["x-fit-authority-field-names"]; ok {
		if containsAuthority(value, authorityNames(v, "UntrustedJsonObject")) {
			return fmt.Errorf("%s: untrusted authority field", path)
		}
	}
	if _, ok := schema["x-fit-request-digest"]; ok {
		if !isObject {
			return fmt.Errorf("%s: request object", path)
		}
		d, err := RequestDigest(m)
		if err != nil || m["canonical_request_digest"] != d {
			return fmt.Errorf("%s: request digest mismatch", path)
		}
	}
	if _, ok := schema["x-fit-websocket-deadline"]; ok {
		if !isObject {
			return fmt.Errorf("%s: websocket object", path)
		}
		if e := websocketDeadline(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-event-payload"]; ok {
		if !isObject {
			return fmt.Errorf("%s: event payload object", path)
		}
		if e := v.eventPayload(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-payload-integrity"]; ok {
		if !isObject {
			return fmt.Errorf("%s: event envelope object", path)
		}
		if e := v.eventEnvelope(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-outbox-causality"]; ok {
		if !isObject {
			return fmt.Errorf("%s: outbox object", path)
		}
		if e := outboxCausality(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-aggregate-history"]; ok {
		if !isObject {
			return fmt.Errorf("%s: history object", path)
		}
		if e := v.aggregateHistory(m); e != nil {
			return e
		}
	}
	if _, ok := schema["x-fit-durable-record-digest"]; ok {
		if !isObject {
			return fmt.Errorf("%s: durable object", path)
		}
		d, err := DurableRecordDigest(m)
		if err != nil || m["payload_digest"] != d {
			return fmt.Errorf("%s: durable digest mismatch", path)
		}
	}
	if _, ok := schema["x-fit-recovery-consistency"]; ok {
		if !isObject {
			return fmt.Errorf("%s: recovery object", path)
		}
		if e := recoveryConsistency(m); e != nil {
			return e
		}
	}
	return nil
}
func matchesType(t any, v any) bool {
	types := strSlice(t)
	if s, ok := t.(string); ok {
		types = []string{s}
	}
	for _, x := range types {
		switch x {
		case "object":
			if _, ok := v.(map[string]any); ok {
				return true
			}
		case "array":
			if _, ok := v.([]any); ok {
				return true
			}
		case "string":
			if _, ok := v.(string); ok {
				return true
			}
		case "boolean":
			if _, ok := v.(bool); ok {
				return true
			}
		case "number":
			if _, ok := binary64(v); ok {
				return true
			}
		case "integer":
			if value, ok := binary64(v); ok && math.Trunc(value) == value {
				return true
			}
		case "null":
			if v == nil {
				return true
			}
		}
	}
	return false
}
func equalJSON(a, b any) bool {
	left, leftErr := canonical(a)
	right, rightErr := canonical(b)
	return leftErr == nil && rightErr == nil && left == right
}
func validFormat(f, s string) bool {
	switch f {
	case "uuid":
		return ajvUUIDRE.MatchString(s)
	case "date-time":
		return validTimestamp(s)
	default:
		return false
	}
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var ajvUUIDRE = regexp.MustCompile(`(?i)^(urn:uuid:)?[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$`)
var timestampRE = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{3})?Z$`)

func validTimestamp(s string) bool {
	if !timestampRE.MatchString(s) {
		return false
	}
	year, yearErr := strconv.Atoi(s[0:4])
	month, monthErr := strconv.Atoi(s[5:7])
	day, dayErr := strconv.Atoi(s[8:10])
	hour, hourErr := strconv.Atoi(s[11:13])
	minute, minuteErr := strconv.Atoi(s[14:16])
	second, secondErr := strconv.Atoi(s[17:19])
	if yearErr != nil || monthErr != nil || dayErr != nil ||
		hourErr != nil || minuteErr != nil || secondErr != nil {
		return false
	}
	days := [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		days[2] = 29
	}
	if month < 1 || month > 12 || day < 1 || day > days[month] ||
		hour > 23 || minute > 59 {
		return false
	}
	// ajv-formats accepts an RFC 3339 leap second only at 23:59 UTC.
	// The frozen Timestamp pattern has already fixed the zone to Z.
	return second < 60 || (hour == 23 && minute == 59 && second == 60)
}

// FrozenContractSHA256 returns hashes for generated source-drift checks.
func FrozenContractSHA256() map[string]string {
	o := map[string]string{}
	for k, v := range frozenContractSHA256 {
		o[k] = v
	}
	return o
}

// ContractJSON returns a defensive copy of a generated contract document.
func (v *Validator) ContractJSON(name string) ([]byte, error) {
	b64, ok := frozenContractBase64[name]
	if !ok {
		return nil, fmt.Errorf("platformcontract: missing contract %s", name)
	}
	return base64.StdEncoding.DecodeString(b64)
}

// FieldMappings is the generated, exact mechanical disposition for every
// schema property: a concrete accessor, a tagged-union branch, or the single
// transport-only writeOnly field.
func (v *Validator) FieldMappings() map[string]string {
	return GeneratedFieldDispositions()
}

// FieldMappingsDigest pins the complete JSON-pointer disposition inventory.
// It changes for any property addition, removal, disposition, or pointer
// traversal change and is intentionally independent of Go map iteration.
func (v *Validator) FieldMappingsDigest() string {
	m := v.FieldMappings()
	canonical := make(map[string]any, len(m))
	for k, value := range m {
		canonical[k] = value
	}
	return sha256Hex(CanonicalJSON(canonical))
}
func sha256Hex(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func sortedKeys(m map[string]any) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}
