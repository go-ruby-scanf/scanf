// Copyright (c) the go-ruby-scanf/scanf authors
//
// SPDX-License-Identifier: BSD-3-Clause

package scanf

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// hexClass is the character class for a hexadecimal digit, MRI's local `h`.
const hexClass = `[A-Fa-f0-9]`

// handler converts the captured submatch string into a scanf value. It returns
// (value, true) when the spec yields a value, or (nil, false) when the spec is
// suppressed (the * flag), is %%, or is a literal run — those consume input but
// contribute nothing to the result, exactly like MRI's nil_proc / extract_*
// returning nil under skip.
type handler func(s string) (any, bool)

// formatSpecifier is one parsed conversion (or literal run): the regex that
// matches its input prefix and the handler that converts the capture.
type formatSpecifier struct {
	spec    string
	re      *regexp.Regexp
	handler handler

	// countSpace is true when leading input whitespace must NOT be stripped
	// before matching — MRI's count_space? (true for %c-with-leading-space-in-
	// format, plain %c, and any %[...] set). Every other spec strips leading
	// whitespace first.
	countSpace bool

	// last holds the conversion produced by the most recent successful match,
	// so conversion() can hand it back without re-running the handler.
	last   any
	lastOK bool
}

// specMatch is the outcome of matching a spec against an input prefix: the text
// after the consumed prefix.
type specMatch struct {
	post string
}

// newFormatSpecifier builds the matcher and handler for a single format token,
// mirroring MRI's FormatSpecifier#initialize case-by-case.
func newFormatSpecifier(spec string) *formatSpecifier {
	fsp := &formatSpecifier{spec: spec}
	reStr, h := buildSpec(spec)
	fsp.handler = h
	fsp.re = regexp.MustCompile(`(?s)\A` + reStr)
	fsp.countSpace = wantsCountSpace(spec)
	return fsp
}

// star strips an optional leading run of whitespace and reports whether the spec
// (after that whitespace) begins with %*, which is how the absorbed-whitespace
// form " %*c" still counts as suppressed.
func star(spec string) bool {
	s := strings.TrimLeft(spec, " \t\n\r\f\v")
	return strings.HasPrefix(s, "%*")
}

// wantsCountSpace mirrors FormatSpecifier#count_space?: leading whitespace is
// preserved (not stripped) for a %c that follows whitespace in the format, a
// plain %c, or any %[...] character-class set.
func wantsCountSpace(spec string) bool {
	return countSpaceRe.MatchString(spec)
}

var countSpaceRe = regexp.MustCompile(`(?:\A|\S)%\*?\d*c|%\d*\[`)

var (
	reNamedClass  = regexp.MustCompile(`%\*?(\[\[:[a-z]+:\]\])`)
	reNamedClassW = regexp.MustCompile(`%\*?(\d+)(\[\[:[a-z]+:\]\])`)
	reSet         = regexp.MustCompile(`%\*?\[([^\]]*)\]`)
	reSetW        = regexp.MustCompile(`%\*?(\d+)\[([^\]]*)\]`)
	reI           = regexp.MustCompile(`%\*?i`)
	reIW          = regexp.MustCompile(`%\*?(\d+)i`)
	reDU          = regexp.MustCompile(`%\*?[du]`)
	reDUW         = regexp.MustCompile(`%\*?(\d+)[du]`)
	reX           = regexp.MustCompile(`%\*?[Xx]`)
	reXW          = regexp.MustCompile(`%\*?(\d+)[Xx]`)
	reO           = regexp.MustCompile(`%\*?o`)
	reOW          = regexp.MustCompile(`%\*?(\d+)o`)
	reF           = regexp.MustCompile(`%\*?[aefgAEFG]`)
	reFW          = regexp.MustCompile(`%\*?(\d+)[aefgAEFG]`)
	reSW          = regexp.MustCompile(`%\*?(\d+)s`)
	reS           = regexp.MustCompile(`%\*?s`)
	reCSpace      = regexp.MustCompile(`\s%\*?c`)
	reC           = regexp.MustCompile(`%\*?c`)
	reCW          = regexp.MustCompile(`%\*?(\d+)c`)
	rePercent     = regexp.MustCompile(`%%`)
)

