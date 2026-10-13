// SPDX-License-Identifier: Apache-2.0

package avatar

import (
	"slices"
	"unicode"
)

// midLetters are the marks Unicode lets stand inside a word without ending it.
var midLetters = []rune{
	'\'', '.', ':', 0x00B7, 0x0387, 0x055F, 0x05F4, 0x2018, 0x2019, 0x2024, 0x2027, 0xFE13, 0xFE52, 0xFE55, 0xFF07,
	0xFF0E, 0xFF1A,
}

// isJavaScriptSpace reports whether JavaScript's trim strips r.
func isJavaScriptSpace(r rune) bool {
	switch r {
	case 0xFEFF:
		return true
	case 0x0085:
		return false
	default:
		return unicode.IsSpace(r)
	}
}

// lowerAsJavaScript returns runes lowercased as JavaScript's toLowerCase lowercases them.
func lowerAsJavaScript(runes []rune) []rune {
	lowered := make([]rune, 0, len(runes))
	for i, r := range runes {
		switch {
		case r == 0x0130:
			lowered = append(lowered, 'i', 0x0307)
		case r == 0x03A3 && endsWord(runes, i):
			lowered = append(lowered, 0x03C2)
		default:
			lowered = append(lowered, unicode.ToLower(r))
		}
	}
	return lowered
}

// endsWord reports whether a cased letter comes before the character at i and none comes after it in its word.
func endsWord(runes []rune, i int) bool {
	before := slices.Clone(runes[:i])
	slices.Reverse(before)
	return casedFirst(before) && !casedFirst(runes[i+1:])
}

// casedFirst reports whether the first character of runes that is not case-ignorable is a cased letter.
func casedFirst(runes []rune) bool {
	for _, r := range runes {
		if !caseIgnorable(r) {
			return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
		}
	}
	return false
}

// caseIgnorable reports whether Unicode lets r stand between a cased letter and a final sigma.
func caseIgnorable(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk) || slices.Contains(midLetters, r)
}
