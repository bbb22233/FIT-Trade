package api

import (
	"bytes"
	"encoding/json"
)

// serverOwnedFields is the sole production copy of the frozen
// transaction-boundaries-v1.json server_owned_fields set. Keep this list at
// exact manifest parity; endpoint-specific schemas still reject their own
// unknown fields independently.
var serverOwnedFields = map[string]struct{}{
	"user_id": {}, "trading_account_id": {}, "session_id": {}, "device_id": {},
	"source_key": {}, "source_key_digest": {}, "server_received_at": {}, "server_time": {},
	"token_digest": {}, "family_id": {}, "family_created_at": {}, "family_deadline": {},
	"refresh_family_id": {}, "rotated_to_digest": {}, "canonical_request_digest": {},
	"device_verification": {}, "identity_origin": {}, "signature_verified": {},
}

// strictJSON detects duplicate keys at every depth before endpoint decoding.
func strictJSON(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	if !strictValue(d) {
		return false
	}
	var extra any
	return d.Decode(&extra) != nil
}

func strictValue(d *json.Decoder) bool {
	token, err := d.Token()
	if err != nil {
		return false
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			seen := map[string]struct{}{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok {
					return false
				}
				if _, duplicate := seen[name]; duplicate {
					return false
				}
				if name == "__proto__" || name == "constructor" || name == "prototype" {
					return false
				}
				seen[name] = struct{}{}
				if !strictValue(d) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !strictValue(d) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		}
	}
	return true
}

func hasAuthorityField(raw []byte) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	return containsAuthority(value)
}

func containsAuthority(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			if isServerOwnedAlias(key) || containsAuthority(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if containsAuthority(nested) {
				return true
			}
		}
	}
	return false
}

// isServerOwnedAlias recognizes the ASCII spellings of a frozen field without
// admitting a second authority list. ASCII letters are case-insensitive and
// underscores/hyphens are spelling separators, so snake, camel, Pascal and
// hyphen forms all canonicalize to the manifest field's compact form.
func isServerOwnedAlias(field string) bool {
	canonical := make([]byte, 0, len(field))
	for i := 0; i < len(field); i++ {
		character := field[i]
		switch {
		case character >= 'a' && character <= 'z':
			canonical = append(canonical, character)
		case character >= 'A' && character <= 'Z':
			canonical = append(canonical, character+('a'-'A'))
		case character == '_' || character == '-':
			continue
		default:
			return false
		}
	}
	for owned := range serverOwnedFields {
		if compactAuthorityField(owned) == string(canonical) {
			return true
		}
	}
	return false
}

func compactAuthorityField(field string) string {
	compact := make([]byte, 0, len(field))
	for i := 0; i < len(field); i++ {
		if field[i] != '_' {
			compact = append(compact, field[i])
		}
	}
	return string(compact)
}
