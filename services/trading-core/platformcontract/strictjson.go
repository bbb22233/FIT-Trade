package platformcontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"
)

// ParseStrictJSON accepts exactly one I-JSON value.  In particular it does
// not use encoding/json's permissive object decoder: duplicate keys and lone
// UTF-16 surrogate escapes are rejected before a Go value is constructed.
// Numbers remain json.Number; callers therefore never receive float64 through
// an untrusted decode path.
func ParseStrictJSON(raw []byte) (any, error) {
	p := strictParser{raw: raw}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i != len(raw) {
		return nil, p.err("trailing bytes")
	}
	return v, nil
}

type strictParser struct {
	raw []byte
	i   int
}

func (p *strictParser) err(s string) error {
	return fmt.Errorf("platformcontract: strict JSON at byte %d: %s", p.i, s)
}
func (p *strictParser) space() {
	for p.i < len(p.raw) && (p.raw[p.i] == ' ' || p.raw[p.i] == '\t' || p.raw[p.i] == '\r' || p.raw[p.i] == '\n') {
		p.i++
	}
}
func (p *strictParser) value() (any, error) {
	p.space()
	if p.i == len(p.raw) {
		return nil, p.err("missing value")
	}
	switch p.raw[p.i] {
	case '{':
		return p.object()
	case '[':
		return p.array()
	case '"':
		return p.string()
	case 't':
		if p.literal("true") {
			return true, nil
		}
	case 'f':
		if p.literal("false") {
			return false, nil
		}
	case 'n':
		if p.literal("null") {
			return nil, nil
		}
	}
	start := p.i
	if p.raw[p.i] == '-' {
		p.i++
	}
	if p.i >= len(p.raw) {
		return nil, p.err("invalid number")
	}
	if p.raw[p.i] == '0' {
		p.i++
	} else if p.raw[p.i] >= '1' && p.raw[p.i] <= '9' {
		for p.i < len(p.raw) && p.raw[p.i] >= '0' && p.raw[p.i] <= '9' {
			p.i++
		}
	} else {
		return nil, p.err("invalid value")
	}
	if p.i < len(p.raw) && p.raw[p.i] == '.' {
		p.i++
		d := p.i
		for p.i < len(p.raw) && p.raw[p.i] >= '0' && p.raw[p.i] <= '9' {
			p.i++
		}
		if d == p.i {
			return nil, p.err("invalid number")
		}
	}
	if p.i < len(p.raw) && (p.raw[p.i] == 'e' || p.raw[p.i] == 'E') {
		p.i++
		if p.i < len(p.raw) && (p.raw[p.i] == '+' || p.raw[p.i] == '-') {
			p.i++
		}
		d := p.i
		for p.i < len(p.raw) && p.raw[p.i] >= '0' && p.raw[p.i] <= '9' {
			p.i++
		}
		if d == p.i {
			return nil, p.err("invalid number")
		}
	}
	n := json.Number(p.raw[start:p.i])
	if _, ok := parseFiniteJSONNumber(string(n)); !ok {
		return nil, p.err("number is not I-JSON")
	}
	return n, nil
}
func (p *strictParser) literal(s string) bool {
	if len(p.raw)-p.i >= len(s) && string(p.raw[p.i:p.i+len(s)]) == s {
		p.i += len(s)
		return true
	}
	return false
}

