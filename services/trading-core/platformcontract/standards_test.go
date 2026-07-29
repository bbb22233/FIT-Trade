package platformcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestRFC8785AppendixBNumbers(t *testing.T) {
	t.Parallel()
	vectors := []struct {
		bits uint64
		want string
	}{
		{0x0000000000000000, "0"},
		{0x8000000000000000, "0"},
		{0x0000000000000001, "5e-324"},
		{0x8000000000000001, "-5e-324"},
		{0x7fefffffffffffff, "1.7976931348623157e+308"},
		{0xffefffffffffffff, "-1.7976931348623157e+308"},
		{0x4340000000000000, "9007199254740992"},
		{0xc340000000000000, "-9007199254740992"},
		{0x4430000000000000, "295147905179352830000"},
		{0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{0x44b52d02c7e14af6, "1e+23"},
		{0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{0x444b1ae4d6e2ef4e, "999999999999999700000"},
		{0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{0x444b1ae4d6e2ef50, "1e+21"},
		{0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{0x3eb0c6f7a0b5ed8d, "0.000001"},
		{0x41b3de4355555553, "333333333.3333332"},
		{0x41b3de4355555554, "333333333.33333325"},
		{0x41b3de4355555555, "333333333.3333333"},
		{0x41b3de4355555556, "333333333.3333334"},
		{0x41b3de4355555557, "333333333.33333343"},
		{0xbecbf647612f3696, "-0.0000033333333333333333"},
		{0x43143ff3c1cb0959, "1424953923781206.2"},
	}
	for _, vector := range vectors {
		number := math.Float64frombits(vector.bits)
		input := strconv.FormatFloat(number, 'g', -1, 64)
		got, err := jcsNumber(input)
		if err != nil {
			t.Errorf("%016x: %v", vector.bits, err)
			continue
		}
		if got != vector.want {
			t.Errorf("%016x: JCS(%s) = %s, want %s", vector.bits, input, got, vector.want)
		}
	}
	for _, bits := range []uint64{0x7fffffffffffffff, 0x7ff0000000000000, 0xfff0000000000000} {
		input := strconv.FormatFloat(math.Float64frombits(bits), 'g', -1, 64)
		if _, err := jcsNumber(input); err == nil {
			t.Errorf("%016x: accepted non-finite %q", bits, input)
		}
	}
}

func TestRFC8785CanonicalObjectAndUTF16Ordering(t *testing.T) {
	t.Parallel()
	const input = `{
		"numbers": [333333333.33333329, 1E30, 4.50,
		            2e-3, 0.000000000000000000000000001],
		"string": "\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/",
		"literals": [null, true, false]
	}`
	const want = `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],"string":"€$\u000f\nA'B\"\\\\\"/"}`
	value, err := ParseStrictJSON([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got := CanonicalJSON(value); got != want {
		t.Fatalf("RFC 8785 sample:\n got %s\nwant %s", got, want)
	}

	ordering := map[string]any{
		"\u20ac": "Euro Sign",
		"\r":     "Carriage Return",
		"\ufb33": "Hebrew Letter Dalet With Dagesh",
		"1":      "One",
		"😀":      "Emoji: Grinning Face",
		"\u0080": "Control",
		"\u00f6": "Latin Small Letter O With Diaeresis",
	}
	const ordered = "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\",\"ö\":\"Latin Small Letter O With Diaeresis\",\"€\":\"Euro Sign\",\"😀\":\"Emoji: Grinning Face\",\"דּ\":\"Hebrew Letter Dalet With Dagesh\"}"
	if got := CanonicalJSON(ordering); got != ordered {
		t.Fatalf("UTF-16 ordering:\n got %s\nwant %s", got, ordered)
	}
}

func TestRFC8785ControlEscapingAndInvalidIJSON(t *testing.T) {
	t.Parallel()
	controls := make([]rune, 0, 37)
	for current := rune(0); current < 0x20; current++ {
		controls = append(controls, current)
	}
	controls = append(controls, '"', '\\', '/', '\u2028', '\u2029')
	const escapedControls = `"\u0000\u0001\u0002\u0003\u0004\u0005\u0006\u0007\b\t\n\u000b\f\r\u000e\u000f\u0010\u0011\u0012\u0013\u0014\u0015\u0016\u0017\u0018\u0019\u001a\u001b\u001c\u001d\u001e\u001f\"\\/`
	want := escapedControls + "\u2028\u2029" + `"`
	got, err := jcsString(string(controls))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("control escaping:\n got %q\nwant %q", got, want)
	}

	for _, raw := range [][]byte{
		[]byte(`"\ud800"`),
		[]byte(`"\udc00"`),
		[]byte(`{"a":1,"\u0061":2}`),
		[]byte(`1e9999`),
		{'"', 0xff, '"'},
	} {
		if _, err := ParseStrictJSON(raw); err == nil {
			t.Errorf("accepted invalid I-JSON %q", raw)
		}
	}
	for _, number := range []string{"01", "+1", ".1", "1.", "1e", "NaN", "Infinity", "-Infinity", "0x1"} {
		if _, err := jcsNumber(number); err == nil {
			t.Errorf("canonicalized invalid JSON number %q", number)
		}
	}
	for raw, want := range map[string]string{
		"1e-400000":        "0",
		"9007199254740993": "9007199254740992",
	} {
		value, err := ParseStrictJSON([]byte(raw))
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if got := CanonicalJSON(value); got != want {
			t.Errorf("JCS(%s) = %s, want %s", raw, got, want)
		}
	}
	decomposed, err := ParseStrictJSON([]byte(`"e\u0301"`))
	if err != nil {
		t.Fatal(err)
	}
	if got := CanonicalJSON(decomposed); got != `"é"` {
		t.Errorf("JCS altered Unicode normalization: %q", got)
	}
}

func TestJSONNumberBinary64Semantics(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"0", "-0", "1.0", "1e0", "1e-400000", "-1e-400000"} {
		value, err := ParseStrictJSON([]byte(raw))
		if err != nil {
			t.Errorf("ParseStrictJSON(%s): %v", raw, err)
			continue
		}
		number, ok := numberInt(value)
		if !ok {
			t.Errorf("numberInt(%s) rejected an ECMAScript mathematical integer", raw)
			continue
		}
		if raw != "1.0" && raw != "1e0" && number != 0 {
			t.Errorf("numberInt(%s) = %d, want 0", raw, number)
		}
		if (raw == "1.0" || raw == "1e0") && number != 1 {
			t.Errorf("numberInt(%s) = %d, want 1", raw, number)
		}
	}
	for _, raw := range []string{"0.1", "1e400000", "-1e400000"} {
		if value, err := ParseStrictJSON([]byte(raw)); err == nil {
			if _, ok := numberInt(value); ok {
				t.Errorf("numberInt(%s) accepted a fraction or overflow", raw)
			}
			if strings.Contains(raw, "400000") {
				t.Errorf("ParseStrictJSON(%s) accepted an infinite conversion", raw)
			}
		}
	}
}

func TestJCSNodeNumberDifferential(t *testing.T) {
	t.Parallel()
	bits := []uint64{
		0, 1, 0x8000000000000000, 0x7fefffffffffffff,
		math.Float64bits(math.Nextafter(1e-6, 0)),
		math.Float64bits(1e-6),
		math.Float64bits(math.Nextafter(1e-6, math.Inf(1))),
		math.Float64bits(math.Nextafter(1e21, 0)),
		math.Float64bits(1e21),
		math.Float64bits(math.Nextafter(1e21, math.Inf(1))),
	}
	state := uint64(0x9e3779b97f4a7c15)
	for len(bits) < 25000 {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		bits = append(bits, state)
	}
	var input strings.Builder
	for _, value := range bits {
		fmt.Fprintf(&input, "%016x\n", value)
	}
	const authority = `
const fs = require("fs");
const lines = fs.readFileSync(0, "utf8").trimEnd().split("\n");
for (const line of lines) {
  const bytes = Buffer.allocUnsafe(8);
  bytes.writeBigUInt64BE(BigInt("0x" + line));
  const value = bytes.readDoubleBE();
  process.stdout.write((Number.isFinite(value) ? JSON.stringify(value) : "#") + "\n");
}`
	command := exec.Command("node", "-e", authority)
	command.Stdin = strings.NewReader(input.String())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Node JCS authority unavailable: %v: %s", err, output)
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) != len(bits)+1 || lines[len(lines)-1] != "" {
		t.Fatalf("Node JCS authority returned %d lines for %d values", len(lines)-1, len(bits))
	}
	for index, rawBits := range bits {
		number := math.Float64frombits(rawBits)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			if lines[index] != "#" {
				t.Fatalf("%016x: Node accepted non-finite value as %q", rawBits, lines[index])
			}
			continue
		}
		input := strconv.FormatFloat(number, 'g', -1, 64)
		got, err := jcsNumber(input)
		if err != nil {
			t.Fatalf("%016x (%s): %v", rawBits, input, err)
		}
		if got != lines[index] {
			t.Fatalf("%016x (%s): Go %q, Node %q", rawBits, input, got, lines[index])
		}
	}
}

func TestAuthorityNormalizationCompatibilityAndOrdinaryUnicode(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"server_time":         "servertime",
		"ＳＥＲＶＥＲ＿ＴＩＭＥ":         "servertime",
		"ˢᵉʳᵛᵉʳ_time":         "servertime",
		"ⓢⓔⓡⓥⓔⓡ_time":         "servertime",
		"𝐬𝐞𝐫𝐯𝐞𝐫_time":         "servertime",
		"ſerver_time":         "servertime",
		"source_Key":          "sourcekey",
		"deⅴⅰce_id":           "deviceid",
		"server_time\u0338":   "servertime",
		"server_time\u0301":   "servertim",
		"ѕerver_time":         "ervertime",
		"café":                "caf",
		"İd":                  "id",
		"\u0345A\u0334\u0300": "",
		"\u1100\u1161\u11a8":  "",
		"\uac01":              "",
	} {
		if got := authorityAlias(input); got != want {
			t.Errorf("authorityAlias(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAuthorityNormalizationNodeParity(t *testing.T) {
	t.Parallel()
	if generatedUnicodeVersion != "17.0" {
		t.Fatalf("generated Unicode version = %s", generatedUnicodeVersion)
	}
	corpus := []string{
		"", "server_time", "ＳＥＲＶＥＲ＿ＴＩＭＥ", "ˢᵉʳᵛᵉʳ_time",
		"ⓢⓔⓡⓥⓔⓡ_time", "𝐬𝐞𝐫𝐯𝐞𝐫_time", "ſerver_time",
		"source_Key", "deⅴⅰce_id", "server_time\u0338",
		"server_time\u0301", "ѕerver_time", "café", "İd",
		"A\u030A", "\u0345A\u0334\u0300", "\u1100\u1161\u11a8", "\uac01",
	}
	decompositionKeys := make([]int, 0, len(generatedNFKD))
	for current := range generatedNFKD {
		decompositionKeys = append(decompositionKeys, int(current))
	}
	sort.Ints(decompositionKeys)
	for _, current := range decompositionKeys {
		corpus = append(corpus, string(rune(current)))
	}
	combiningKeys := make([]int, 0, len(generatedCombiningClass))
	for current := range generatedCombiningClass {
		combiningKeys = append(combiningKeys, int(current))
	}
	sort.Ints(combiningKeys)
	for _, current := range combiningKeys {
		mark := string(rune(current))
		corpus = append(corpus, "A"+mark+"\u030A", "\u0345"+mark+"\u0334")
	}
	random := rand.New(rand.NewSource(0x464954))
	for range 4096 {
		length := 1 + random.Intn(8)
		value := make([]rune, 0, length)
		for range length {
			for {
				current := rune(random.Intn(0x110000))
				if current < 0xd800 || current > 0xdfff {
					value = append(value, current)
					break
				}
			}
		}
		corpus = append(corpus, string(value))
	}

	var input bytes.Buffer
	for _, value := range corpus {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		input.Write(encoded)
		input.WriteByte('\n')
	}
	const authority = `
const fs = require("fs");
if (process.versions.unicode !== "17.0") {
  throw new Error("Node Unicode " + process.versions.unicode + ", want 17.0");
}
const alias = (value) =>
  value.normalize("NFKC").toLowerCase().replace(/[^a-z0-9]/gu, "");
for (const line of fs.readFileSync(0, "utf8").trimEnd().split("\n")) {
  process.stdout.write(alias(JSON.parse(line)) + "\n");
}`
	command := exec.Command("node", "-e", authority)
	command.Stdin = bytes.NewReader(input.Bytes())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Node Unicode authority unavailable: %v: %s", err, output)
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) != len(corpus)+1 || lines[len(lines)-1] != "" {
		t.Fatalf("Node Unicode authority returned %d lines for %d values", len(lines)-1, len(corpus))
	}
	for index, value := range corpus {
		if got := authorityAlias(value); got != lines[index] {
			t.Fatalf("case %d authorityAlias(%q) = %q, Node %q", index, value, got, lines[index])
		}
	}
}

func TestGeneratedUnicodeDrift(t *testing.T) {
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	command := exec.Command(goBinary, "run", "./cmd/genunicode", "-check")
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Unicode generator drift check failed: %v: %s", err, output)
	}
}
