package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"hash/fnv"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type schemaFile struct {
	key          string
	filename     string
	rootName     string
	localPrefix  string
	platformDefs bool
}

type schemaDocument struct {
	schemaFile
	root map[string]any
	defs map[string]any
}

type manifestFile struct {
	key      string
	filename string
	goName   string
	value    map[string]any
}

type schemaRef struct {
	doc     *schemaDocument
	node    map[string]any
	pointer string
}

func (ref schemaRef) key() string {
	return ref.doc.key + ref.pointer
}

type fieldModel struct {
	jsonName  string
	goName    string
	private   string
	ref       schemaRef
	required  bool
	writeOnly bool
	spec      *valueSpec
}

type valueSpec struct {
	goType      string
	builder     string
	slice       bool
	element     *valueSpec
	model       *typeModel
	enum        *enumModel
	transport   bool
	manifestAny bool
}

type enumModel struct {
	name   string
	values []string
}

type embeddedUnion struct {
	name        string
	privateName string
	pointer     string
	keyword     string
	branches    int
}

type unionBranch struct {
	name    string
	private string
	ref     schemaRef
	spec    *valueSpec
}

type typeModel struct {
	name           string
	ref            schemaRef
	contract       bool
	category       string
	fields         []*fieldModel
	open           bool
	scalarType     string
	enum           *enumModel
	unionPointer   string
	unionKeyword   string
	unionBranches  []*unionBranch
	embeddedUnions []*embeddedUnion
	arrayElement   *valueSpec
}

type manifestObjectModel struct {
	name   string
	fields []*manifestFieldModel
}

type manifestFieldModel struct {
	jsonName string
	goName   string
	private  string
	value    any
	spec     *manifestValueSpec
}

type manifestValueSpec struct {
	goType  string
	kind    string
	object  *manifestObjectModel
	element *manifestValueSpec
	union   *manifestUnionModel
}

type manifestUnionModel struct {
	name     string
	variants []*manifestUnionVariant
}

type manifestUnionVariant struct {
	name    string
	private string
	shape   string
	value   any
	spec    *manifestValueSpec
}

type generator struct {
	root           string
	documents      []*schemaDocument
	manifests      []*manifestFile
	inputs         map[string][]byte
	typeNames      map[string]string
	modelsByName   map[string]*typeModel
	modelsByRef    map[string]*typeModel
	models         []*typeModel
	enumsByName    map[string]*enumModel
	enums          []*enumModel
	usedTypeNames  map[string]string
	manifestObjs   []*manifestObjectModel
	manifestUnions []*manifestUnionModel
	manifestNames  map[string]string
}