// buildSpec returns the (regex-fragment, handler) pair for spec, following MRI's
// FormatSpecifier#initialize ordering exactly so the same token compiles to the
// same matcher.
func buildSpec(spec string) (string, handler) {
	sk := star(spec)
	h := hexClass

	switch {
	case reNamedClass.MatchString(spec):
		m := reNamedClass.FindStringSubmatch(spec)
		return "(" + namedClass(m[1]) + "+)", plainH(sk)

	case reNamedClassW.MatchString(spec):
		m := reNamedClassW.FindStringSubmatch(spec)
		return "(" + namedClass(m[2]) + "{1," + m[1] + "})", plainH(sk)

	case reSetW.MatchString(spec):
		m := reSetW.FindStringSubmatch(spec)
		yes := m[2]
		return "([" + yes + "]{1," + m[1] + "})", plainH(sk)

	case reSet.MatchString(spec):
		m := reSet.FindStringSubmatch(spec)
		yes := m[1]
		// MRI builds a complementary class for a lookahead; greedy [yes]+ is
		// already maximal so the anchored capture alone is equivalent.
		return "([" + yes + "]+)", plainH(sk)

	case reIW.MatchString(spec):
		m := reIW.FindStringSubmatch(spec)
		return buildIW(m[1], h), integerH(sk)

	case reI.MatchString(spec):
		return "([-+]?(?:(?:0[0-7]+)|(?:0[Xx]" + h + "+)|(?:[1-9]\\d*)))", integerH(sk)

	case reDUW.MatchString(spec):
		m := reDUW.FindStringSubmatch(spec)
		n := atoi(m[1])
		s := "("
		if n > 1 {
			s += "[-+]\\d{1," + itoa(n-1) + "}|"
		}
		s += "\\d{1," + m[1] + "})"
		return s, decimalH(sk)

	case reDU.MatchString(spec):
		return `([-+]?\d+)`, decimalH(sk)

	case reXW.MatchString(spec):
		m := reXW.FindStringSubmatch(spec)
		n := atoi(m[1])
		s := "("
		if n > 3 {
			s += "[-+]0[Xx]" + h + "{1," + itoa(n-3) + "}|"
		}
		if n > 2 {
			s += "0[Xx]" + h + "{1," + itoa(n-2) + "}|"
		}
		if n > 1 {
			s += "[-+]" + h + "{1," + itoa(n-1) + "}|"
		}
		s += h + "{1," + m[1] + "})"
		return s, hexH(sk)

	case reX.MatchString(spec):
		return "([-+]?(?:0[Xx])?" + h + "+)", hexH(sk)

	case reOW.MatchString(spec):
		m := reOW.FindStringSubmatch(spec)
		n := atoi(m[1])
		return "([-+][0-7]{1," + itoa(n-1) + "}|[0-7]{1," + m[1] + "})", octalH(sk)

	case reO.MatchString(spec):
		return `([-+]?[0-7]+)`, octalH(sk)

	case reFW.MatchString(spec), reF.MatchString(spec):
		// Float specs need lookahead RE2 lacks; (*formatSpecifier).match
		// intercepts them via matchFloatSpecial, so the compiled regex here is a
		// never-used placeholder. The handler still carries the conversion.
		return `(\z)`, floatH(sk)

	case reSW.MatchString(spec):
		m := reSW.FindStringSubmatch(spec)
		return `(\S{1,` + m[1] + `})`, plainH(sk)

	case reS.MatchString(spec):
		return `(\S+)`, plainH(sk)

	case reCSpace.MatchString(spec):
		return `\s*(.)`, plainH(sk)

	case reCW.MatchString(spec):
		m := reCW.FindStringSubmatch(spec)
		return `(.{1,` + m[1] + `})`, plainH(sk)

	case reC.MatchString(spec):
		return `(.)`, plainH(sk)

	case rePercent.MatchString(spec):
		return `(\s*%)`, nilH

	default:
		return "(" + regexp.QuoteMeta(spec) + ")", nilH
	}
}

// namedClass maps a POSIX-style [[:alpha:]] token to itself (Go's regexp accepts
// the same syntax), so a named character class compiles unchanged.
func namedClass(tok string) string {
	return tok
}

// buildIW expands the %<n>i width form to MRI's alternation of base-prefixed
// integers of bounded length.
func buildIW(width, h string) string {
	n := atoi(width)
	s := "("
	if n > 1 {
		s += "[1-9]\\d{1," + itoa(n-1) + "}|"
	}
	if n > 1 {
		s += "0[0-7]{1," + itoa(n-1) + "}|"
	}
	if n > 2 {
		s += "[-+]0[0-7]{1," + itoa(n-2) + "}|"
	}
	if n > 2 {
		s += "[-+][1-9]\\d{1," + itoa(n-2) + "}|"
	}
	if n > 2 {
		s += "0[Xx]" + h + "{1," + itoa(n-2) + "}|"
	}
	if n > 3 {
		s += "[-+]0[Xx]" + h + "{1," + itoa(n-3) + "}|"
	}
	s += "\\d)"
	return s
}

