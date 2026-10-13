// SPDX-License-Identifier: Apache-2.0

package avatar_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/gopherium/gouncer/avatar"
)

// gravatarBase is the Gravatar address of reader@example.com, before its query.
const gravatarBase = "https://secure.gravatar.com/avatar/" +
	"d108b279434fe1d54ac0f1da633564604b26c2e0e221d108b0fbadb87aba02c0"

// gravatar draws through Gravatar with the WordPress default image and rating.
var gravatar = avatar.Options{Mode: avatar.ModeGravatar}

// drawing is the part of an initials SVG the tests read.
type drawing struct {
	Width   string      `xml:"width,attr"`
	Height  string      `xml:"height,attr"`
	ViewBox string      `xml:"viewBox,attr"`
	Circle  circle      `xml:"circle"`
	Text    *letterText `xml:"text"`
}

// circle is the round fill of an initials SVG.
type circle struct {
	Fill string `xml:"fill,attr"`
	R    string `xml:"r,attr"`
}

// letterText is the letter an initials SVG draws.
type letterText struct {
	Fill       string `xml:"fill,attr"`
	FontSize   string `xml:"font-size,attr"`
	FontWeight string `xml:"font-weight,attr"`
	Letter     string `xml:",chardata"`
}

// drawn decodes an initials address into its drawing and its SVG text, ending the test when it is not one.
func drawn(t *testing.T, address string) (drawing, string) {
	t.Helper()
	encoded, ok := strings.CutPrefix(address, "data:image/svg+xml;base64,")
	if !ok {
		t.Fatalf("Address() = %q, want a base64 SVG data address", address)
	}
	svg, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("the address does not decode as base64: %v", err)
	}
	var image drawing
	if err := xml.Unmarshal(svg, &image); err != nil {
		t.Fatalf("the SVG does not parse: %v\n%s", err, svg)
	}
	return image, string(svg)
}

// letterOf returns the letter a drawing shows, or nothing when it draws none.
func letterOf(image drawing) string {
	if image.Text == nil {
		return ""
	}
	return image.Text.Letter
}

// paint is the circle and letter colours of a drawing.
type paint struct {
	circle, letter string
}

// paintOf returns the colours a drawing uses, the letter's empty when it draws none.
func paintOf(image drawing) paint {
	if image.Text == nil {
		return paint{circle: image.Circle.Fill}
	}
	return paint{circle: image.Circle.Fill, letter: image.Text.Fill}
}

func TestInitialsPaintTheColourPairGodminPicks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want paint
	}{
		{"Ana Lopez", paint{"#3b59e4", "#f7ecbd"}},
		{"Maria Perez", paint{"#c6396e", "#ecfaf4"}},
		{"another@example.com", paint{"#964c9a", "#deefdd"}},
		{"reader@example.com", paint{"#007017", "#ffffff"}},
		{"Luis Garcia", paint{"#996800", "#ffffff"}},
		{"Admin", paint{"#50575e", "#ffffff"}},
	}
	for _, tt := range tests {
		image, _ := drawn(t, avatar.Address(tt.name, "", 32, avatar.Options{}))
		if got := paintOf(image); got != tt.want {
			t.Errorf("Address(%q) paints %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestInitialsKeepTheirColourWhateverTheCase(t *testing.T) {
	t.Parallel()

	lower, _ := drawn(t, avatar.Address("maria perez", "", 32, avatar.Options{}))
	upper, _ := drawn(t, avatar.Address("MARIA PEREZ", "", 32, avatar.Options{}))
	if paintOf(lower) != paintOf(upper) {
		t.Errorf("the two cases paint %+v and %+v, want the same", paintOf(lower), paintOf(upper))
	}
}

func TestInitialsLowercaseAsGodminDoesBeforeTheyPickAColour(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, circle string
	}{
		{"ΜΑΡΙΑ ΠΕΡΕΣ", "#964c9a"},
		{"Μαρια Περες", "#964c9a"},
		{"ΣΑΣ", "#964c9a"},
		{"ΣΑΣ.", "#3b59e4"},
		{"ΣΑΣ.Α", "#996800"},
		{"Σ", "#007017"},
		{"Α.Σ", "#007017"},
		{"Α\U00000301Σ", "#3b59e4"},
		{"ΑΣ\U00000301", "#3b59e4"},
		{"ΑΣ Α", "#996800"},
		{"İMaria Perez", "#964c9a"},
	}
	for _, tt := range tests {
		image, _ := drawn(t, avatar.Address(tt.name, "", 32, avatar.Options{}))
		if got := image.Circle.Fill; got != tt.circle {
			t.Errorf("Address(%q) paints the circle %s, want %s as godmin does", tt.name, got, tt.circle)
		}
	}
}

func TestInitialsAllocateAboutAlikeForAThousandFinalSigmas(t *testing.T) {
	plain := testing.AllocsPerRun(20, func() { avatar.Address(strings.Repeat("ΑΒ ", 1000), "", 32, avatar.Options{}) })
	sigmas := testing.AllocsPerRun(20, func() { avatar.Address(strings.Repeat("ΑΣ ", 1000), "", 32, avatar.Options{}) })
	if sigmas > 2*plain {
		t.Errorf("Address() allocates %v times for 1000 final sigmas and %v for none, want them about alike", sigmas, plain)
	}
}

func TestInitialsTrimTheNameAsGodminDoes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, letter string
		want         paint
	}{
		{"\U0000FEFFMaria Perez", "M", paint{"#c6396e", "#ecfaf4"}},
		{"\u0085Maria Perez", "\u0085", paint{"#007017", "#ffffff"}},
	}
	for _, tt := range tests {
		image, _ := drawn(t, avatar.Address(tt.name, "", 32, avatar.Options{}))
		if got := paintOf(image); got != tt.want {
			t.Errorf("Address(%q) paints %+v, want %+v", tt.name, got, tt.want)
		}
		if got := letterOf(image); got != tt.letter {
			t.Errorf("Address(%q) draws %q, want %q", tt.name, got, tt.letter)
		}
	}
}