func loadGenerator(root string) (*generator, error) {
	g := &generator{
		root:          root,
		inputs:        map[string][]byte{},
		typeNames:     map[string]string{},
		modelsByName:  map[string]*typeModel{},
		modelsByRef:   map[string]*typeModel{},
		enumsByName:   map[string]*enumModel{},
		usedTypeNames: map[string]string{},
		manifestNames: map[string]string{},
	}
	for _, file := range schemaFiles {
		value, raw, err := decodeJSONFile(filepath.Join(root, file.filename))
		if err != nil {
			return nil, err
		}
		defs, _ := value["$defs"].(map[string]any)
		document := &schemaDocument{schemaFile: file, root: value, defs: defs}
		g.documents = append(g.documents, document)
		g.inputs[file.key] = raw
	}
	for _, file := range manifestFiles {
		value, raw, err := decodeJSONFile(filepath.Join(root, file.filename))
		if err != nil {
			return nil, err
		}
		copyFile := file
		copyFile.value = value
		g.manifests = append(g.manifests, &copyFile)
		g.inputs[file.filename] = raw
	}
	if err := g.registerNamedSchemas(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *generator) registerNamedSchemas() error {
	for _, doc := range g.documents {
		if doc.platformDefs {
			names := sortedMapKeys(doc.defs)
			for _, name := range names {
				g.typeNames[doc.key+"#/$defs/"+name] = name
			}
			continue
		}
		g.typeNames[doc.key+"#"] = doc.rootName
		for _, name := range sortedMapKeys(doc.defs) {
			g.typeNames[doc.key+"#/$defs/"+name] = doc.localPrefix + name
		}
	}
	for _, doc := range g.documents {
		if doc.platformDefs {
			for _, name := range sortedMapKeys(doc.defs) {
				node, _ := doc.defs[name].(map[string]any)
				if _, err := g.ensureModel(schemaRef{doc: doc, node: node, pointer: "#/$defs/" + escapePointer(name)}, name, true); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := g.ensureModel(schemaRef{doc: doc, node: doc.root, pointer: "#"}, doc.rootName, true); err != nil {
			return err
		}
		for _, name := range sortedMapKeys(doc.defs) {
			node, _ := doc.defs[name].(map[string]any)
			goName := doc.localPrefix + name
			if _, err := g.ensureModel(schemaRef{doc: doc, node: node, pointer: "#/$defs/" + escapePointer(name)}, goName, false); err != nil {
				return err
			}
		}
	}
	for index := 0; index < len(g.models); index++ {
		if err := g.populateModel(g.models[index]); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) ensureModel(ref schemaRef, requestedName string, contract bool) (*typeModel, error) {
	if existing := g.modelsByRef[ref.key()]; existing != nil {
		if contract {
			existing.contract = true
		}
		return existing, nil
	}
	if isManualSchemaName(requestedName) {
		return &typeModel{name: requestedName, ref: ref, contract: contract, category: "manual"}, nil
	}
	name := g.uniqueTypeName(requestedName, ref.key())
	model := &typeModel{name: name, ref: ref, contract: contract}
	g.modelsByRef[ref.key()] = model
	g.modelsByName[name] = model
	g.models = append(g.models, model)
	return model, nil
}

func isManualSchemaName(name string) bool {
	switch name {
	case "JsonObject", "JsonValue", "UntrustedJsonObject", "UntrustedJsonValue":
		return true
	default:
		return false
	}
}

func (g *generator) populateModel(model *typeModel) error {
	if model.category != "" {
		return nil
	}
	node := model.ref.node
	if _, ok := node["enum"]; ok && node["type"] == nil && node["properties"] == nil {
		model.category = "enum"
		values, err := stringEnum(node)
		if err != nil {
			return fmt.Errorf("%s: %w", model.ref.key(), err)
		}
		enum := g.ensureEnum(model.name+"Value", values)
		model.enum = enum
		return nil
	}
	if unionNode, pointer, keyword := positiveUnion(node, model.ref.pointer); unionNode != nil {
		model.category = "union"
		model.unionPointer = pointer
		model.unionKeyword = keyword
		branches, _ := unionNode[keyword].([]any)
		used := map[string]int{}
		for index, raw := range branches {
			branchNode, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: union branch %d is not an object", model.ref.key(), index)
			}
			branchRef := schemaRef{
				doc:     model.ref.doc,
				node:    branchNode,
				pointer: pointer + "/" + keyword + "/" + strconv.Itoa(index),
			}
			label := unionBranchLabel(branchNode, index)
			used[label]++
			if used[label] > 1 {
				label += strconv.Itoa(used[label])
			}
			spec, err := g.valueSpec(branchRef, model.name+label, false)
			if err != nil {
				return err
			}
			model.unionBranches = append(model.unionBranches, &unionBranch{
				name:    label,
				private: privateIdentifier(label),
				ref:     branchRef,
				spec:    spec,
			})
		}
		return nil
	}
	if isObjectLike(g, model.ref, map[string]bool{}) {
		model.category = "object"
		fields, open, err := g.objectFields(model.ref)
		if err != nil {
			return err
		}
		model.fields = fields
		model.open = open
		for index, raw := range arrayValue(node["allOf"]) {
			item, _ := raw.(map[string]any)
			if len(arrayValue(item["oneOf"])) == 0 {
				continue
			}
			name := model.name + "Union" + strconv.Itoa(index+1) + "Tag"
			model.embeddedUnions = append(model.embeddedUnions, &embeddedUnion{
				name:        name,
				privateName: privateIdentifier(name),
				pointer:     model.ref.pointer + "/allOf/" + strconv.Itoa(index),
				keyword:     "oneOf",
				branches:    len(arrayValue(item["oneOf"])),
			})
		}
		return nil
	}
	if node["type"] == "array" {
		model.category = "array"
		items, _ := node["items"].(map[string]any)
		spec, err := g.valueSpec(schemaRef{
			doc:     model.ref.doc,
			node:    items,
			pointer: model.ref.pointer + "/items",
		}, model.name+"Item", false)
		if err != nil {
			return err
		}
		model.arrayElement = spec
		return nil
	}
	if node["type"] == "string" {
		model.category = "scalar"
		model.scalarType = "string"
		return nil
	}
	return fmt.Errorf("%s: unsupported named schema shape", model.ref.key())
}

func positiveUnion(node map[string]any, pointer string) (map[string]any, string, string) {
	if len(arrayValue(node["oneOf"])) > 0 {
		return node, pointer, "oneOf"
	}
	// AuditKindFieldRules is an allOf whose first member is the positive
	// discriminator union. Other allOf unions remain refinements on objects.
	if node["type"] == nil && node["properties"] == nil {
		for index, raw := range arrayValue(node["allOf"]) {
			item, _ := raw.(map[string]any)
			if len(arrayValue(item["oneOf"])) > 0 {
				return item, pointer + "/allOf/" + strconv.Itoa(index), "oneOf"
			}
		}
	}
	return nil, "", ""
}

func isObjectLike(g *generator, ref schemaRef, seen map[string]bool) bool {
	if ref.node["type"] == "object" || ref.node["properties"] != nil {
		return true
	}
	if target, ok := g.resolveRef(ref); ok {
		if seen[target.key()] {
			return false
		}
		seen[target.key()] = true
		return isObjectLike(g, target, seen)
	}
	for _, raw := range arrayValue(ref.node["allOf"]) {
		item, _ := raw.(map[string]any)
		child := schemaRef{doc: ref.doc, node: item, pointer: ref.pointer + "/allOf"}
		if isObjectLike(g, child, seen) {
			return true
		}
	}
	return false
}

func (g *generator) objectFields(ref schemaRef) ([]*fieldModel, bool, error) {
	fieldsByJSON := map[string]*fieldModel{}
	order := []string{}
	open := (ref.node["type"] == "object" || ref.node["properties"] != nil) &&
		ref.node["additionalProperties"] == nil &&
		ref.node["unevaluatedProperties"] != false
	var collect func(schemaRef, map[string]bool) error
	collect = func(current schemaRef, seen map[string]bool) error {
		if target, ok := g.resolveRef(current); ok {
			if !seen[target.key()] {
				next := copyBoolMap(seen)
				next[target.key()] = true
				if err := collect(target, next); err != nil {
					return err
				}
			}
		}
		required := stringSet(current.node["required"])
		properties, _ := current.node["properties"].(map[string]any)
		for _, jsonName := range sortedMapKeys(properties) {
			raw, _ := properties[jsonName].(map[string]any)
			propertyRef := schemaRef{
				doc:     current.doc,
				node:    raw,
				pointer: current.pointer + "/properties/" + escapePointer(jsonName),
			}
			existing := fieldsByJSON[jsonName]
			if existing == nil {
				goName := goIdentifier(jsonName)
				existing = &fieldModel{
					jsonName:  jsonName,
					goName:    goName,
					private:   privateIdentifier(goName),
					ref:       propertyRef,
					required:  required[jsonName],
					writeOnly: raw["writeOnly"] == true,
				}
				fieldsByJSON[jsonName] = existing
				order = append(order, jsonName)
			} else if required[jsonName] {
				existing.required = true
			}
		}
		for index, raw := range arrayValue(current.node["allOf"]) {
			item, _ := raw.(map[string]any)
			if item["$ref"] == nil {
				continue
			}
			child := schemaRef{
				doc:     current.doc,
				node:    item,
				pointer: current.pointer + "/allOf/" + strconv.Itoa(index),
			}
			if err := collect(child, copyBoolMap(seen)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(ref, map[string]bool{ref.key(): true}); err != nil {
		return nil, false, err
	}
	sort.Strings(order)
	fields := make([]*fieldModel, 0, len(order))
	for _, jsonName := range order {
		field := fieldsByJSON[jsonName]
		spec, err := g.valueSpec(field.ref, typeOwnerName(ref, g)+field.goName, true)
		if err != nil {
			return nil, false, err
		}
		if field.writeOnly {
			spec = &valueSpec{goType: "transportSecret", builder: "buildTransportSecret", transport: true}
		}
		field.spec = spec
		fields = append(fields, field)
	}
	return fields, open, nil
}

func typeOwnerName(ref schemaRef, g *generator) string {
	if model := g.modelsByRef[ref.key()]; model != nil {
		return model.name
	}
	return "Inline"
}

func (g *generator) valueSpec(ref schemaRef, suggested string, primaryEnum bool) (*valueSpec, error) {
	if target, ok := g.resolveRef(ref); ok {
		name := g.typeNames[target.key()]
		if name == "" {
			return nil, fmt.Errorf("%s: unresolved generated type for ref", ref.key())
		}
		switch name {
		case "JsonObject":
			return &valueSpec{goType: name, builder: "buildJsonObjectField"}, nil
		case "JsonValue":
			return &valueSpec{goType: name, builder: "buildJsonValueField"}, nil
		case "UntrustedJsonObject":
			return &valueSpec{goType: name, builder: "buildUntrustedJsonObjectField"}, nil
		case "UntrustedJsonValue":
			return &valueSpec{goType: name, builder: "buildUntrustedJsonValueField"}, nil
		}
		model := g.modelsByRef[target.key()]
		if model == nil {
			var err error
			model, err = g.ensureModel(target, name, false)
			if err != nil {
				return nil, err
			}
		}
		return &valueSpec{goType: model.name, builder: "build" + model.name, model: model}, nil
	}
	if values, err := stringEnum(ref.node); err == nil && len(values) > 0 {
		enumName := suggested
		if !primaryEnum {
			enumName += "Value"
		}
		enum := g.ensureEnum(enumName, values)
		return &valueSpec{goType: enum.name, builder: "build" + enum.name, enum: enum}, nil
	}
	if _, ok := ref.node["const"]; ok {
		switch ref.node["const"].(type) {
		case string:
			return &valueSpec{goType: "string", builder: "buildStringField"}, nil
		case bool:
			return &valueSpec{goType: "bool", builder: "buildBooleanField"}, nil
		case json.Number:
			return &valueSpec{goType: "JSONNumber", builder: "buildNumberField"}, nil
		}
	}
	switch kind := ref.node["type"].(type) {
	case string:
		switch kind {
		case "string":
			return &valueSpec{goType: "string", builder: "buildStringField"}, nil
		case "boolean":
			return &valueSpec{goType: "bool", builder: "buildBooleanField"}, nil
		case "number", "integer":
			return &valueSpec{goType: "JSONNumber", builder: "buildNumberField"}, nil
		case "array":
			items, _ := ref.node["items"].(map[string]any)
			element, err := g.valueSpec(schemaRef{
				doc:     ref.doc,
				node:    items,
				pointer: ref.pointer + "/items",
			}, suggested+"Item", false)
			if err != nil {
				return nil, err
			}
			return &valueSpec{goType: "[]" + element.goType, slice: true, element: element}, nil
		case "object":
			if len(mapValue(ref.node["properties"])) == 0 {
				if additional, _ := ref.node["additionalProperties"].(map[string]any); additional != nil {
					if additional["$ref"] == "#/$defs/UntrustedJsonValue" {
						return &valueSpec{goType: "UntrustedJsonObject", builder: "buildUntrustedJsonObjectField"}, nil
					}
				}
				if ref.node["additionalProperties"] != false {
					return &valueSpec{goType: "JsonObject", builder: "buildJsonObjectField"}, nil
				}
			}
			model, err := g.ensureModel(ref, suggested, false)
			if err != nil {
				return nil, err
			}
			return &valueSpec{goType: model.name, builder: "build" + model.name, model: model}, nil
		case "null":
			return &valueSpec{goType: "JsonValue", builder: "buildJsonValueField"}, nil
		}
	}
	if len(arrayValue(ref.node["oneOf"])) > 0 {
		model, err := g.ensureModel(ref, suggested, false)
		if err != nil {
			return nil, err
		}
		return &valueSpec{goType: model.name, builder: "build" + model.name, model: model}, nil
	}
	if ref.node["properties"] != nil {
		model, err := g.ensureModel(ref, suggested, false)
		if err != nil {
			return nil, err
		}
		return &valueSpec{goType: model.name, builder: "build" + model.name, model: model}, nil
	}
	return nil, fmt.Errorf("%s: unsupported field schema", ref.key())
}

func (g *generator) resolveRef(ref schemaRef) (schemaRef, bool) {
	raw, _ := ref.node["$ref"].(string)
	const prefix = "#/$defs/"
	if !strings.HasPrefix(raw, prefix) {
		return schemaRef{}, false
	}
	name := unescapePointer(strings.TrimPrefix(raw, prefix))
	node, ok := ref.doc.defs[name].(map[string]any)
	if !ok {
		return schemaRef{}, false
	}
	return schemaRef{
		doc:     ref.doc,
		node:    node,
		pointer: "#/$defs/" + escapePointer(name),
	}, true
}

func (g *generator) ensureEnum(name string, values []string) *enumModel {
	name = g.uniqueTypeName(name, "enum:"+name+":"+strings.Join(values, "\x00"))
	if existing := g.enumsByName[name]; existing != nil {
		return existing
	}
	enum := &enumModel{name: name, values: append([]string(nil), values...)}
	g.enumsByName[name] = enum
	g.enums = append(g.enums, enum)
	return enum
}

func (g *generator) uniqueTypeName(name, identity string) string {
	name = typeIdentifier(name)
	if prior, ok := g.usedTypeNames[name]; !ok || prior == identity {
		g.usedTypeNames[name] = identity
		return name
	}
	suffixed := name + shortHash(identity)
	g.usedTypeNames[suffixed] = identity
	return suffixed
}

func stringEnum(node map[string]any) ([]string, error) {
	raw := arrayValue(node["enum"])
	if len(raw) == 0 {
		return nil, fmt.Errorf("not a string enum")
	}
	values := make([]string, len(raw))
	for index, value := range raw {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("enum contains non-string value")
		}
		values[index] = text
	}
	return values, nil
}

func unionBranchLabel(node map[string]any, index int) string {
	if ref, _ := node["$ref"].(string); ref != "" {
		return typeIdentifier(unescapePointer(ref[strings.LastIndex(ref, "/")+1:]))
	}
	properties := mapValue(node["properties"])
	for _, preferred := range []string{"record_type", "kind", "type", "scope", "trigger", "subject", "event_kind"} {
		property, _ := properties[preferred].(map[string]any)
		if property == nil {
			continue
		}
		if value, ok := property["const"].(string); ok {
			return exportedIdentifier(value)
		}
		if ref, _ := property["$ref"].(string); ref != "" {
			return typeIdentifier(unescapePointer(ref[strings.LastIndex(ref, "/")+1:]))
		}
		if values, err := stringEnum(property); err == nil && len(values) > 0 {
			return exportedIdentifier(values[0])
		}
	}
	return "Variant" + strconv.Itoa(index+1)
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func arrayValue(value any) []any {
	result, _ := value.([]any)
	return result
}

func stringSet(value any) map[string]bool {
	result := map[string]bool{}
	for _, item := range arrayValue(value) {
		if text, ok := item.(string); ok {
			result[text] = true
		}
	}
	return result
}

func copyBoolMap(value map[string]bool) map[string]bool {
	result := make(map[string]bool, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func unescapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~")
}

var initialisms = map[string]string{
	"api": "API", "http": "HTTP", "id": "ID", "ip": "IP", "jcs": "JCS",
	"json": "JSON", "nats": "NATS", "rpo": "RPO", "rto": "RTO",
	"sha256": "SHA256", "utc": "UTC", "uuid": "UUID", "wal": "WAL",
}

func goIdentifier(value string) string {
	parts := splitIdentifier(value)
	var out strings.Builder
	for _, part := range parts {
		lower := strings.ToLower(part)
		if initial := initialisms[lower]; initial != "" {
			out.WriteString(initial)
			continue
		}
		runes := []rune(lower)
		if len(runes) == 0 {
			continue
		}
		out.WriteRune(unicode.ToUpper(runes[0]))
		out.WriteString(string(runes[1:]))
	}
	result := out.String()
	if result == "" {
		return "Value"
	}
	if result[0] >= '0' && result[0] <= '9' {
		result = "Value" + result
	}
	return result
}

func exportedIdentifier(value string) string {
	result := goIdentifier(value)
	if result == "" {
		return "Generated"
	}
	return result
}

// typeIdentifier preserves already-composed Go names such as WebSocketReauth
// and JSONValue. JSON spellings such as "access_token" still pass through the
// mechanical identifier conversion used for fields and enum literals.
func typeIdentifier(value string) string {
	if token.IsIdentifier(value) {
		runes := []rune(value)
		if len(runes) > 0 && unicode.IsUpper(runes[0]) {
			return value
		}
	}
	return exportedIdentifier(value)
}

func splitIdentifier(value string) []string {
	var parts []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			parts = append(parts, string(current))
			current = nil
		}
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			flush()
		}
	}
	flush()
	return parts
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func privateIdentifier(value string) string {
	if value == "" {
		return "value"
	}
	allUpper := true
	for _, r := range value {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			allUpper = false
			break
		}
	}
	result := lowerFirst(value)
	if allUpper {
		result = strings.ToLower(value)
	}
	switch result {
	case "break", "default", "func", "interface", "select", "case", "defer",
		"go", "map", "struct", "chan", "else", "goto", "package", "switch",
		"const", "fallthrough", "if", "range", "type", "continue", "for",
		"import", "return", "var":
		return result + "Value"
	default:
		return result
	}
}

func shortHash(value string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return fmt.Sprintf("%08x", hash.Sum32())
}

func canonicalSchema(node map[string]any) string {
	var buffer bytes.Buffer
	encodeCanonical(&buffer, node)
	return buffer.String()
}

func encodeCanonical(buffer *bytes.Buffer, value any) {
	switch item := value.(type) {
	case nil:
		buffer.WriteString("null")
	case bool:
		if item {
			buffer.WriteString("true")
		} else {
			buffer.WriteString("false")
		}
	case string:
		raw, _ := json.Marshal(item)
		buffer.Write(raw)
	case json.Number:
		buffer.WriteString(string(item))
	case []any:
		buffer.WriteByte('[')
		for index, child := range item {
			if index > 0 {
				buffer.WriteByte(',')
			}
			encodeCanonical(buffer, child)
		}
		buffer.WriteByte(']')
	case map[string]any:
		buffer.WriteByte('{')
		keys := sortedMapKeys(item)
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			raw, _ := json.Marshal(key)
			buffer.Write(raw)
			buffer.WriteByte(':')
			encodeCanonical(buffer, item[key])
		}
		buffer.WriteByte('}')
	default:
		panic(fmt.Sprintf("unsupported canonical generator value %T", value))
	}
}
