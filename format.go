// Copyright (c) the go-ruby-scanf/scanf authors
//
// SPDX-License-Identifier: BSD-3-Clause

package scanf

import "regexp"

// specifiers is MRI's set of conversion letters (FormatString::SPECIFIERS). %b is
// deliberately absent — MRI's scanf gem has no binary directive (a %b in a format
// is treated as the literal characters "%b", which match nothing useful), so we
// omit it to stay byte-for-byte faithful.
const specifiers = "diuXxofFeEgGscaA"

// specRegex tokenizes a format string into specs, mirroring FormatString::REGEX.
// Each match is one of: a %% / %<flags><width><conv> conversion (optionally
// preceded by whitespace that is absorbed and discarded, as MRI does), or a run
// of literal non-% non-space characters.
var specRegex = regexp.MustCompile(
	`(?:\s*%(?:%|\*?\d*(?:\[\[:\w+:\]\]|\[[^\]]*\]|[` + specifiers + `])))` +
		`|[^%\s]+`,
)

// formatString is a parsed scanf format: an ordered list of specs plus the
// match-loop state MRI exposes (the unconsumed input tail, after a match).
type formatString struct {
	specs      []*formatSpecifier
	stringLeft string
}

// newFormatString parses str into its specs. A format with no non-whitespace
// content has no specs and matches nothing, exactly as MRI returns early on a
// blank format.
func newFormatString(str string) *formatString {
	fs := &formatString{}
	if !containsNonSpace(str) {
		return fs
	}
	for _, tok := range specRegex.FindAllString(str, -1) {
		fs.specs = append(fs.specs, newFormatSpecifier(tok))
	}
	return fs
}

// match applies the specs left to right over str, eating the matched prefix from
// the working string as each spec succeeds, and stops at the first spec that
// fails to match or once the input is exhausted — MRI's FormatString#match. The
// converted values (suppressed specs and %% contribute nothing) are returned.
func (fs *formatString) match(str string) []any {
	var accum []any
	fs.stringLeft = str
	for _, spec := range fs.specs {
		m := spec.match(fs.stringLeft)
		if m == nil {
			break
		}
		if v, ok := spec.conversion(); ok {
			accum = append(accum, v)
		}
		fs.stringLeft = m.post
		if fs.stringLeft == "" {
			break
		}
	}
	return accum
}

// containsNonSpace reports whether s has any non-whitespace byte (MRI's
// /\S/.match guard). Only ASCII whitespace is relevant to scanf formats.
func containsNonSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isSpace(s[i]) {
			return true
		}
	}
	return false
}

// isSpace reports whether b is one of the ASCII whitespace bytes Ruby's \s and
// String#strip treat as space.
func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}
