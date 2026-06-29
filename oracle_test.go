// Copyright (c) the go-ruby-scanf/scanf authors
//
// SPDX-License-Identifier: BSD-3-Clause

package scanf

import (
	"bufio"
	"fmt"
	"math"
	"math/big"
	"os/exec"
	"strings"
	"testing"
)

// rubyBin locates a usable `ruby` once and confirms it is MRI 4.0+ with the scanf
// gem loadable. The oracle tests skip themselves otherwise (the qemu cross-arch
// lanes, the Windows lane, and any host without ruby or the gem), so the
// deterministic golden suite alone drives the 100% gate there.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping MRI oracle")
	}
	// Gate on MRI >= 4.0 and a loadable scanf gem so we only ever diff against a
	// reference matching our target (MRI 4.0.5's scanf 1.0.0).
	probe := `$stdout.binmode
if RUBY_VERSION < "4.0"
  print "SKIP version"
else
  begin
    require "scanf"
    print "OK"
  rescue LoadError
    print "SKIP gem"
  end
end`
	out, err := exec.Command(path, "-e", probe).CombinedOutput()
	if err != nil || !strings.HasPrefix(string(out), "OK") {
		t.Skipf("ruby unusable for scanf oracle (%s); skipping", strings.TrimSpace(string(out)))
	}
	return path
}

// oracleScript is the Ruby driver. It reads NUL-separated mode\0input\0format
// records on stdin and prints, one per line, the inspected scanf result so Go can
// compare it byte-for-byte. stdin and stdout are put in binary mode so Windows
// text translation never alters the bytes (the go-ruby-erb / go-ruby-yaml lesson).
const oracleScript = `
$stdout.binmode
$stdin.binmode
require "scanf"
def insp(a); "[" + a.map { |v| v.inspect }.join(", ") + "]"; end
$stdin.each_line("\n") do |line|
  line = line.chomp("\n")
  next if line.empty?
  mode, input, fmt = line.split("\x00", 3)
  if mode == "all"
    groups = input.scanf(fmt) { |m| m }
    puts "[" + groups.map { |g| insp(g) }.join(", ") + "]"
  else
    puts insp(input.scanf(fmt))
  end
end
`