func TestInitialsDrawTheFirstLetterOfTheNameInCapitals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, letter string
	}{
		{"maria perez", "M"},
		{"  ana lopez", "A"},
		{"étude account", "É"},
		{"\U0001D49C account", "\U0001D49C"},
		{"   ", ""},
		{"", ""},
	}
	for _, tt := range tests {
		image, _ := drawn(t, avatar.Address(tt.name, "", 32, avatar.Options{}))
		if got := letterOf(image); got != tt.letter {
			t.Errorf("Address(%q) draws %q, want %q", tt.name, got, tt.letter)
		}
	}
}

func TestABlankNameDrawsTheFirstColourAndNoLetter(t *testing.T) {
	t.Parallel()

	image, _ := drawn(t, avatar.Address("   ", "", 32, avatar.Options{}))
	if got := paintOf(image); got != (paint{circle: "#3b59e4"}) {
		t.Errorf("Address() of a blank name paints %+v, want the circle #3b59e4 alone", got)
	}
}

func TestInitialsEscapeTheLetter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, letter, escaped string
	}{
		{"<b>", "<", "&lt;"},
		{"&co", "&", "&amp;"},
		{"\"quoted\"", "\"", "&#34;"},
		{"\x01control", "�", "�"},
	}
	for _, tt := range tests {
		image, svg := drawn(t, avatar.Address(tt.name, "", 32, avatar.Options{}))
		if got := letterOf(image); got != tt.letter {
			t.Errorf("Address(%q) draws %q, want %q", tt.name, got, tt.letter)
		}
		if !strings.Contains(svg, ">"+tt.escaped+"</text>") {
			t.Errorf("Address(%q) wrote %s, want the letter written as %s", tt.name, svg, tt.escaped)
		}
	}
}

func TestInitialsAddressesAreBase64SoNoFragmentCutsThem(t *testing.T) {
	t.Parallel()

	address := avatar.Address("<Maria Perez>", "", 32, avatar.Options{})
	if strings.ContainsAny(address, "#<>\" ") {
		t.Errorf("Address() = %q, want base64 with no #, angle bracket, quote or space", address)
	}
}

func TestInitialsScaleToTheSizeWithTheLetterAtTwelveThirtySecondsInSemibold(t *testing.T) {
	t.Parallel()

	image, _ := drawn(t, avatar.Address("Maria Perez", "", 48, avatar.Options{}))
	got := []string{image.Width, image.Height, image.ViewBox, image.Circle.R}
	if image.Text != nil {
		got = append(got, image.Text.FontSize, image.Text.FontWeight)
	}
	want := []string{"48", "48", "0 0 32 32", "16", "12", "600"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("width, height, viewBox, radius, font size and weight = %q, want %q", got, want)
	}
}

func TestInitialsLeaveTheEmailOut(t *testing.T) {
	t.Parallel()

	_, svg := drawn(t, avatar.Address("Maria Perez", "reader@example.com", 32, avatar.Options{}))
	if strings.Contains(svg, "reader") || strings.Contains(svg, "d108b279") {
		t.Errorf("the SVG %s carries the email or its hash, want neither", svg)
	}
}