// match runs the spec against the input prefix, first stripping leading
// whitespace unless the spec opts out (countSpace), and records the conversion of
// the capture for conversion() to return. A nil result means the spec did not
// match (scanning will stop).
func (fsp *formatSpecifier) match(str string) *specMatch {
	fsp.last, fsp.lastOK = nil, false
	s := str
	if !fsp.countSpace {
		s = strings.TrimLeft(s, " \t\n\r\f\v")
	}

	// The unbounded-float and width-float forms need the negative-lookahead /
	// lookahead semantics RE2 lacks; handle them directly.
	if cap, post, ok := fsp.matchFloatSpecial(s); ok {
		fsp.last, fsp.lastOK = fsp.handler(cap)
		return &specMatch{post: post}
	}
	if fsp.isFloatSpecial() {
		// Float spec but no float present: no match.
		return nil
	}

	loc := fsp.re.FindStringSubmatchIndex(s)
	if loc == nil {
		return nil
	}
	cap := s[loc[2]:loc[3]]
	fsp.last, fsp.lastOK = fsp.handler(cap)
	return &specMatch{post: s[loc[1]:]}
}

// conversion returns the value the last successful match produced and whether the
// spec yields a value at all (false for suppressed / %% / literal specs).
func (fsp *formatSpecifier) conversion() (any, bool) {
	return fsp.last, fsp.lastOK
}

// --- handlers -------------------------------------------------------------

// plainH returns the capture verbatim (MRI extract_plain): a string, unless
// suppressed.
func plainH(skip bool) handler {
	return func(s string) (any, bool) {
		if skip {
			return nil, false
		}
		return s, true
	}
}

// decimalH parses a base-10 integer (extract_decimal: String#to_i), as int or
// *big.Int when it overflows, unless suppressed.
func decimalH(skip bool) handler {
	return func(s string) (any, bool) {
		if skip {
			return nil, false
		}
		return parseInt(s, 10), true
	}
}

// hexH parses a hexadecimal integer (extract_hex: String#hex), honouring an
// optional sign and 0x prefix, unless suppressed.
func hexH(skip bool) handler {
	return func(s string) (any, bool) {
		if skip {
			return nil, false
		}
		neg, body := splitSign(s)
		body = strings.TrimPrefix(body, "0x")
		body = strings.TrimPrefix(body, "0X")
		return parseIntSigned(neg, body, 16), true
	}
}

// octalH parses an octal integer (extract_octal: String#oct), unless suppressed.
func octalH(skip bool) handler {
	return func(s string) (any, bool) {
		if skip {
			return nil, false
		}
		neg, body := splitSign(s)
		return parseIntSigned(neg, body, 8), true
	}
}

// integerH parses %i with base detection (extract_integer: Kernel#Integer):
// 0x/0X = hex, leading 0 = octal, else decimal — unless suppressed.
func integerH(skip bool) handler {
	return func(s string) (any, bool) {
		if skip {
			return nil, false
		}
		neg, body := splitSign(s)
		switch {
		case strings.HasPrefix(body, "0x"), strings.HasPrefix(body, "0X"):
			return parseIntSigned(neg, body[2:], 16), true
		case len(body) > 1 && body[0] == '0':
			return parseIntSigned(neg, body[1:], 8), true
		default:
			return parseIntSigned(neg, body, 10), true
		}
	}
}

// floatH parses %a/%e/%f/%g (extract_float), handling the hexadecimal-float and
// "integer.[eE]exp" special forms MRI special-cases before falling back to
// String#to_f, unless suppressed.
func floatH(skip bool) handler {
	return func(s string) (any, bool) {
		if skip {
			return nil, false
		}
		return parseFloat(s), true
	}
}

// nilH is MRI's nil_proc: %% and literal runs consume input but emit no value.
func nilH(string) (any, bool) { return nil, false }

// --- numeric parsing ------------------------------------------------------

// splitSign peels an optional leading +/- off s, returning whether it was
// negative and the remaining body. s is a non-empty integer capture, so its first
// byte is a sign or a digit.
func splitSign(s string) (neg bool, body string) {
	switch s[0] {
	case '-':
		return true, s[1:]
	case '+':
		return false, s[1:]
	}
	return false, s
}

// parseInt converts a signed base-10 string to int, widening to *big.Int on
// overflow, matching Ruby's transparent Bignum promotion. The big path is reached
// only once ParseInt has reported overflow, so the result is genuinely wide.
func parseInt(s string, base int) any {
	if v, err := strconv.ParseInt(s, base, 64); err == nil {
		return int(v)
	}
	bi, _ := new(big.Int).SetString(s, base)
	return bi
}

// parseIntSigned converts an unsigned-magnitude body in the given base, applying
// neg, to int or *big.Int. The body always matched the spec's regex, so it is a
// non-empty run of valid digits for base; the big path is overflow-only.
func parseIntSigned(neg bool, body string, base int) any {
	if v, err := strconv.ParseInt(body, base, 64); err == nil {
		if neg {
			v = -v
		}
		return int(v)
	}
	bi, _ := new(big.Int).SetString(body, base)
	if neg {
		bi.Neg(bi)
	}
	return bi
}

