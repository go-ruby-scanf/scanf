// Copyright (c) the go-ruby-scanf/scanf authors
//
// SPDX-License-Identifier: BSD-3-Clause

package scanf

import (
	"math"
	"math/big"
	"reflect"
	"testing"
)

// bigStr builds a *big.Int from a base-10 literal for the golden table.
func bigStr(s string) *big.Int {
	bi, _ := new(big.Int).SetString(s, 10)
	return bi
}

// TestScan is the deterministic, ruby-free golden table: every directive, width,
// set, suppression, literal, partial and failed match, with the exact values MRI
// 4.0.5's scanf gem produces (captured by the differential harness). This table
// alone drives the package to 100% coverage so the no-ruby CI lanes pass the gate.
func TestScan(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		format string
		want   []any
	}{
		// %d / %u — signed decimal.
		{"d_plain", "42", "%d", []any{42}},
		{"d_neg", "-42", "%d", []any{-42}},
		{"d_pos", "+42", "%d", []any{42}},
		{"d_lead_ws", "  42", "%d", []any{42}},
		{"d_leading_zero", "042", "%d", []any{42}},
		{"u_plain", "42", "%u", []any{42}},
		{"u_neg", "-42", "%u", []any{-42}},
		{"d_bignum", "123456789012345678901234567890", "%d",
			[]any{bigStr("123456789012345678901234567890")}},
		{"d_neg_bignum", "-123456789012345678901234567890", "%d",
			[]any{bigStr("-123456789012345678901234567890")}},

		// %i — base detection.
		{"i_hex", "0x1f", "%i", []any{31}},
		{"i_octal", "010", "%i", []any{8}},
		{"i_decimal", "10", "%i", []any{10}},
		{"i_bare_zero", "0", "%i", nil},
		{"i_double_zero", "00", "%i", []any{0}},
		{"i_eight_after_zero", "08", "%i", nil},
		{"d_on_hex", "0x1f", "%d", []any{0}},

		// %x / %X — hexadecimal.
		{"x_plain", "ff", "%x", []any{255}},
		{"x_prefix", "0xff", "%x", []any{255}},
		{"x_neg", "-ff", "%x", []any{-255}},
		{"x_mixedcase", "deadBEEF", "%x", []any{3735928559}},
		{"X_upper", "FF", "%X", []any{255}},
		{"x_pos", "+ff", "%x", []any{255}},
		{"x_pos_prefix", "+0xff", "%x", []any{255}},

		// %o — octal.
		{"o_plain", "17", "%o", []any{15}},
		{"o_neg", "-17", "%o", []any{-15}},
		{"o_pos", "+17", "%o", []any{15}},
		{"i_pos_hex", "+0x1f", "%i", []any{31}},
		{"i_pos_octal", "+010", "%i", []any{8}},

		// %a/%e/%f/%g — floats.
		{"f_plain", "3.14", "%f", []any{3.14}},
		{"e_int_only", "1e3", "%e", []any{1.0}},
		{"e_with_frac", "1.0e3", "%e", []any{1000.0}},
		{"g_neg_exp", "1.5e-2", "%g", []any{0.015}},
		{"f_lead_dot", ".5", "%f", []any{0.5}},
		{"f_trail_dot", "3.", "%f", []any{3.0}},
		{"e_simple", "12.5", "%e", []any{12.5}},
		{"e_cap_E", "1E3", "%e", []any{1.0}},
		{"f_int_value", "42", "%f", []any{42.0}},
		{"a_hexfloat", "0x1.8p1", "%a", []any{3.0}},
		{"e_pos_exp", "1.5e+2", "%e", []any{150.0}},
		{"f_int_dot_exp_signed", "12.e+3", "%f", []any{12000.0}},
		{"f_int_dot_exp_unsigned", "12.e3", "%f", []any{12000.0}},
		{"a_neg_hexfloat", "-0x1p2", "%a", []any{-4.0}},
		{"a_frac_only", "0x.8p0", "%a", []any{0.5}},
		{"f_lone_dot", ".", "%f", []any{0.0}},
		{"f_no_float", "abc", "%f", nil},
		{"f_width_no_float", "abc", "%5f", nil},
		{"f_overflow_inf", "1.0e999", "%f", []any{math.Inf(1)}},
		{"f_overflow_neg_inf", "-1.0e999", "%f", []any{math.Inf(-1)}},

		// %s — whitespace-delimited string.
		{"s_one", "hello world", "%s", []any{"hello"}},
		{"s_two", "hello world", "%s %s", []any{"hello", "world"}},
		{"s_tab_stop", "a\tb", "%s", []any{"a"}},
		{"s_skip_lead_ws", "\t\tx", "%s", []any{"x"}},

		// %c — single char (and width).
		{"c_one", "abc", "%c", []any{"a"}},
		{"c_width", "abc", "%3c", []any{"abc"}},
		{"c_width_clip", "abcdef", "%3c", []any{"abc"}},
		{"c_width_short", "a", "%2c", []any{"a"}},
		{"c_empty", "", "%c", nil},
		{"c_keeps_space", "   x", "%c", []any{" "}},
		{"c_space_in_fmt", "   x", " %c", []any{"x"}},
		{"c_width_keeps_space", "  xy", "%4c", []any{"  xy"}},
		{"c_second_space", "a  b", "%c%c", []any{"a", " "}},
		{"c_skip_between", "a b", "%c %c", []any{"a", "b"}},

		// width forms on numerics / strings.
		{"d_width", "12345", "%2d", []any{12}},
		{"d_width_then_s", "123abc", "%3d%s", []any{123, "abc"}},
		{"s_width", "hello", "%3s", []any{"hel"}},
		{"d_width_big", "1234567", "%5d", []any{12345}},
		{"i_width", "0x1F", "%5i", []any{31}},
		{"x_width", "abcdef", "%5x", []any{703710}},
		{"o_width", "777", "%2o", []any{63}},
		{"f_width", "3.14159", "%4f", []any{3.14}},

		// %[...] / %[^...] — sets.
		{"set_range", "abc123", "%[a-c]", []any{"abc"}},
		{"set_neg", "abc123", "%[^0-9]", []any{"abc"}},
		{"set_list", "aaabbb", "%[ab]", []any{"aaabbb"}},
		{"set_letters", "hello", "%[helo]", []any{"hello"}},
		{"set_width", "123456", "%3[0-9]", []any{"123"}},
		{"set_dash_last", "a-b", "%[a-]", []any{"a-"}},
		{"set_no_skip_ws", "  ab", "%[a-z]", nil},
		{"set_then_s", "abc def", "%[^ ] %s", []any{"abc", "def"}},
		{"set_neg_x", "abcxd", "%[^x]", []any{"abc"}},

		// %% literal percent.
		{"percent", "50%", "%d%%", []any{50}},

		// assignment suppression.
		{"suppress_d", "42 99", "%*d %d", []any{99}},
		{"suppress_s", "a b c", "%*s %s", []any{"b"}},
		{"suppress_mid", "1 2 3", "%d %*d %d", []any{1, 3}},
		{"suppress_c", "ab", "%*c%c", []any{"b"}},
		{"suppress_x", "ff 1", "%*x %d", []any{1}},
		{"suppress_o", "17 1", "%*o %d", []any{1}},
		{"suppress_i", "0x1f 1", "%*i %d", []any{1}},
		{"suppress_f", "3.14 1", "%*f %d", []any{1}},
		{"suppress_set", "abc 1", "%*[a-c] %d", []any{1}},
		{"suppress_star_c", "x9", " %*c%d", []any{9}},

		// literals + whitespace in format.
		{"literal_prefix", "x=42", "x=%d", []any{42}},
		{"ws_between", "  42  99", "%d %d", []any{42, 99}},
		{"char_comma", "a,b", "%c,%c", []any{"a", "b"}},
		{"set_colon", "foo:bar", "%[^:]:%s", []any{"foo", "bar"}},
		{"literal_match", "abc", "abc", nil},
		{"literal_mismatch", "abc", "abx", nil},
		{"literal_suffix_ok", "42x", "%dx", []any{42}},
		{"literal_suffix_fail", "42y", "%dx", []any{42}},
		{"trailing_ws_fmt", "42 ", "%d ", []any{42}},

		// partial / failed matches.
		{"fail_first", "abc", "%d", nil},
		{"partial_two", "42 abc", "%d %d", []any{42}},
		{"empty_input", "", "%d", nil},
		{"literal_first_fail", "nomatch", "x%d", nil},
		{"more_specs_than_input", "12 34 56", "%d %d %d %d", []any{12, 34, 56}},
		{"alternating", "a1b2", "%c%d%c%d", []any{"a", 1, "b", 2}},
		{"d_then_s_fail", "xx", "%d%s", nil},
		{"plus_space", "+ 42", "%d", nil},

		// empty / blank format.
		{"empty_format", "abc", "", nil},
		{"blank_format", "abc", "   ", nil},

		// %b is not an MRI scanf directive — treated as literal "%b".
		{"b_is_literal", "1010", "%b", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Scan(c.input, c.format)
			if err != nil {
				t.Fatalf("Scan(%q,%q) error: %v", c.input, c.format, err)
			}
			if !equalValues(got, c.want) {
				t.Errorf("Scan(%q,%q) = %#v, want %#v", c.input, c.format, got, c.want)
			}
		})
	}
}

