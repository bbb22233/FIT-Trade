package platformcontract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// CanonicalJSON serializes I-JSON according to RFC 8785 (JCS).  It returns an
// empty string for an invalid value; digest APIs return the underlying error.
func CanonicalJSON(v any) string {
	s, err := canonical(v)
	if err != nil {
		return ""
	}
	return s
}
func canonical(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "null", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case string:
		return jcsString(x)
	case json.Number:
		return jcsNumber(string(x))
	case []any:
		p := make([]string, len(x))
		for i, a := range x {
			q, err := canonical(a)
			if err != nil {
				return "", err
			}
			p[i] = q
		}
		return "[" + strings.Join(p, ",") + "]", nil
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			if !utf8.ValidString(k) {
				return "", fmt.Errorf("invalid I-JSON key")
			}
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		p := make([]string, 0, len(keys))
		for _, k := range keys {
			q, err := canonical(x[k])
			if err != nil {
				return "", err
			}
			ks, err := jcsString(k)
			if err != nil {
				return "", err
			}
			p = append(p, ks+":"+q)
		}
		return "{" + strings.Join(p, ",") + "}", nil
	default:
		return "", fmt.Errorf("unsupported canonical value %T", v)
	}
}
func utf16Less(a, b string) bool {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}
func jcsString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("invalid I-JSON string")
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte("0123456789abcdef"[r>>4])
				b.WriteByte("0123456789abcdef"[r&15])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}
func jcsNumber(n string) (string, error) {
	if !validJSONNumberLexeme(n) {
		return "", fmt.Errorf("invalid JSON number")
	}
	f, ok := parseFiniteJSONNumber(n)
	if !ok {
		return "", fmt.Errorf("non-I-JSON number")
	}
	if f == 0 {
		return "0", nil
	}
	a := math.Abs(f)
	if a >= 1e-6 && a < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	i := strings.IndexByte(s, 'e')
	mantissa, exponent := s[:i], s[i+1:]
	sign := ""
	if exponent[0] == '+' || exponent[0] == '-' {
		sign, exponent = exponent[:1], exponent[1:]
	}
	exponent = strings.TrimLeft(exponent, "0")
	if exponent == "" {
		exponent = "0"
	}
	return mantissa + "e" + sign + exponent, nil
}
func digest(v any) (string, error) {
	s, err := canonical(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s))), nil
}

func RequestDigest(input map[string]any) (string, error) {
	bound := map[string]any{}
	sv, ok := input["schema_version"]
	if !ok {
		return "", fmt.Errorf("platformcontract: missing schema_version")
	}
	if sv == "fit.platform.request-envelope.v1" {
		bound["schema_version"] = "fit.platform.request-digest.v1"
	} else {
		bound["schema_version"] = sv
	}
	for _, k := range []string{"method", "route_template", "path", "query", "body", "user_id", "trading_account_id"} {
		x, ok := input[k]
		if !ok {
			return "", fmt.Errorf("platformcontract: request digest missing %s", k)
		}
		bound[k] = x
	}
	m, _ := bound["method"].(string)
	if !asciiUpperMethod(m) {
		return "", fmt.Errorf("platformcontract: method must contain only A-Z")
	}
	return digest(bound)
}

func asciiUpperMethod(method string) bool {
	if method == "" {
		return false
	}
	for _, current := range []byte(method) {
		if current < 'A' || current > 'Z' {
			return false
		}
	}
	return true
}
func PayloadDigest(payload any) (string, error) { return digest(payload) }
func DurableRecordDigest(record map[string]any) (string, error) {
	c := make(map[string]any, len(record))
	for k, v := range record {
		if k != "payload_digest" {
			c[k] = v
		}
	}
	return digest(c)
}

func ParseConfirmationTarget(raw string) (map[string]any, map[string]any, error) {
	if raw == "" || raw[0] != '/' || strings.ContainsAny(raw, "#\\") {
		return nil, nil, fmt.Errorf("platformcontract: invalid request target")
	}
	for _, b := range []byte(raw) {
		if b < 0x21 || b > 0x7e {
			return nil, nil, fmt.Errorf("platformcontract: target must be visible ASCII")
		}
	}
	parts := strings.SplitN(raw, "?", 2)
	path := parts[0]
	if strings.Contains(path, "%") {
		return nil, nil, fmt.Errorf("platformcontract: encoded path forbidden")
	}
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[0] != "" {
		return nil, nil, fmt.Errorf("platformcontract: invalid path")
	}
	for _, s := range segments[1:] {
		if s == "" || s == "." || s == ".." {
			return nil, nil, fmt.Errorf("platformcontract: ambiguous path")
		}
		for _, b := range []byte(s) {
			if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._~-", rune(b)) {
				return nil, nil, fmt.Errorf("platformcontract: invalid path byte")
			}
		}
	}
	if len(parts) == 2 && parts[1] != "" {
		return nil, nil, fmt.Errorf("platformcontract: confirmation query forbidden")
	}
	const pre, suf = "/v1/confirmations/", "/consume"
	foldedPath := strings.ToLower(path)
	if !strings.HasPrefix(foldedPath, pre) || !strings.HasSuffix(foldedPath, suf) {
		return nil, nil, fmt.Errorf("platformcontract: route mismatch")
	}
	id := path[len(pre) : len(path)-len(suf)]
	if !uuidRE.MatchString(strings.ToLower(id)) {
		return nil, nil, fmt.Errorf("platformcontract: invalid confirmation id")
	}
	return map[string]any{"confirmation_id": strings.ToLower(id)}, map[string]any{}, nil
}
func ChallengeSigningBytes(challenge map[string]any) ([]byte, error) {
	d, ok := challenge["domain"].(string)
	if !ok || d == "" {
		return nil, fmt.Errorf("platformcontract: challenge domain missing")
	}
	c, err := canonical(challenge)
	if err != nil {
		return nil, err
	}
	return append(append([]byte(d), 0), []byte(c)...), nil
}