// parseFloat implements MRI extract_float: a hex float (0x...p...), the
// "<int>.[eE]<exp>" oddity, then String#to_f.
func parseFloat(s string) float64 {
	if m := hexFloatRe.FindStringSubmatch(s); m != nil {
		sign := m[1]
		frac := m[2]
		exp := m[3]
		parts := strings.SplitN(frac, ".", 2)
		f := hexToFloat(parts[0])
		if len(parts) == 2 && len(parts[1]) > 0 {
			f += hexToFloat(parts[1]) / math.Pow(16, float64(len(parts[1])))
		}
		e := atoi(exp)
		r := math.Ldexp(f, e)
		if sign == "-" {
			r = -r
		}
		return r
	}
	if m := intDotExpRe.FindStringSubmatch(s); m != nil {
		return rubyToF(m[1] + m[2])
	}
	return rubyToF(s)
}

var (
	hexFloatRe  = regexp.MustCompile(`\A([-+]?)0[xX](\.[A-Fa-f0-9]+|[A-Fa-f0-9]+(?:\.[A-Fa-f0-9]*)?)[pP]([-+]?\d+)`)
	intDotExpRe = regexp.MustCompile(`\A([-+]?\d+)\.([eE][-+]\d+)`)
)

// hexToFloat parses a hex-digit magnitude string to float64 (Ruby String#hex on
// a part of a hex float). An empty string — the integer part of e.g. 0x.8p0 — is
// zero.
func hexToFloat(s string) float64 {
	if s == "" {
		return 0
	}
	bi, _ := new(big.Int).SetString(s, 16)
	r, _ := new(big.Float).SetInt(bi).Float64()
	return r
}

// rubyToF emulates String#to_f for the float tokens our grammar produces: a
// trailing bare dot is dropped (Ruby reads "3." as 3.0), an all-dot/sign token is
// zero, and an out-of-range exponent saturates to ±Inf as String#to_f does. The
// tokens always match MRI's float grammar, so strconv's only non-nil error is the
// range error, which still returns the saturated value.
func rubyToF(s string) float64 {
	s = strings.TrimRight(s, ".")
	if s == "" || s == "+" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// matchFloatSpecial handles the float specs whose MRI regex uses lookahead RE2
// cannot express. It returns the captured float text, the post-match remainder,
// and whether this spec is a float spec that matched here.
func (fsp *formatSpecifier) matchFloatSpecial(s string) (capture, post string, ok bool) {
	if reFW.MatchString(fsp.spec) {
		m := reFW.FindStringSubmatch(fsp.spec)
		width := atoi(m[1])
		ft := leadingFloatToken(s)
		if ft == "" {
			return "", "", false
		}
		if len(ft) > width {
			ft = nonSpacePrefix(s, width)
		}
		return ft, s[len(ft):], true
	}
	if reF.MatchString(fsp.spec) {
		ft := leadingFloatToken(s)
		if ft == "" {
			return "", "", false
		}
		return ft, s[len(ft):], true
	}
	return "", "", false
}

// isFloatSpecial reports whether the spec is a float spec (so a nil
// matchFloatSpecial result means "no float here", not "try the regex").
func (fsp *formatSpecifier) isFloatSpecial() bool {
	return reF.MatchString(fsp.spec) || reFW.MatchString(fsp.spec)
}

// leadingFloatToken returns the longest prefix of s that MRI's float grammar
// accepts: a hex float, an integer not followed by a digit or dot, or
// digits.digits with an optional exponent. It returns "" when no float begins s.
func leadingFloatToken(s string) string {
	if m := floatTokenRe.FindString(s); m != "" {
		return m
	}
	return ""
}

// floatTokenRe matches a leading float token. The integer branch \d+(?![\d.]) is
// expressed as \d+ with a guard applied in code (floatTokenRe deliberately keeps
// the dotted/exponent branch greedy so it wins when a dot or exponent follows).
var floatTokenRe = regexp.MustCompile(
	`\A[-+]?(?:` +
		`0[xX](?:\.[A-Fa-f0-9]+|[A-Fa-f0-9]+(?:\.[A-Fa-f0-9]*)?)[pP][-+]?\d+` +
		`|\d*\.\d*(?:[eE][-+]?\d+)?` +
		`|\d+` +
		`)`,
)

// nonSpacePrefix returns up to n leading non-space characters of s (the %<n>f
// capture once the guard has confirmed a float is present).
func nonSpacePrefix(s string, n int) string {
	end := 0
	for end < len(s) && end < n && !isSpace(s[end]) {
		end++
	}
	return s[:end]
}

// --- small helpers --------------------------------------------------------

func atoi(s string) int { v, _ := strconv.Atoi(s); return v }
func itoa(n int) string { return strconv.Itoa(n) }
