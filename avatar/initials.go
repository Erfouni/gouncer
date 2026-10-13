// SPDX-License-Identifier: Apache-2.0

package avatar

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// colourPair is a circle colour and the colour its letter takes on it.
type colourPair struct {
	circle, letter string
}

// palette is godmin's initials avatar colours, in its order.
var palette = [...]colourPair{
	{"#3b59e4", "#f7ecbd"},
	{"#c6396e", "#ecfaf4"},
	{"#964c9a", "#deefdd"},
	{"#007017", "#ffffff"},
	{"#996800", "#ffffff"},
	{"#50575e", "#ffffff"},
}

// fontFamily is godmin's body font stack.
const fontFamily = "-apple-system, system-ui, 'Segoe UI', 'Roboto', 'Oxygen-Sans', 'Ubuntu', 'Cantarell', " +
	"'Helvetica Neue', sans-serif"

// initialsAddress returns the data address of a round SVG showing the first letter of name, size pixels wide.
func initialsAddress(name string, size int) string {
	name = strings.TrimFunc(name, isJavaScriptSpace)
	pair := palette[colourIndex(name)]
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 32 32">`+
		`<circle cx="16" cy="16" r="16" fill="%s"/>%s</svg>`, size, size, pair.circle, letterText(name, pair.letter))
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

// letterText returns the SVG text drawing the first letter of name in colour, nothing for an empty name.
func letterText(name, colour string) string {
	letter := firstLetter(name)
	if letter == "" {
		return ""
	}
	var escaped strings.Builder
	_ = xml.EscapeText(&escaped, []byte(letter))
	return fmt.Sprintf(`<text x="16" y="20.2" fill="%s" font-family="%s" font-size="12" font-weight="600" `+
		`text-anchor="middle">%s</text>`, colour, fontFamily, escaped.String())
}

// firstLetter returns the first character of name in capitals, nothing for an empty name.
func firstLetter(name string) string {
	first, width := utf8.DecodeRuneInString(name)
	if width == 0 {
		return ""
	}
	return string(unicode.ToUpper(first))
}

// colourIndex returns the palette entry name picks, whatever its case, as godmin picks it.
func colourIndex(name string) int {
	var hash int64
	for _, r := range lowerAsJavaScript([]rune(name)) {
		hash = (hash*31 + int64(r)) % 2147483647
	}
	return int(hash % int64(len(palette)))
}