// TestScanAll is the deterministic golden table for the repeated, block-style
// scan (MRI String#scanf with a block), one group per pass until exhausted.
func TestScanAll(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		format string
		want   [][]any
	}{
		{"pairs", "1 2 3 4", "%d %d", [][]any{{1, 2}, {3, 4}}},
		{"singles", "1 2 3", "%d", [][]any{{1}, {2}, {3}}},
		{"mixed", "1 a 2 b", "%d %s", [][]any{{1, "a"}, {2, "b"}}},
		{"doubled_block", "10 20 30", "%d", [][]any{{10}, {20}, {30}}},
		{"empty", "", "%d", nil},
		{"words", "a b c", "%s", [][]any{{"a"}, {"b"}, {"c"}}},
		{"trailing_partial", "1 2 3", "%d %d", [][]any{{1, 2}, {3}}},
		{"stops_on_fail", "1 x", "%d", [][]any{{1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ScanAll(c.input, c.format)
			if err != nil {
				t.Fatalf("ScanAll(%q,%q) error: %v", c.input, c.format, err)
			}
			if !equalGroups(got, c.want) {
				t.Errorf("ScanAll(%q,%q) = %#v, want %#v", c.input, c.format, got, c.want)
			}
		})
	}
}

// TestNamedClass covers the POSIX named-character-class set forms, which the main
// table does not exercise (MRI accepts %[[:alpha:]] and the width form).
func TestNamedClass(t *testing.T) {
	cases := []struct {
		input, format string
		want          []any
	}{
		{"abc123", "%[[:alpha:]]", []any{"abc"}},
		{"abcdef", "%3[[:alpha:]]", []any{"abc"}},
		{"123abc", "%[[:digit:]]", []any{"123"}},
	}
	for _, c := range cases {
		got, _ := Scan(c.input, c.format)
		if !equalValues(got, c.want) {
			t.Errorf("Scan(%q,%q) = %#v, want %#v", c.input, c.format, got, c.want)
		}
	}
}

