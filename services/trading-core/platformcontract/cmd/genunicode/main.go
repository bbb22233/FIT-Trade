// Command genunicode generates the Unicode tables used to reproduce the
// ECMAScript authority-field normalization boundary without a runtime
// dependency. The authoritative contract runs on Node.js Unicode 17.0.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const requiredUnicodeVersion = "17.0"

const nodeProgram = `
const expectedVersion = process.argv[1];
if (process.versions.unicode !== expectedVersion) {
  throw new Error(
    "Unicode version mismatch: Node provides " + process.versions.unicode +
    ", generator requires " + expectedVersion
  );
}

const codePoints = (value) =>
  Array.from(value, (item) => item.codePointAt(0));
const scalar = (value) =>
  value >= 0 && value <= 0x10ffff &&
  !(value >= 0xd800 && value <= 0xdfff);

const decompositions = [];
for (let value = 0; value <= 0x10ffff; value += 1) {
  if (!scalar(value) || (value >= 0xac00 && value <= 0xd7a3)) continue;
  const input = String.fromCodePoint(value);
  const normalized = input.normalize("NFKD");
  if (normalized !== input) {
    decompositions.push([value, codePoints(normalized)]);
  }
}

// Canonical combining classes are inferred from the normalization operation
// itself. U+0334 has the lowest non-zero class and U+0345 the highest. A class
// zero scalar blocks reordering between those probes; a non-zero scalar does
// not. Pairwise NFD ordering then gives the complete stable total preorder.
const lowest = "\u0334";
const highest = "\u0345";
const combining = [];
for (let value = 0; value <= 0x10ffff; value += 1) {
  if (!scalar(value)) continue;
  const input = String.fromCodePoint(value);
  if (input.normalize("NFKD") !== input) continue;
  const probe = highest + input + lowest;
  if (probe.normalize("NFD") !== probe) combining.push(value);
}
const compareClass = (leftValue, rightValue) => {
  if (leftValue === rightValue) return 0;
  const left = String.fromCodePoint(leftValue);
  const right = String.fromCodePoint(rightValue);
  const leftRight = (left + right).normalize("NFD");
  const rightLeft = (right + left).normalize("NFD");
  if (leftRight === left + right && rightLeft === right + left) return 0;
  if (leftRight === left + right && rightLeft === left + right) return -1;
  if (leftRight === right + left && rightLeft === right + left) return 1;
  throw new Error(
    "unable to order canonical combining classes U+" +
    leftValue.toString(16) + " and U+" + rightValue.toString(16)
  );
};
combining.sort(compareClass);
const combiningClasses = [];
let classNumber = 0;
let prior = -1;
for (const value of combining) {
  if (prior < 0 || compareClass(prior, value) !== 0) classNumber += 1;
  combiningClasses.push([value, classNumber]);
  prior = value;
}

// A canonically composable scalar's final composition step is the NFC form of
// its NFD prefix plus the last decomposed scalar. Hangul is handled
// algorithmically by the Go runtime and is intentionally excluded here.
const compositions = [];
for (let value = 0; value <= 0x10ffff; value += 1) {
  if (!scalar(value) || (value >= 0xac00 && value <= 0xd7a3)) continue;
  const input = String.fromCodePoint(value);
  const nfd = input.normalize("NFD");
  if (nfd === input || nfd.normalize("NFC") !== input) continue;
  const parts = Array.from(nfd);
  const last = parts.pop();
  const prefix = parts.join("").normalize("NFC");
  if (Array.from(prefix).length !== 1 ||
      (prefix + last).normalize("NFC") !== input) {
    throw new Error("unable to derive composition for U+" + value.toString(16));
  }
  compositions.push([
    prefix.codePointAt(0),
    last.codePointAt(0),
    value,
  ]);
}

// After NFKC, ECMAScript default lowercasing can contribute ASCII for the
// ordinary ASCII repertoire and U+0130. Generate the non-ASCII exceptions
// instead of relying on that observation remaining true.
const lowerASCII = [];
for (let value = 0x80; value <= 0x10ffff; value += 1) {
  if (!scalar(value)) continue;
  const input = String.fromCodePoint(value);
  if (input.normalize("NFKC") !== input) continue;
  const projected = input.toLowerCase().replace(/[^a-z0-9]/gu, "");
  if (projected !== "") lowerASCII.push([value, projected]);
}

process.stdout.write(JSON.stringify({
  unicodeVersion: process.versions.unicode,
  decompositions,
  combiningClasses,
  compositions,
  lowerASCII,
}));
`

type generatedData struct {
	UnicodeVersion   string         `json:"unicodeVersion"`
	Decompositions   []runeSequence `json:"decompositions"`
	CombiningClasses []runeClass    `json:"combiningClasses"`
	Compositions     []composition  `json:"compositions"`
	LowerASCII       []runeString   `json:"lowerASCII"`
}

type runeSequence struct {
	Rune     int
	Sequence []int
}

