// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// cleanName returns name with C0 controls dropped and spaces folded, or "" when name is not valid UTF-8.
func cleanName(name string) string {
	if !utf8.ValidString(name) {
		return ""
	}
	var cleaned strings.Builder
	cleaned.Grow(len(name))
	spaceDue := false
	for _, r := range name {
		switch {
		case strings.ContainsRune(" \t\n\r", r):
			spaceDue = cleaned.Len() > 0
		case r < 0x20:
			continue
		default:
			if spaceDue {
				cleaned.WriteByte(' ')
				spaceDue = false
			}
			cleaned.WriteRune(r)
		}
	}
	return strings.TrimFunc(cleaned.String(), isTrimmedSpace)
}

// isTrimmedSpace reports whether r is trimmed from the ends of a name.
func isTrimmedSpace(r rune) bool {
	return r != '\u0085' && unicode.IsSpace(r)
}
