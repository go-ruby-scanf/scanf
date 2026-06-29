// Copyright (c) the go-ruby-scanf/scanf authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package scanf is a pure-Go (no cgo) reimplementation of Ruby's scanf stdlib —
// the deterministic, interpreter-independent core of MRI 4.0.5's String#scanf /
// IO#scanf. It parses values out of an input string according to a scanf format
// string, the inverse of sprintf, and returns the converted values.
//
// It mirrors MRI's bundled scanf gem (v1.0.0) directive-for-directive: %d/%u and
// %i (with 0x/0 base detection), %x/%X/%o hex & octal integers, %a/%e/%f/%g
// floats, %s whitespace-delimited strings, %c single/width chars, %[...] and
// %[^...] character-class sets, %% literal percent, decimal field widths, the *
// assignment-suppression flag, whitespace in the format meaning "skip optional
// whitespace", and literal characters that must match the input. Conversions
// produce Go int / *big.Int / float64 / string, the natural mapping of Ruby's
// Integer / Float / String.
//
// Scanning stops — as in MRI — at the first input that fails to match the format,
// at end of input, or when the format is exhausted; the values converted up to
// that point are returned (a partial match), so a failed conversion is never an
// error, just a shorter result.
//
// It is a standalone, reusable module with no dependency on a Ruby runtime; it is
// the scanf backend for go-embedded-ruby and pairs with go-ruby-format (the
// sprintf side).
package scanf

// Scan parses values out of input according to format, matching MRI's
// String#scanf called without a block: it runs the format once and returns the
// converted values it produced. A directive that fails to match ends the scan,
// so the result holds only the values converted before that point (a partial
// match yields a shorter slice; no match yields an empty slice). The returned
// error is always nil — scanf reports a non-match by returning fewer values, not
// by failing — but it is part of the signature so the idiomatic Go shape (and an
// rbgo binding) need not special-case it.
//
//	Scan("abc 123", "%s %d")   // []any{"abc", int(123)}, nil
//	Scan("50%", "%d%%")        // []any{int(50)}, nil
//	Scan("42 abc", "%d %d")    // []any{int(42)}, nil  (partial)
func Scan(input, format string) ([]any, error) {
	fs := newFormatString(format)
	return fs.match(input), nil
}

// ScanAll repeatedly applies format to input, matching MRI's String#scanf called
// with a block (and String#block_scanf): each pass converts one group of values,
// then scanning resumes from where the previous pass stopped, until the input is
// exhausted or a pass converts nothing. Every non-empty group is appended to the
// result, so ScanAll returns one inner slice per successful pass.
//
//	ScanAll("1 2 3 4", "%d %d")  // [][]any{{1, 2}, {3, 4}}, nil
//	ScanAll("1 2 3", "%d")       // [][]any{{1}, {2}, {3}}, nil
//
// As with Scan the error is always nil.
func ScanAll(input, format string) ([][]any, error) {
	fs := newFormatString(format)
	var out [][]any
	str := input
	for {
		cur := fs.match(str)
		if len(cur) > 0 {
			out = append(out, cur)
		}
		str = fs.stringLeft
		if len(cur) == 0 || str == "" {
			break
		}
	}
	return out, nil
}
