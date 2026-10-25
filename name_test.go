// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
)

func TestAccountConstructorsCleanNames(t *testing.T) {
	t.Parallel()

	constructors := map[string]func(string) (gouncer.User, error){
		"NewUser": func(name string) (gouncer.User, error) {
			return gouncer.NewUser("ada@example.com", name, "correct horse battery")
		},
		"NewInvitedUser": func(name string) (gouncer.User, error) {
			return gouncer.NewInvitedUser("ada@example.com", name)
		},
	}
	tests := map[string]struct {
		name string
		want string
	}{
		"NUL dropped":                 {"Ada\x00 Lovelace", "Ada Lovelace"},
		"tabs and line breaks folded": {" \tAda  \r\n\t Lovelace\n ", "Ada Lovelace"},
		"other C0 controls dropped":   {"Ad\x01a\x1f Lovelace", "Ada Lovelace"},
		"Persian letters kept":        {"  آدا لاولیس  ", "آدا لاولیس"},
		"DEL and C1 kept inside": {
			"Ada\U0000007f\U00000085\U0000009b Lovelace", "Ada\U0000007f\U00000085\U0000009b Lovelace",
		},
		"bidi and zero width kept inside": {"Ada\U0000202e\U0000200b Lovelace", "Ada\U0000202e\U0000200b Lovelace"},
		"next line kept at the start":     {"\U00000085Ada", "\U00000085Ada"},
		"next line kept at the end":       {"Ada\U00000085", "Ada\U00000085"},
		"next line alone":                 {"\U00000085", "\U00000085"},
		"no-break spaces around next line": {
			" \U000000a0\U00000085Ada\U00000085\U000000a0 ", "\U00000085Ada\U00000085",
		},
		"no-break spaces at the ends":     {" \U000000a0Ada\U000000a0 ", "Ada"},
		"DEL kept at the ends":            {"\U0000007fAda\U0000007f", "\U0000007fAda\U0000007f"},
		"C1 kept at the ends":             {"\U00000080\U0000009bAda\U0000009f", "\U00000080\U0000009bAda\U0000009f"},
		"bidi and zero width at the ends": {"\U0000202eAda\U0000200b", "\U0000202eAda\U0000200b"},
		"256 letters after cleaning":      {strings.Repeat("a", 256) + "\x00", strings.Repeat("a", 256)},
		"256 two-byte letters":            {strings.Repeat("\U000000e9", 256), strings.Repeat("\U000000e9", 256)},
		"no-break space kept inside":      {"Ada\U000000a0Lovelace", "Ada\U000000a0Lovelace"},
		"line separator kept inside":      {"Ada\U00002028Lovelace", "Ada\U00002028Lovelace"},
		"control between two spaces":      {"Ada \x01 Lovelace", "Ada Lovelace"},
		"control before a space":          {"\x01 Ada", "Ada"},
	}

	for constructor, create := range constructors {
		t.Run(constructor, func(t *testing.T) {
			t.Parallel()

			for testName, tc := range tests {
				t.Run(testName, func(t *testing.T) {
					t.Parallel()

					u, err := create(tc.name)
					if err != nil {
						t.Fatalf("%s(%+q) error = %v, want nil", constructor, tc.name, err)
					}
					if u.Name != tc.want {
						t.Errorf("name = %+q, want %+q", u.Name, tc.want)
					}
				})
			}
		})
	}
}

func TestNewInvitedUserCleansEveryC0Control(t *testing.T) {
	t.Parallel()

	spaces := map[rune]string{'\t': " ", '\n': " ", '\r': " "}
	for control := rune(0); control < 0x20; control++ {
		t.Run(fmt.Sprintf("U+%04X", control), func(t *testing.T) {
			t.Parallel()

			want := "Ada" + spaces[control] + "Lovelace"
			u, err := gouncer.NewInvitedUser("ada@example.com", "Ada"+string(control)+"Lovelace")
			if err != nil {
				t.Fatalf("NewInvitedUser() error = %v, want nil", err)
			}
			if u.Name != want {
				t.Errorf("name = %+q, want %+q", u.Name, want)
			}
		})
	}
}