// runOracle feeds the records to the Ruby driver and returns its output lines.
func runOracle(t *testing.T, bin string, records []string) []string {
	t.Helper()
	cmd := exec.Command(bin, "-e", oracleScript)
	cmd.Stdin = strings.NewReader(strings.Join(records, "\n") + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby oracle error: %v\noutput:\n%s", err, out)
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// inspectValue renders a Go scanf value the way Ruby's #inspect renders the
// matching Integer / Float / String, so the oracle comparison is a string compare.
func inspectValue(v any) string {
	switch x := v.(type) {
	case string:
		return rubyInspectString(x)
	case int:
		return fmt.Sprintf("%d", x)
	case *big.Int:
		return x.String()
	case float64:
		return rubyInspectFloat(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// rubyInspectString renders a Go string as Ruby String#inspect does for the ASCII
// content scanf produces: wrap in double quotes and escape the handful of control
// characters and metacharacters the corpus can yield.
func rubyInspectString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// rubyInspectFloat renders a float64 as Ruby Float#inspect: Infinity / -Infinity /
// NaN spelled out, an integral value with a trailing ".0", and everything else via
// Go's shortest round-trip form (which agrees with Ruby for the corpus values).
func rubyInspectFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case math.IsNaN(f):
		return "NaN"
	}
	s := fmt.Sprintf("%g", f)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// inspectScan renders Scan's result as a Ruby array #inspect.
func inspectScan(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = inspectValue(v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// inspectScanAll renders ScanAll's result as a Ruby array-of-arrays #inspect.
func inspectScanAll(groups [][]any) string {
	parts := make([]string, len(groups))
	for i, g := range groups {
		parts[i] = inspectScan(g)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// oracleCorpus is the differential corpus: every directive, width, set,
// suppression, literal, and partial/failed match, in both the single (Scan) and
// block (ScanAll) forms. Each entry is checked against the live MRI scanf gem.
var oracleCorpus = []struct {
	mode, input, format string
}{
	// %d / %u
	{"one", "42", "%d"}, {"one", "-42", "%d"}, {"one", "+42", "%d"},
	{"one", "  42", "%d"}, {"one", "042", "%d"}, {"one", "42", "%u"},
	{"one", "-42", "%u"}, {"one", "123456789012345678901234567890", "%d"},
	{"one", "-123456789012345678901234567890", "%d"},
	{"one", "255", "%5d"}, {"one", "1234567", "%5d"},
	// %i
	{"one", "0x1f", "%i"}, {"one", "010", "%i"}, {"one", "10", "%i"},
	{"one", "0", "%i"}, {"one", "00", "%i"}, {"one", "08", "%i"},
	{"one", "0x1f", "%d"}, {"one", "0x1F", "%5i"},
	{"one", "+0x1f", "%i"}, {"one", "+010", "%i"},
	// %x / %X / %o
	{"one", "ff", "%x"}, {"one", "0xff", "%x"}, {"one", "-ff", "%x"},
	{"one", "deadBEEF", "%x"}, {"one", "FF", "%X"}, {"one", "abcdef", "%5x"},
	{"one", "+ff", "%x"}, {"one", "+0xff", "%x"},
	{"one", "17", "%o"}, {"one", "-17", "%o"}, {"one", "+17", "%o"},
	{"one", "777", "%2o"},
	{"one", "ffffffffffffffffff", "%x"}, {"one", "-ffffffffffffffffff", "%x"},
	// %a/%e/%f/%g
	{"one", "3.14", "%f"}, {"one", "1e3", "%e"}, {"one", "1.0e3", "%e"},
	{"one", "1.5e-2", "%g"}, {"one", ".5", "%f"}, {"one", "3.", "%f"},
	{"one", "12.5", "%e"}, {"one", "1E3", "%e"}, {"one", "42", "%f"},
	{"one", "0x1.8p1", "%a"}, {"one", "1.5e+2", "%e"}, {"one", "12.e+3", "%f"},
	{"one", "12.e3", "%f"}, {"one", "-0x1p2", "%a"}, {"one", "0x.8p0", "%a"},
	{"one", ".", "%f"}, {"one", "abc", "%f"}, {"one", "abc", "%5f"},
	{"one", "1.0e999", "%f"}, {"one", "-1.0e999", "%f"}, {"one", "3.14159", "%4f"},
	// %s
	{"one", "hello world", "%s"}, {"one", "hello world", "%s %s"},
	{"one", "a\tb", "%s"}, {"one", "\t\tx", "%s"}, {"one", "hello", "%3s"},
	// %c
	{"one", "abc", "%c"}, {"one", "abc", "%3c"}, {"one", "abcdef", "%3c"},
	{"one", "a", "%2c"}, {"one", "", "%c"}, {"one", "   x", "%c"},
	{"one", "   x", " %c"}, {"one", "  xy", "%4c"}, {"one", "a  b", "%c%c"},
	{"one", "a b", "%c %c"},
	// width then s / d
	{"one", "12345", "%2d"}, {"one", "123abc", "%3d%s"},
	// sets
	{"one", "abc123", "%[a-c]"}, {"one", "abc123", "%[^0-9]"},
	{"one", "aaabbb", "%[ab]"}, {"one", "hello", "%[helo]"},
	{"one", "123456", "%3[0-9]"}, {"one", "a-b", "%[a-]"},
	{"one", "  ab", "%[a-z]"}, {"one", "abc def", "%[^ ] %s"},
	{"one", "abcxd", "%[^x]"}, {"one", "abc123", "%[[:alpha:]]"},
	{"one", "abcdef", "%3[[:alpha:]]"}, {"one", "123abc", "%[[:digit:]]"},
	// %%
	{"one", "50%", "%d%%"},
	// suppression
	{"one", "42 99", "%*d %d"}, {"one", "a b c", "%*s %s"},
	{"one", "1 2 3", "%d %*d %d"}, {"one", "ab", "%*c%c"},
	{"one", "ff 1", "%*x %d"}, {"one", "17 1", "%*o %d"},
	{"one", "0x1f 1", "%*i %d"}, {"one", "3.14 1", "%*f %d"},
	{"one", "abc 1", "%*[a-c] %d"}, {"one", "x9", " %*c%d"},
	// literals + whitespace
	{"one", "x=42", "x=%d"}, {"one", "  42  99", "%d %d"},
	{"one", "a,b", "%c,%c"}, {"one", "foo:bar", "%[^:]:%s"},
	{"one", "abc", "abc"}, {"one", "abc", "abx"},
	{"one", "42x", "%dx"}, {"one", "42y", "%dx"}, {"one", "42 ", "%d "},
	// partial / failed
	{"one", "abc", "%d"}, {"one", "42 abc", "%d %d"}, {"one", "", "%d"},
	{"one", "nomatch", "x%d"}, {"one", "12 34 56", "%d %d %d %d"},
	{"one", "a1b2", "%c%d%c%d"}, {"one", "xx", "%d%s"}, {"one", "+ 42", "%d"},
	// ScanAll / block
	{"all", "1 2 3 4", "%d %d"}, {"all", "1 2 3", "%d"},
	{"all", "1 a 2 b", "%d %s"}, {"all", "10 20 30", "%d"},
	{"all", "a b c", "%s"}, {"all", "1 2 3", "%d %d"},
}

// TestOracle diffs every corpus entry against the live MRI scanf gem: Scan vs
// String#scanf and ScanAll vs String#scanf-with-block, comparing Ruby's #inspect
// of each result to ours byte-for-byte.
func TestOracle(t *testing.T) {
	bin := rubyBin(t)

	records := make([]string, len(oracleCorpus))
	for i, c := range oracleCorpus {
		records[i] = c.mode + "\x00" + c.input + "\x00" + c.format
	}
	want := runOracle(t, bin, records)
	if len(want) != len(oracleCorpus) {
		t.Fatalf("oracle returned %d lines, want %d", len(want), len(oracleCorpus))
	}

	for i, c := range oracleCorpus {
		var got string
		if c.mode == "all" {
			g, _ := ScanAll(c.input, c.format)
			got = inspectScanAll(g)
		} else {
			g, _ := Scan(c.input, c.format)
			got = inspectScan(g)
		}
		if got != want[i] {
			t.Errorf("%s scanf(%q, %q):\n  go   = %s\n  ruby = %s",
				c.mode, c.input, c.format, got, want[i])
		}
	}
}