// TestBignumNarrowing checks the boundary where a hex/octal magnitude crosses
// int64, so the *big.Int path and its narrow-back-to-int branch both run.
func TestBignumNarrowing(t *testing.T) {
	// 18 f's = 0xFFFFFFFFFFFFFFFFFF > int64, forcing the big path on %x.
	got, _ := Scan("ffffffffffffffffff", "%x")
	want := func() *big.Int { bi, _ := new(big.Int).SetString("ffffffffffffffffff", 16); return bi }()
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}
	bi, ok := got[0].(*big.Int)
	if !ok || bi.Cmp(want) != 0 {
		t.Errorf("hex bignum = %#v, want %v", got[0], want)
	}

	// A large octal magnitude likewise overflows int64.
	got, _ = Scan("7777777777777777777777", "%o")
	if _, ok := got[0].(*big.Int); !ok {
		t.Errorf("octal bignum = %#v, want *big.Int", got[0])
	}

	// A negative hex bignum exercises parseIntSigned's bi.Neg branch.
	got, _ = Scan("-ffffffffffffffffff", "%x")
	wantNeg := func() *big.Int { bi, _ := new(big.Int).SetString("-ffffffffffffffffff", 16); return bi }()
	bi, ok = got[0].(*big.Int)
	if !ok || bi.Cmp(wantNeg) != 0 {
		t.Errorf("neg hex bignum = %#v, want %v", got[0], wantNeg)
	}
}

// equalValues compares two []any, treating *big.Int by value so the table can use
// concrete bignums.
func equalValues(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalOne(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalOne(x, y any) bool {
	if bx, ok := x.(*big.Int); ok {
		by, ok := y.(*big.Int)
		return ok && bx.Cmp(by) == 0
	}
	return reflect.DeepEqual(x, y)
}

func equalGroups(a, b [][]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalValues(a[i], b[i]) {
			return false
		}
	}
	return true
}
