package platformcontract

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// TypedContract is sealed by the generated contract types. A caller can only
// obtain a populated value through DecodeTyped or a generated Decode<Type>
// method, each of which starts from strict raw JSON bytes.
type TypedContract interface {
	SchemaName() string
	sealedTypedContract()
}

// FrozenManifest is sealed by the ten generated immutable manifest views.
type FrozenManifest interface {
	ManifestName() string
	sealedFrozenManifest()
}

// JSONNumber is a finite JSON number preserved without converting the
// validated input to an implementation-defined integer width.
type JSONNumber struct {
	text string
}

func (n JSONNumber) String() string { return n.text }

func (n JSONNumber) Float64() (float64, error) {
	return strconv.ParseFloat(n.text, 64)
}

func (n JSONNumber) Int64() (int64, bool) {
	f, err := strconv.ParseFloat(n.text, 64)
	if err != nil {
		return 0, false
	}
	i := int64(f)
	return i, float64(i) == f
}

type optionalValue[T any] struct {
	value T
	set   bool
}

func presentValue[T any](value T) optionalValue[T] {
	return optionalValue[T]{value: value, set: true}
}

// JsonValueKind is the explicit tag used by JsonValue, UntrustedJsonValue,
// ManifestValue, and all open-object values.
type JsonValueKind uint8

const (
	JsonValueInvalid JsonValueKind = iota
	JsonValueNull
	JsonValueBoolean
	JsonValueNumber
	JsonValueString
	JsonValueArray
	JsonValueObject
)

// JsonObject is a validated recursive JSON object. It intentionally exposes
// lookup and sorted-key access, never its backing map.
type JsonObject struct {
	fields map[string]JsonValue
}