func TestTheInitialsModeDrawsWhatTheZeroOptionsDraw(t *testing.T) {
	t.Parallel()

	initials := avatar.Address("Maria Perez", "reader@example.com", 32, avatar.Options{Mode: avatar.ModeInitials})
	if zero := avatar.Address("Maria Perez", "reader@example.com", 32, avatar.Options{}); initials != zero {
		t.Errorf("ModeInitials gives %q, the zero options %q, want the same", initials, zero)
	}
}

func TestGravatarAddressesFollowWordPress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		what    string
		options avatar.Options
		want    string
	}{
		{"the WordPress defaults", gravatar, gravatarBase + "?s=48&d=mm&r=g"},
		{"the mystery person rated G",
			avatar.Options{Mode: avatar.ModeGravatar, Default: avatar.DefaultMystery, Rating: avatar.RatingG},
			gravatarBase + "?s=48&d=mm&r=g"},
		{"the Gravatar logo",
			avatar.Options{Mode: avatar.ModeGravatar, Default: avatar.DefaultGravatarLogo},
			gravatarBase + "?s=48&r=g"},
		{"an identicon rated PG",
			avatar.Options{Mode: avatar.ModeGravatar, Default: avatar.DefaultIdenticon, Rating: avatar.RatingPG},
			gravatarBase + "?s=48&d=identicon&r=pg"},
		{"Gravatar's initials rated X",
			avatar.Options{Mode: avatar.ModeGravatar, Default: avatar.DefaultInitials, Rating: avatar.RatingX},
			gravatarBase + "?s=48&d=initials&r=x&initials=MP"},
		{"a rating holding an ampersand",
			avatar.Options{Mode: avatar.ModeGravatar, Rating: "G&D=X"},
			gravatarBase + "?s=48&d=mm&r=g%26d%3Dx"},
	}
	for _, tt := range tests {
		if got := avatar.Address("Maria Perez", "reader@example.com", 48, tt.options); got != tt.want {
			t.Errorf("Address() under %s = %q, want %q", tt.what, got, tt.want)
		}
	}
}

func TestEachDefaultImageSendsTheValueWordPressSends(t *testing.T) {
	t.Parallel()

	tests := map[avatar.Default]string{
		avatar.DefaultMystery:      "&d=mm",
		avatar.DefaultBlank:        "&d=blank",
		avatar.DefaultGravatarLogo: "",
		avatar.DefaultIdenticon:    "&d=identicon",
		avatar.DefaultWavatar:      "&d=wavatar",
		avatar.DefaultMonsterID:    "&d=monsterid",
		avatar.DefaultRetro:        "&d=retro",
		avatar.DefaultRoboHash:     "&d=robohash",
		avatar.DefaultInitials:     "&d=initials",
		avatar.DefaultColor:        "&d=color",
	}
	for image, sent := range tests {
		options := avatar.Options{Mode: avatar.ModeGravatar, Default: image}
		want := gravatarBase + "?s=48" + sent + "&r=g"
		if got := avatar.Address("", "reader@example.com", 48, options); got != want {
			t.Errorf("Address() with the default %q = %q, want %q", image, got, want)
		}
	}
}

func TestGravatarHashesTheEmailTrimmedAndLowercasedAsWordPressDoes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		email, hashed string
	}{
		{"reader@example.com", "reader@example.com"},
		{" Reader@Example.COM\n", "reader@example.com"},
		{"ÉCOLE@example.com", "École@example.com"},
	}
	for _, tt := range tests {
		sum := sha256.Sum256([]byte(tt.hashed))
		want := "https://secure.gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?s=48&d=mm&r=g"
		if got := avatar.Address("", tt.email, 48, gravatar); got != want {
			t.Errorf("Address() of %q = %q, want %q", tt.email, got, want)
		}
	}
}

func TestGravatarAddressesLeaveTheEmailOut(t *testing.T) {
	t.Parallel()

	if got := avatar.Address("Maria Perez", "reader@example.com", 48, gravatar); strings.Contains(got, "reader") {
		t.Errorf("Address() = %q, want the email only as its hash", got)
	}
}

func TestGravatarInitialsFollowWordPress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, sent string
	}{
		{"Maria Perez", "&initials=MP"},
		{"Maria  Perez", "&initials=MP"},
		{"alpha bravo charlie", "&initials=ac"},
		{"Maria", "&initials=Ma"},
		{"M", "&initials=M"},
		{"Étude account", "&initials=%C3%89a"},
		{"漢字テスト", "&initials=%E6%BC%A2%E5%AD%97"},
		{"漢字 テスト", "&initials=%E6%BC%A2%E5%AD%97"},
		{"a+b", "&initials=a%2B"},
		{"", ""},
		{"   ", ""},
	}
	options := avatar.Options{Mode: avatar.ModeGravatar, Default: avatar.DefaultInitials}
	for _, tt := range tests {
		want := gravatarBase + "?s=48&d=initials&r=g" + tt.sent
		if got := avatar.Address(tt.name, "reader@example.com", 48, options); got != want {
			t.Errorf("Address(%q) = %q, want %q", tt.name, got, want)
		}
	}
}

