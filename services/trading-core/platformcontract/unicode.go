package platformcontract

import (
	"strings"
	"unicode/utf8"
)

const (
	hangulSBase  = 0xAC00
	hangulLBase  = 0x1100
	hangulVBase  = 0x1161
	hangulTBase  = 0x11A7
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulNCount = hangulVCount * hangulTCount
	hangulSCount = hangulLCount * hangulNCount
)

// unicodeAuthorityAlias exactly implements the normalization projection used
// by the frozen Node authority:
//
//	value.normalize("NFKC").toLowerCase().replace(/[^a-z0-9]/gu, "")
//
// The generated data is pinned to Unicode 17.0. Invalid UTF-8 cannot enter
// through ParseStrictJSON; handling it here by dropping invalid bytes matches
// the fact that ECMAScript strings contain Unicode code units, not raw bytes.
func unicodeAuthorityAlias(value string) string {
	normalized := unicodeNFKC(strings.ToValidUTF8(value, ""))
	var out strings.Builder
	out.Grow(len(value))
	for _, current := range normalized {
		switch {
		case current >= 'A' && current <= 'Z':
			out.WriteByte(byte(current + ('a' - 'A')))
		case current >= 'a' && current <= 'z':
			out.WriteByte(byte(current))
		case current >= '0' && current <= '9':
			out.WriteByte(byte(current))
		default:
			if replacement := generatedLowerASCII[current]; replacement != "" {
				out.WriteString(replacement)
			}
		}
	}
	return out.String()
}

func unicodeNFKC(value string) []rune {
	decomposed := make([]rune, 0, utf8.RuneCountInString(value))
	for _, current := range value {
		if current >= hangulSBase && current < hangulSBase+hangulSCount {
			index := int(current - hangulSBase)
			decomposed = append(
				decomposed,
				rune(hangulLBase+index/hangulNCount),
				rune(hangulVBase+(index%hangulNCount)/hangulTCount),
			)
			if trailing := index % hangulTCount; trailing != 0 {
				decomposed = append(decomposed, rune(hangulTBase+trailing))
			}
			continue
		}
		if replacement, ok := generatedNFKD[current]; ok {
			decomposed = append(decomposed, []rune(replacement)...)
			continue
		}
		decomposed = append(decomposed, current)
	}
	reorderCanonical(decomposed)
	return composeCanonical(decomposed)
}

func reorderCanonical(value []rune) {
	segmentStart := 0
	for index := 0; index < len(value); index++ {
		current := value[index]
		class := generatedCombiningClass[current]
		if class == 0 {
			segmentStart = index + 1
			continue
		}
		insert := index
		for insert > segmentStart {
			priorClass := generatedCombiningClass[value[insert-1]]
			if priorClass == 0 || priorClass <= class {
				break
			}
			value[insert] = value[insert-1]
			insert--
		}
		value[insert] = current
	}
}

func composeCanonical(value []rune) []rune {
	if len(value) < 2 {
		return value
	}
	out := make([]rune, 0, len(value))
	starterIndex := -1
	var starter rune
	var priorClass uint8
	for _, current := range value {
		class := generatedCombiningClass[current]
		composed, ok := composePair(starter, current)
		if starterIndex >= 0 && ok && (priorClass == 0 || priorClass < class) {
			out[starterIndex] = composed
			starter = composed
			continue
		}
		out = append(out, current)
		if class == 0 {
			starterIndex = len(out) - 1
			starter = current
		}
		priorClass = class
	}
	return out
}

func composePair(first, second rune) (rune, bool) {
	// Unicode Standard Annex #15 algorithmic Hangul composition.
	if first >= hangulLBase && first < hangulLBase+hangulLCount &&
		second >= hangulVBase && second < hangulVBase+hangulVCount {
		lIndex := int(first - hangulLBase)
		vIndex := int(second - hangulVBase)
		return rune(hangulSBase + (lIndex*hangulVCount+vIndex)*hangulTCount), true
	}
	if first >= hangulSBase && first < hangulSBase+hangulSCount &&
		(first-hangulSBase)%hangulTCount == 0 &&
		second > hangulTBase && second < hangulTBase+hangulTCount {
		return first + (second - hangulTBase), true
	}
	composed, ok := generatedCompositions[uint64(first)<<21|uint64(second)]
	return composed, ok
}