func (JsonObject) SchemaName() string   { return "JsonObject" }
func (JsonObject) sealedTypedContract() {}
func (o JsonObject) Len() int           { return len(o.fields) }
func (o JsonObject) Lookup(key string) (JsonValue, bool) {
	value, ok := o.fields[key]
	return value, ok
}
func (o JsonObject) Keys() []string {
	keys := make([]string, 0, len(o.fields))
	for key := range o.fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// JsonValue is the faithful recursive representation of the open JsonValue
// schema. No branch contains interface{} or map[string]interface{}.
type JsonValue struct {
	kind        JsonValueKind
	boolean     bool
	number      JSONNumber
	stringValue string
	array       []JsonValue
	object      JsonObject
}

func (JsonValue) SchemaName() string   { return "JsonValue" }
func (JsonValue) sealedTypedContract() {}
func (v JsonValue) Kind() JsonValueKind {
	return v.kind
}
func (v JsonValue) IsNull() bool { return v.kind == JsonValueNull }
func (v JsonValue) Boolean() (bool, bool) {
	return v.boolean, v.kind == JsonValueBoolean
}
func (v JsonValue) Number() (JSONNumber, bool) {
	return v.number, v.kind == JsonValueNumber
}
func (v JsonValue) StringValue() (string, bool) {
	return v.stringValue, v.kind == JsonValueString
}
func (v JsonValue) Array() ([]JsonValue, bool) {
	if v.kind != JsonValueArray {
		return nil, false
	}
	return append([]JsonValue(nil), v.array...), true
}
func (v JsonValue) Object() (JsonObject, bool) {
	return v.object, v.kind == JsonValueObject
}

// UntrustedJsonObject and UntrustedJsonValue remain distinct from JsonObject
// and JsonValue because their construction is guarded by recursive x-fit
// authority-field validation.
type UntrustedJsonObject struct {
	fields map[string]UntrustedJsonValue
}

func (UntrustedJsonObject) SchemaName() string   { return "UntrustedJsonObject" }
func (UntrustedJsonObject) sealedTypedContract() {}
func (o UntrustedJsonObject) Len() int           { return len(o.fields) }
func (o UntrustedJsonObject) Lookup(key string) (UntrustedJsonValue, bool) {
	value, ok := o.fields[key]
	return value, ok
}
func (o UntrustedJsonObject) Keys() []string {
	keys := make([]string, 0, len(o.fields))
	for key := range o.fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type UntrustedJsonValue struct {
	kind        JsonValueKind
	boolean     bool
	number      JSONNumber
	stringValue string
	array       []UntrustedJsonValue
	object      UntrustedJsonObject
}

func (UntrustedJsonValue) SchemaName() string   { return "UntrustedJsonValue" }
func (UntrustedJsonValue) sealedTypedContract() {}
func (v UntrustedJsonValue) Kind() JsonValueKind {
	return v.kind
}
func (v UntrustedJsonValue) IsNull() bool { return v.kind == JsonValueNull }
func (v UntrustedJsonValue) Boolean() (bool, bool) {
	return v.boolean, v.kind == JsonValueBoolean
}
func (v UntrustedJsonValue) Number() (JSONNumber, bool) {
	return v.number, v.kind == JsonValueNumber
}
func (v UntrustedJsonValue) StringValue() (string, bool) {
	return v.stringValue, v.kind == JsonValueString
}
func (v UntrustedJsonValue) Array() ([]UntrustedJsonValue, bool) {
	if v.kind != JsonValueArray {
		return nil, false
	}
	return append([]UntrustedJsonValue(nil), v.array...), true
}
func (v UntrustedJsonValue) Object() (UntrustedJsonObject, bool) {
	return v.object, v.kind == JsonValueObject
}

// ManifestObject and ManifestValue provide immutable recursive access for
// manifest locations whose literal data is heterogeneous.
type ManifestObject struct {
	fields map[string]ManifestValue
}

func (o ManifestObject) Len() int { return len(o.fields) }
func (o ManifestObject) Lookup(key string) (ManifestValue, bool) {
	value, ok := o.fields[key]
	return value, ok
}
func (o ManifestObject) Keys() []string {
	keys := make([]string, 0, len(o.fields))
	for key := range o.fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type ManifestValue struct {
	kind        JsonValueKind
	boolean     bool
	number      JSONNumber
	stringValue string
	array       []ManifestValue
	object      ManifestObject
}

func (v ManifestValue) Kind() JsonValueKind { return v.kind }
func (v ManifestValue) IsNull() bool        { return v.kind == JsonValueNull }
func (v ManifestValue) Boolean() (bool, bool) {
	return v.boolean, v.kind == JsonValueBoolean
}
func (v ManifestValue) Number() (JSONNumber, bool) {
	return v.number, v.kind == JsonValueNumber
}
func (v ManifestValue) StringValue() (string, bool) {
	return v.stringValue, v.kind == JsonValueString
}
func (v ManifestValue) Array() ([]ManifestValue, bool) {
	if v.kind != JsonValueArray {
		return nil, false
	}
	return append([]ManifestValue(nil), v.array...), true
}
func (v ManifestValue) Object() (ManifestObject, bool) {
	return v.object, v.kind == JsonValueObject
}

// transportSecret is an intentionally empty marker. Construction checks the
// transport field's concrete JSON type, then discards the credential so a
// generated contract cannot accidentally retain, expose, or marshal it.
type transportSecret struct{}

func (transportSecret) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED]")
}

// DecodeTyped is the sole dynamic named construction boundary.
func (v *Validator) DecodeTyped(name string, raw []byte) (TypedContract, error) {
	value, err := ParseStrictJSON(raw)
	if err != nil {
		return nil, err
	}
	if err := v.validate(name, value); err != nil {
		return nil, err
	}
	return buildGeneratedContract(v, name, value)
}

func (v *Validator) Manifest(name string) (FrozenManifest, error) {
	doc, ok := v.docs["manifests/"+name+"-v1.json"]
	if !ok {
		return nil, fmt.Errorf("platformcontract: unknown manifest %q", name)
	}
	return buildGeneratedManifest(name, doc)
}

func requireObject(value any, name string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("platformcontract: generated %s expected object", name)
	}
	return object, nil
}

func requireArray(value any, name string) ([]any, error) {
	array, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("platformcontract: generated %s expected array", name)
	}
	return array, nil
}

func requireString(value any, name string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("platformcontract: generated %s expected string", name)
	}
	return text, nil
}

func requireBoolean(value any, name string) (bool, error) {
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("platformcontract: generated %s expected boolean", name)
	}
	return boolean, nil
}

func requireNumber(value any, name string) (JSONNumber, error) {
	number, ok := value.(json.Number)
	if !ok {
		return JSONNumber{}, fmt.Errorf("platformcontract: generated %s expected number", name)
	}
	return JSONNumber{text: string(number)}, nil
}

func buildStringField(_ *Validator, value any) (string, error) {
	return requireString(value, "string field")
}

func buildBooleanField(_ *Validator, value any) (bool, error) {
	return requireBoolean(value, "boolean field")
}

func buildNumberField(_ *Validator, value any) (JSONNumber, error) {
	return requireNumber(value, "number field")
}

