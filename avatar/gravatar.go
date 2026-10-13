// SPDX-License-Identifier: Apache-2.0

package avatar

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// gravatarHost is where Gravatar serves avatars.
const gravatarHost = "https://secure.gravatar.com/avatar/"

// phpSpace is what PHP's trim strips.
const phpSpace = " \t\n\r\x00\x0b"

// gravatarAddress returns the Gravatar address of email under query.
func gravatarAddress(email, query string) string {
	return gravatarHost + emailHash(email) + "?" + query
}

// gravatarQuery returns the query WordPress sends Gravatar, in its order: size, default image, rating, initials.
func gravatarQuery(name string, size int, options Options) string {
	image := cmp.Or(options.Default, DefaultMystery)
	query := "s=" + strconv.Itoa(size)
	if sent := sentDefault(image); sent != "" {
		query += "&d=" + rawURLEncode(sent)
	}
	query += "&r=" + strings.ToLower(string(cmp.Or(options.Rating, RatingG)))
	return query + initialsArgument(name, image)
}

// sentDefault returns the default image as Gravatar names it, nothing for the Gravatar logo.
func sentDefault(image Default) string {
	switch image {
	case DefaultMystery:
		return "mm"
	case DefaultGravatarLogo:
		return ""
	default:
		return string(image)
	}
}

// initialsArgument returns the initials WordPress sends with Gravatar's initials image, nothing otherwise.
func initialsArgument(name string, image Default) string {
	if image != DefaultInitials {
		return ""
	}
	letters := gravatarInitials(strings.TrimSpace(name))
	if letters == "" {
		return ""
	}
	return "&initials=" + rawURLEncode(letters)
}

// gravatarInitials returns up to two letters of name as WordPress picks them for Gravatar.
func gravatarInitials(name string) string {
	last := strings.LastIndex(name, " ")
	if last < 0 || strings.ContainsFunc(name, isEastAsian) {
		return firstRunes(name, 2)
	}
	return firstRunes(name, 1) + firstRunes(name[last+1:], 1)
}

// isEastAsian reports whether r is Han, Hiragana, Katakana or Hangul.
func isEastAsian(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// firstRunes returns the first count characters of s, or all of s when it is shorter.
func firstRunes(s string, count int) string {
	runes := []rune(s)
	return string(runes[:min(count, len(runes))])
}

// emailHash returns the SHA-256 hash of email trimmed and lowercased as PHP's trim and strtolower do.
func emailHash(email string) string {
	sum := sha256.Sum256(asciiLower(strings.Trim(email, phpSpace)))
	return hex.EncodeToString(sum[:])
}

// asciiLower returns the bytes of s with only A to Z lowercased.
func asciiLower(s string) []byte {
	lowered := []byte(s)
	for i, b := range lowered {
		if 'A' <= b && b <= 'Z' {
			lowered[i] = b + 'a' - 'A'
		}
	}
	return lowered
}

// rawURLEncode returns value encoded as PHP's rawurlencode encodes it.
func rawURLEncode(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
