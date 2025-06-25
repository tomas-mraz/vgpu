// Copyright (c) 2023, Cogent Core. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package enums provides common interfaces for enums
// and bit flag enums and utilities for using them
package enums

// BitFlag is the interface that all bit flag enum types
// satisfy. Bit flag enum types support all of the operations
// that standard enums do, and additionally can check if they
// have a given bit flag. Note that HasFlag is defined on
// [BitFlagSetter] since it requires a pointer receiver for
// atomic operations to prevent race conditions.
type BitFlag interface {
	Enum

	// BitIndexString returns the string
	// representation of the bit flag if
	// the bit flag is a bit index value
	// (typically an enum constant), and
	// not an actual bit flag value.
	BitIndexString() string
}

// BitFlagSetter is an expanded interface that all pointers
// to bit flag enum types satisfy. Pointers to bit flag
// enum types must satisfy all of the methods of [EnumSetter]
// and [BitFlag], and must also be able to set a given bit flag.
type BitFlagSetter interface {
	EnumSetter
	BitFlag

	// Has returns whether these flags
	// have the given flag set.
	HasFlag(f BitFlag) bool

	// Set sets the value of the given
	// flags in these flags to the given value.
	SetFlag(on bool, f ...BitFlag)

	// SetStringOr sets the bit flag from its
	// string representation while preserving any
	// bit flags already set, and returns an
	// error if the string is invalid.
	SetStringOr(s string) error
}