func buildTransportSecret(_ *Validator, value any) (transportSecret, error) {
	_, err := requireString(value, "transport-only field")
	if err != nil {
		return transportSecret{}, err
	}
	return transportSecret{}, nil
}

func buildJsonObjectField(_ *Validator, value any) (JsonObject, error) {
	return buildJsonObject(value)
}

func buildJsonValueField(_ *Validator, value any) (JsonValue, error) {
	return buildJsonValue(value)
}

func buildUntrustedJsonObjectField(_ *Validator, value any) (UntrustedJsonObject, error) {
	return buildUntrustedJsonObject(value)
}

func buildUntrustedJsonValueField(_ *Validator, value any) (UntrustedJsonValue, error) {
	return buildUntrustedJsonValue(value)
}

func buildGeneratedSlice[T any](
	v *Validator,
	value any,
	name string,
	builder func(*Validator, any) (T, error),
) ([]T, error) {
	raw, err := requireArray(value, name)
	if err != nil {
		return nil, err
	}
	result := make([]T, len(raw))
	for index, item := range raw {
		converted, err := builder(v, item)
		if err != nil {
			return nil, err
		}
		result[index] = converted
	}
	return result, nil
}

func buildJsonObject(value any) (JsonObject, error) {
	object, err := requireObject(value, "JsonObject")
	if err != nil {
		return JsonObject{}, err
	}
	fields := make(map[string]JsonValue, len(object))
	for key, item := range object {
		converted, err := buildJsonValue(item)
		if err != nil {
			return JsonObject{}, err
		}
		fields[key] = converted
	}
	return JsonObject{fields: fields}, nil
}

func buildJsonValue(value any) (JsonValue, error) {
	switch item := value.(type) {
	case nil:
		return JsonValue{kind: JsonValueNull}, nil
	case bool:
		return JsonValue{kind: JsonValueBoolean, boolean: item}, nil
	case json.Number:
		return JsonValue{kind: JsonValueNumber, number: JSONNumber{text: string(item)}}, nil
	case string:
		return JsonValue{kind: JsonValueString, stringValue: item}, nil
	case []any:
		array := make([]JsonValue, len(item))
		for index, child := range item {
			converted, err := buildJsonValue(child)
			if err != nil {
				return JsonValue{}, err
			}
			array[index] = converted
		}
		return JsonValue{kind: JsonValueArray, array: array}, nil
	case map[string]any:
		object, err := buildJsonObject(item)
		if err != nil {
			return JsonValue{}, err
		}
		return JsonValue{kind: JsonValueObject, object: object}, nil
	default:
		return JsonValue{}, fmt.Errorf("platformcontract: generated JsonValue cannot contain %T", value)
	}
}

func buildUntrustedJsonObject(value any) (UntrustedJsonObject, error) {
	object, err := requireObject(value, "UntrustedJsonObject")
	if err != nil {
		return UntrustedJsonObject{}, err
	}
	fields := make(map[string]UntrustedJsonValue, len(object))
	for key, item := range object {
		converted, err := buildUntrustedJsonValue(item)
		if err != nil {
			return UntrustedJsonObject{}, err
		}
		fields[key] = converted
	}
	return UntrustedJsonObject{fields: fields}, nil
}

func buildUntrustedJsonValue(value any) (UntrustedJsonValue, error) {
	switch item := value.(type) {
	case nil:
		return UntrustedJsonValue{kind: JsonValueNull}, nil
	case bool:
		return UntrustedJsonValue{kind: JsonValueBoolean, boolean: item}, nil
	case json.Number:
		return UntrustedJsonValue{kind: JsonValueNumber, number: JSONNumber{text: string(item)}}, nil
	case string:
		return UntrustedJsonValue{kind: JsonValueString, stringValue: item}, nil
	case []any:
		array := make([]UntrustedJsonValue, len(item))
		for index, child := range item {
			converted, err := buildUntrustedJsonValue(child)
			if err != nil {
				return UntrustedJsonValue{}, err
			}
			array[index] = converted
		}
		return UntrustedJsonValue{kind: JsonValueArray, array: array}, nil
	case map[string]any:
		object, err := buildUntrustedJsonObject(item)
		if err != nil {
			return UntrustedJsonValue{}, err
		}
		return UntrustedJsonValue{kind: JsonValueObject, object: object}, nil
	default:
		return UntrustedJsonValue{}, fmt.Errorf("platformcontract: generated UntrustedJsonValue cannot contain %T", value)
	}
}