func TestTheOffModeDrawsNothing(t *testing.T) {
	t.Parallel()

	if got := avatar.Address("Maria Perez", "reader@example.com", 48, avatar.Options{Mode: avatar.ModeOff}); got != "" {
		t.Errorf("Address() in mode off = %q, want none", got)
	}
}

func TestASizeOutsideTheBoundsComesToTheNearestBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		size int
		want string
	}{
		{0, "?s=1&"},
		{-5, "?s=1&"},
		{1, "?s=1&"},
		{2048, "?s=2048&"},
		{4096, "?s=2048&"},
	}
	for _, tt := range tests {
		if got := avatar.Address("", "reader@example.com", tt.size, gravatar); !strings.Contains(got, tt.want) {
			t.Errorf("Address() at size %d = %q, want it to hold %q", tt.size, got, tt.want)
		}
	}
}

// parseCase is a value one Parse function reads, and what it should answer.
type parseCase struct {
	value, want string
	refused     error
}

// parsed is what one Parse function answered for one value.
type parsed struct {
	value string
	err   error
}

// expectParsed fails t unless the case's value parsed to want, or else to an error wrapping refused that quotes it.
func expectParsed(t *testing.T, tc parseCase, got parsed) {
	t.Helper()
	if tc.refused == nil {
		if got.err != nil || got.value != tc.want {
			t.Errorf("parsing %q = %q, %v, want %q", tc.value, got.value, got.err, tc.want)
		}
		return
	}
	if !errors.Is(got.err, tc.refused) || !strings.Contains(fmt.Sprint(got.err), strconv.Quote(tc.value)) {
		t.Errorf("parsing %q error = %v, want %v naming the value", tc.value, got.err, tc.refused)
	}
}

func TestParseModeKnowsTheThreeModes(t *testing.T) {
	t.Parallel()

	tests := []parseCase{
		{"", "initials", nil},
		{"initials", "initials", nil},
		{"gravatar", "gravatar", nil},
		{"off", "off", nil},
		{"Gravatar", "", avatar.ErrUnknownMode},
		{"none", "", avatar.ErrUnknownMode},
	}
	for _, tt := range tests {
		mode, err := avatar.ParseMode(tt.value)
		expectParsed(t, tt, parsed{string(mode), err})
	}
}

func TestParseDefaultKnowsTheTenWordPressDefaults(t *testing.T) {
	t.Parallel()

	tests := []parseCase{
		{"", "mystery", nil},
		{"mystery", "mystery", nil},
		{"blank", "blank", nil},
		{"gravatar_default", "gravatar_default", nil},
		{"identicon", "identicon", nil},
		{"wavatar", "wavatar", nil},
		{"monsterid", "monsterid", nil},
		{"retro", "retro", nil},
		{"robohash", "robohash", nil},
		{"initials", "initials", nil},
		{"color", "color", nil},
		{"mm", "", avatar.ErrUnknownDefault},
		{"Identicon", "", avatar.ErrUnknownDefault},
		{"https://example.com/default.png", "", avatar.ErrUnknownDefault},
	}
	for _, tt := range tests {
		image, err := avatar.ParseDefault(tt.value)
		expectParsed(t, tt, parsed{string(image), err})
	}
}

func TestParseRatingKnowsTheFourRatings(t *testing.T) {
	t.Parallel()

	tests := []parseCase{
		{"", "G", nil},
		{"G", "G", nil},
		{"PG", "PG", nil},
		{"R", "R", nil},
		{"X", "X", nil},
		{"g", "", avatar.ErrUnknownRating},
		{"NC-17", "", avatar.ErrUnknownRating},
	}
	for _, tt := range tests {
		rating, err := avatar.ParseRating(tt.value)
		expectParsed(t, tt, parsed{string(rating), err})
	}
}

func TestParseSizeKeepsSizesFromOneTo2048(t *testing.T) {
	t.Parallel()

	tests := []parseCase{
		{"", "96", nil},
		{"1", "1", nil},
		{"48", "48", nil},
		{"2048", "2048", nil},
		{"0", "", avatar.ErrSize},
		{"2049", "", avatar.ErrSize},
		{"-48", "", avatar.ErrSize},
		{"48px", "", avatar.ErrSize},
	}
	for _, tt := range tests {
		size, err := avatar.ParseSize(tt.value)
		expectParsed(t, tt, parsed{strconv.Itoa(size), err})
	}
}