func (r *runeSequence) UnmarshalJSON(data []byte) error {
	var value []json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil || len(value) != 2 {
		return fmt.Errorf("invalid decomposition entry")
	}
	if err := json.Unmarshal(value[0], &r.Rune); err != nil {
		return err
	}
	return json.Unmarshal(value[1], &r.Sequence)
}

type runeClass struct {
	Rune  int
	Class int
}

func (r *runeClass) UnmarshalJSON(data []byte) error {
	var value [2]int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	r.Rune, r.Class = value[0], value[1]
	return nil
}

type composition struct {
	First, Second, Composed int
}

func (c *composition) UnmarshalJSON(data []byte) error {
	var value [3]int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	c.First, c.Second, c.Composed = value[0], value[1], value[2]
	return nil
}

type runeString struct {
	Rune  int
	Value string
}

func (r *runeString) UnmarshalJSON(data []byte) error {
	var value []json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil || len(value) != 2 {
		return fmt.Errorf("invalid lowercase entry")
	}
	if err := json.Unmarshal(value[0], &r.Rune); err != nil {
		return err
	}
	return json.Unmarshal(value[1], &r.Value)
}

func main() {
	check := flag.Bool("check", false, "verify that generated output is current")
	output := flag.String("out", "", "generated Go output path")
	node := flag.String("node", "node", "Node.js executable")
	flag.Parse()

	outPath, err := resolveOutput(*output)
	if err != nil {
		fatal(err)
	}
	command := exec.Command(*node, "-e", nodeProgram, requiredUnicodeVersion)
	raw, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			fatal(fmt.Errorf("Node Unicode authority failed: %w: %s", err, exit.Stderr))
		}
		fatal(fmt.Errorf("Node Unicode authority unavailable: %w", err))
	}
	var data generatedData
	if err := json.Unmarshal(raw, &data); err != nil {
		fatal(fmt.Errorf("decode Node Unicode data: %w", err))
	}
	if data.UnicodeVersion != requiredUnicodeVersion {
		fatal(fmt.Errorf("Unicode version %q, want %q", data.UnicodeVersion, requiredUnicodeVersion))
	}
	formatted, err := render(data, raw)
	if err != nil {
		fatal(err)
	}
	if *check {
		current, err := os.ReadFile(outPath)
		if err != nil {
			fatal(err)
		}
		if !bytes.Equal(current, formatted) {
			fatal(fmt.Errorf("%s is stale; run go generate", outPath))
		}
		return
	}
	if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
		fatal(err)
	}
}

func resolveOutput(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	for _, candidate := range []string{
		"unicode_nfkc_gen.go",
		filepath.Join("platformcontract", "unicode_nfkc_gen.go"),
		filepath.Join("services", "trading-core", "platformcontract", "unicode_nfkc_gen.go"),
	} {
		if _, err := os.Stat(filepath.Dir(candidate)); err == nil {
			if filepath.Base(filepath.Dir(candidate)) == "platformcontract" ||
				filepath.Base(candidate) == "unicode_nfkc_gen.go" &&
					filepath.Base(mustGetwd()) == "platformcontract" {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("cannot locate platformcontract output; pass -out")
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func render(data generatedData, source []byte) ([]byte, error) {
	var out bytes.Buffer
	sum := sha256.Sum256(source)
	fmt.Fprintln(&out, "// Code generated by cmd/genunicode; DO NOT EDIT.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "package platformcontract")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "const generatedUnicodeVersion = %q\n", data.UnicodeVersion)
	fmt.Fprintln(&out, `const generatedUnicodeSource = "ECMAScript String.prototype.normalize(NFKD/NFD/NFC) and String.prototype.toLowerCase via Node.js"`)
	fmt.Fprintf(&out, "const generatedUnicodeDataSHA256 = %q\n", fmt.Sprintf("%x", sum))
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "var generatedNFKD = map[rune]string{")
	for _, entry := range data.Decompositions {
		var value []rune
		for _, item := range entry.Sequence {
			value = append(value, rune(item))
		}
		fmt.Fprintf(&out, "\t0x%X: %s,\n", entry.Rune, strconv.QuoteToASCII(string(value)))
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "var generatedCombiningClass = map[rune]uint8{")
	for _, entry := range data.CombiningClasses {
		fmt.Fprintf(&out, "\t0x%X: %d,\n", entry.Rune, entry.Class)
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "var generatedCompositions = map[uint64]rune{")
	for _, entry := range data.Compositions {
		key := uint64(entry.First)<<21 | uint64(entry.Second)
		fmt.Fprintf(&out, "\t0x%X: 0x%X,\n", key, entry.Composed)
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "var generatedLowerASCII = map[rune]string{")
	for _, entry := range data.LowerASCII {
		fmt.Fprintf(&out, "\t0x%X: %s,\n", entry.Rune, strconv.QuoteToASCII(entry.Value))
	}
	fmt.Fprintln(&out, "}")

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}
	return formatted, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genunicode:", err)
	os.Exit(1)
}