func buildManifestObject(value any) (ManifestObject, error) {
	object, err := requireObject(value, "ManifestObject")
	if err != nil {
		return ManifestObject{}, err
	}
	fields := make(map[string]ManifestValue, len(object))
	for key, item := range object {
		converted, err := buildManifestValue(item)
		if err != nil {
			return ManifestObject{}, err
		}
		fields[key] = converted
	}
	return ManifestObject{fields: fields}, nil
}

func buildManifestValue(value any) (ManifestValue, error) {
	switch item := value.(type) {
	case nil:
		return ManifestValue{kind: JsonValueNull}, nil
	case bool:
		return ManifestValue{kind: JsonValueBoolean, boolean: item}, nil
	case json.Number:
		return ManifestValue{kind: JsonValueNumber, number: JSONNumber{text: string(item)}}, nil
	case string:
		return ManifestValue{kind: JsonValueString, stringValue: item}, nil
	case []any:
		array := make([]ManifestValue, len(item))
		for index, child := range item {
			converted, err := buildManifestValue(child)
			if err != nil {
				return ManifestValue{}, err
			}
			array[index] = converted
		}
		return ManifestValue{kind: JsonValueArray, array: array}, nil
	case map[string]any:
		object, err := buildManifestObject(item)
		if err != nil {
			return ManifestValue{}, err
		}
		return ManifestValue{kind: JsonValueObject, object: object}, nil
	default:
		return ManifestValue{}, fmt.Errorf("platformcontract: generated ManifestValue cannot contain %T", value)
	}
}

func buildManifestString(value any) (string, error) {
	return requireString(value, "manifest string")
}

func buildManifestBoolean(value any) (bool, error) {
	return requireBoolean(value, "manifest boolean")
}

func buildManifestNumber(value any) (JSONNumber, error) {
	return requireNumber(value, "manifest number")
}

func buildManifestSlice[T any](
	value any,
	name string,
	builder func(any) (T, error),
) ([]T, error) {
	raw, err := requireArray(value, name)
	if err != nil {
		return nil, err
	}
	result := make([]T, len(raw))
	for index, item := range raw {
		converted, err := builder(item)
		if err != nil {
			return nil, err
		}
		result[index] = converted
	}
	return result, nil
}

func additionalJsonObject(object map[string]any, known map[string]bool) (JsonObject, error) {
	additional := make(map[string]any)
	for key, value := range object {
		if !known[key] {
			additional[key] = value
		}
	}
	return buildJsonObject(additional)
}

func cloneObject(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneContractValue(value)
	}
	return out
}

func cloneContractValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		return cloneObject(item)
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			result[index] = cloneContractValue(child)
		}
		return result
	default:
		return item
	}
}

func generatedSchemaAt(v *Validator, document, pointer string) (map[string]any, error) {
	var current any = v.docs[document]
	if current == nil {
		return nil, fmt.Errorf("platformcontract: generated schema document %q missing", document)
	}
	trimmed := strings.TrimPrefix(pointer, "#")
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		schema, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("platformcontract: generated schema root is not an object")
		}
		return schema, nil
	}
	for _, encoded := range strings.Split(trimmed, "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		switch node := current.(type) {
		case map[string]any:
			current = node[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, fmt.Errorf("platformcontract: generated schema pointer %q is invalid", pointer)
			}
			current = node[index]
		default:
			return nil, fmt.Errorf("platformcontract: generated schema pointer %q is invalid", pointer)
		}
	}
	schema, ok := current.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("platformcontract: generated schema pointer %q is not an object", pointer)
	}
	return schema, nil
}

func generatedUnionBranch(v *Validator, document, pointer, keyword string, value any) (int, error) {
	parent, err := generatedSchemaAt(v, document, pointer)
	if err != nil {
		return -1, err
	}
	branches, ok := parent[keyword].([]any)
	if !ok || len(branches) == 0 {
		return -1, fmt.Errorf("platformcontract: generated %s union %q missing", keyword, pointer)
	}
	match := -1
	for index, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok {
			return -1, fmt.Errorf("platformcontract: generated union branch is not an object")
		}
		if v.checkDocument(document, branch, value, "$") == nil {
			if match >= 0 && keyword == "oneOf" {
				return -1, fmt.Errorf("platformcontract: generated oneOf matched multiple branches")
			}
			match = index
		}
	}
	if match < 0 {
		return -1, fmt.Errorf("platformcontract: generated union matched no branch")
	}
	return match, nil
}