func hex4(b []byte) (rune, bool) {
	if len(b) != 4 {
		return 0, false
	}
	var n rune
	for _, c := range b {
		n <<= 4
		switch {
		case c >= '0' && c <= '9':
			n += rune(c - '0')
		case c >= 'a' && c <= 'f':
			n += rune(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			n += rune(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return n, true
}
func (p *strictParser) string() (string, error) {
	p.i++ // quote
	buf := make([]byte, 0, 16)
	for p.i < len(p.raw) {
		c := p.raw[p.i]
		if c == '"' {
			p.i++
			return string(buf), nil
		}
		if c < 0x20 {
			return "", p.err("control character in string")
		}
		if c != '\\' {
			r, size := utf8.DecodeRune(p.raw[p.i:])
			if r == utf8.RuneError && size == 1 {
				return "", p.err("invalid UTF-8")
			}
			buf = append(buf, p.raw[p.i:p.i+size]...)
			p.i += size
			continue
		}
		p.i++
		if p.i == len(p.raw) {
			return "", p.err("unfinished escape")
		}
		switch p.raw[p.i] {
		case '"', '\\', '/':
			buf = append(buf, p.raw[p.i])
			p.i++
		case 'b':
			buf = append(buf, '\b')
			p.i++
		case 'f':
			buf = append(buf, '\f')
			p.i++
		case 'n':
			buf = append(buf, '\n')
			p.i++
		case 'r':
			buf = append(buf, '\r')
			p.i++
		case 't':
			buf = append(buf, '\t')
			p.i++
		case 'u':
			p.i++
			if p.i+4 > len(p.raw) {
				return "", p.err("short unicode escape")
			}
			r, ok := hex4(p.raw[p.i : p.i+4])
			if !ok {
				return "", p.err("invalid unicode escape")
			}
			p.i += 4
			if r >= 0xD800 && r <= 0xDBFF {
				if p.i+6 > len(p.raw) || p.raw[p.i] != '\\' || p.raw[p.i+1] != 'u' {
					return "", p.err("unpaired high surrogate")
				}
				lo, ok := hex4(p.raw[p.i+2 : p.i+6])
				if !ok || lo < 0xDC00 || lo > 0xDFFF {
					return "", p.err("unpaired high surrogate")
				}
				p.i += 6
				r = 0x10000 + (r-0xD800)*0x400 + (lo - 0xDC00)
			} else if r >= 0xDC00 && r <= 0xDFFF {
				return "", p.err("unpaired low surrogate")
			}
			buf = utf8.AppendRune(buf, r)
		default:
			return "", p.err("invalid escape")
		}
	}
	return "", p.err("unterminated string")
}
func (p *strictParser) object() (any, error) {
	p.i++
	p.space()
	out := map[string]any{}
	if p.i < len(p.raw) && p.raw[p.i] == '}' {
		p.i++
		return out, nil
	}
	for {
		p.space()
		if p.i >= len(p.raw) || p.raw[p.i] != '"' {
			return nil, p.err("object key expected")
		}
		k, err := p.string()
		if err != nil {
			return nil, err
		}
		if _, ok := out[k]; ok {
			return nil, p.err("duplicate object key")
		}
		if k == "__proto__" || k == "constructor" || k == "prototype" {
			return nil, p.err("unsafe object key")
		}
		p.space()
		if p.i >= len(p.raw) || p.raw[p.i] != ':' {
			return nil, p.err("object colon expected")
		}
		p.i++
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out[k] = v
		p.space()
		if p.i < len(p.raw) && p.raw[p.i] == '}' {
			p.i++
			return out, nil
		}
		if p.i >= len(p.raw) || p.raw[p.i] != ',' {
			return nil, p.err("object comma expected")
		}
		p.i++
	}
}
func (p *strictParser) array() (any, error) {
	p.i++
	p.space()
	out := []any{}
	if p.i < len(p.raw) && p.raw[p.i] == ']' {
		p.i++
		return out, nil
	}
	for {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		p.space()
		if p.i < len(p.raw) && p.raw[p.i] == ']' {
			p.i++
			return out, nil
		}
		if p.i >= len(p.raw) || p.raw[p.i] != ',' {
			return nil, p.err("array comma expected")
		}
		p.i++
	}
}

// validJSONNumberLexeme recognizes the RFC 8259 number grammar. It is shared
// with JCS so values assembled inside this package cannot accidentally
// canonicalize Go/JavaScript numeric extensions such as NaN, +1, or 0x1.
func validJSONNumberLexeme(value string) bool {
	if value == "" {
		return false
	}
	index := 0
	if value[index] == '-' {
		index++
		if index == len(value) {
			return false
		}
	}
	switch {
	case value[index] == '0':
		index++
	case value[index] >= '1' && value[index] <= '9':
		index++
		for index < len(value) && value[index] >= '0' && value[index] <= '9' {
			index++
		}
	default:
		return false
	}
	if index < len(value) && value[index] == '.' {
		index++
		start := index
		for index < len(value) && value[index] >= '0' && value[index] <= '9' {
			index++
		}
		if index == start {
			return false
		}
	}
	if index < len(value) && (value[index] == 'e' || value[index] == 'E') {
		index++
		if index < len(value) && (value[index] == '+' || value[index] == '-') {
			index++
		}
		start := index
		for index < len(value) && value[index] >= '0' && value[index] <= '9' {
			index++
		}
		if index == start {
			return false
		}
	}
	return index == len(value)
}

// parseFiniteJSONNumber follows ECMAScript JSON number conversion. ParseFloat
// reports ErrRange for both finite underflow (which JSON.parse accepts as
// signed zero) and overflow (which I-JSON forbids as an infinity), so the
// converted value—not ErrRange alone—decides whether the number is usable.
func parseFiniteJSONNumber(value string) (float64, bool) {
	if !validJSONNumberLexeme(value) {
		return 0, false
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func numberInt(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	// JSON Schema's integer type is mathematical, not lexical: 1, 1.0 and
	// 1e0 all become the same ECMAScript Number before AJV sees them.
	number, ok := parseFiniteJSONNumber(string(n))
	if !ok ||
		math.Trunc(number) != number ||
		number < -9223372036854775808.0 ||
		number >= 9223372036854775808.0 {
		return 0, false
	}
	return int64(number), true
}
