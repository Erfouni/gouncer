// SPDX-License-Identifier: Apache-2.0

// Package avatar builds the address of an account's avatar, drawn locally from its name or served by Gravatar.
package avatar

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// ErrUnknownMode reports a mode the package does not know.
var ErrUnknownMode = errors.New("avatar: unknown mode")

// ErrUnknownDefault reports a default image the package does not know.
var ErrUnknownDefault = errors.New("avatar: unknown default image")

// ErrUnknownRating reports a rating the package does not know.
var ErrUnknownRating = errors.New("avatar: unknown rating")

// ErrSize reports a size that is not a whole number from MinSize to MaxSize.
var ErrSize = errors.New("avatar: invalid size")

// Mode is how an avatar is drawn, any value but gravatar and off drawing initials.
type Mode string

// The modes an avatar is drawn in.
const (
	ModeInitials Mode = "initials"
	ModeGravatar Mode = "gravatar"
	ModeOff      Mode = "off"
)

// Default is the image Gravatar shows for an email that has none, named as WordPress names it.
type Default string

// The default images WordPress offers.
const (
	DefaultMystery      Default = "mystery"
	DefaultBlank        Default = "blank"
	DefaultGravatarLogo Default = "gravatar_default"
	DefaultIdenticon    Default = "identicon"
	DefaultWavatar      Default = "wavatar"
	DefaultMonsterID    Default = "monsterid"
	DefaultRetro        Default = "retro"
	DefaultRoboHash     Default = "robohash"
	DefaultInitials     Default = "initials"
	DefaultColor        Default = "color"
)

// Rating is the most mature Gravatar image an avatar may show.
type Rating string

// The ratings WordPress offers.
const (
	RatingG  Rating = "G"
	RatingPG Rating = "PG"
	RatingR  Rating = "R"
	RatingX  Rating = "X"
)

// The sizes an avatar is drawn at, in pixels.
const (
	MinSize     = 1
	MaxSize     = 2048
	DefaultSize = 96
)

// modes are the modes ParseMode knows, the default first.
var modes = []Mode{ModeInitials, ModeGravatar, ModeOff}

// defaults are the default images ParseDefault knows, the default first.
var defaults = []Default{
	DefaultMystery, DefaultBlank, DefaultGravatarLogo, DefaultIdenticon, DefaultWavatar, DefaultMonsterID,
	DefaultRetro, DefaultRoboHash, DefaultInitials, DefaultColor,
}

// ratings are the ratings ParseRating knows, the default first.
var ratings = []Rating{RatingG, RatingPG, RatingR, RatingX}

// Options is how a site draws its avatars, a zero Mode drawing initials and other zero fields their WordPress default.
type Options struct {
	Mode    Mode
	Default Default
	Rating  Rating
}

// ParseMode returns the mode value names, ModeInitials for an empty value.
func ParseMode(value string) (Mode, error) {
	return parse(value, modes, ErrUnknownMode)
}

// ParseDefault returns the default image value names, DefaultMystery for an empty value.
func ParseDefault(value string) (Default, error) {
	return parse(value, defaults, ErrUnknownDefault)
}

// ParseRating returns the rating value names, RatingG for an empty value.
func ParseRating(value string) (Rating, error) {
	return parse(value, ratings, ErrUnknownRating)
}

// ParseSize returns the size value holds, DefaultSize for an empty value.
func ParseSize(value string) (int, error) {
	if value == "" {
		return DefaultSize, nil
	}
	size, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w %q", ErrSize, value)
	}
	if size != bounded(size) {
		return 0, fmt.Errorf("%w %q", ErrSize, value)
	}
	return size, nil
}

// Address returns the avatar address of the account named name at email, size pixels within MinSize to MaxSize.
func Address(name, email string, size int, options Options) string {
	size = bounded(size)
	switch options.Mode {
	case ModeOff:
		return ""
	case ModeGravatar:
		return gravatarHost + emailHash(email) + "?" + gravatarQuery(name, size, options)
	default:
		return initialsAddress(name, size)
	}
}

// parse returns value as one of known, the first of them for an empty value, or refused naming the value.
func parse[T ~string](value string, known []T, refused error) (T, error) {
	if value == "" {
		return known[0], nil
	}
	if !slices.Contains(known, T(value)) {
		return "", fmt.Errorf("%w %q", refused, value)
	}
	return T(value), nil
}

// bounded returns size brought into MinSize to MaxSize.
func bounded(size int) int {
	return min(max(size, MinSize), MaxSize)
}
