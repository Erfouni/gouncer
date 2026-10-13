// SPDX-License-Identifier: Apache-2.0

package avatar_test

import (
	"fmt"
	"strings"

	"github.com/gopherium/gouncer/avatar"
)

func ExampleAddress() {
	options := avatar.Options{Mode: avatar.ModeGravatar, Default: avatar.DefaultIdenticon, Rating: avatar.RatingPG}
	address, query, _ := strings.Cut(avatar.Address("Maria Perez", "reader@example.com", 48, options), "?")
	fmt.Println(address)
	fmt.Println(query)
	// Output:
	// https://secure.gravatar.com/avatar/d108b279434fe1d54ac0f1da633564604b26c2e0e221d108b0fbadb87aba02c0
	// s=48&d=identicon&r=pg
}

func ExampleParseMode() {
	mode, err := avatar.ParseMode("")
	fmt.Println(mode, err)
	_, err = avatar.ParseMode("none")
	fmt.Println(err)
	// Output:
	// initials <nil>
	// avatar: unknown mode "none"
}
