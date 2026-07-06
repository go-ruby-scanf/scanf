<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-scanf/brand/main/social/go-ruby-scanf-scanf.png" alt="go-ruby-scanf/scanf" width="720"></p>

# scanf — go-ruby-scanf

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-scanf.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's
[`scanf`](https://docs.ruby-lang.org/en/master/Scanf.html) stdlib** — the
deterministic, interpreter-independent core of MRI 4.0.5's `String#scanf` /
`IO#scanf`. It parses values out of an input string according to a scanf format
string and returns the converted values, so it is the **inverse of `sprintf`**:
where `sprintf("%d", 42)` renders `"42"`, `scanf("42", "%d")` reads `42` back.

It is the scanf backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-format](https://github.com/go-ruby-format/format) (the `sprintf`
side), [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo
engine) and [go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (Psych).

> **What it is — and isn't.** Scanning an input string against a scanf format
> grammar (the directives, widths, character-class sets, suppression flag, and
> literal/whitespace matching) is fully deterministic and needs **no interpreter**,
> so it lives here as pure Go. It hands back a small, explicit value model
> (`int` / `*big.Int` / `float64` / `string`, the natural mapping of Ruby's
> `Integer` / `Float` / `String`) the host binds to its own objects.

## Features

A faithful port of MRI's `scanf` gem (v1.0.0), validated against the `ruby`
binary on every supported platform:

- **Integers** — `%d` / `%u` signed decimal, `%i` with base detection (`0x`/`0X`
  = hex, leading `0` = octal, else decimal), `%x` / `%X` hexadecimal (optional
  `0x` prefix and sign), and `%o` octal. Values widen transparently to
  arbitrary-precision (`*big.Int`) the moment they overflow `int`, just like
  Ruby's `Integer`.
- **Floats** — `%a` / `%e` / `%f` / `%g` (and their uppercase forms), including
  hexadecimal floats (`0x1.8p1`), MRI's `123.e+3` form, and `±Infinity`
  saturation on overflow — matching MRI's quirky float grammar byte-for-byte
  (e.g. `"1e3"` under `%e` reads `1.0`, because the exponent is only consumed
  after a fraction).
- **Strings & characters** — `%s` (a run of non-whitespace), `%c` (a single
  character, or *n* with a width — and unlike most directives, `%c` does **not**
  skip leading whitespace).
- **Character-class sets** — `%[…]` and the negated `%[^…]`, with ranges
  (`%[a-z]`), the POSIX named classes (`%[[:alpha:]]`), and width bounds
  (`%3[0-9]`).
- **Widths** — a decimal field width on any directive (`%3d`, `%5s`, `%4f`, …).
- **Assignment suppression** — the `*` flag (`%*d`) consumes input but emits no
  value.
- **Literal & whitespace matching** — `%%` matches a literal `%`; whitespace in
  the format skips optional input whitespace; every other format character must
  match the input exactly.
- **MRI stopping semantics** — scanning stops at the first non-match, at end of
  input, or when the format is exhausted, returning the values converted so far
  (a partial match yields a shorter slice; no match yields an empty one).

`%b` is intentionally **absent** — MRI's `scanf` gem has no binary directive, so
to stay byte-for-byte faithful a `%b` in a format is treated as the literal
characters `%b`.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x).

## Install

```sh
go get github.com/go-ruby-scanf/scanf
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-scanf/scanf"
)

func main() {
	// Scan runs the format once — MRI's String#scanf without a block.
	vs, _ := scanf.Scan("abc 123", "%s %d")
	fmt.Println(vs) // [abc 123]   ("abc" string, 123 int)

	vs, _ = scanf.Scan("50%", "%d%%")
	fmt.Println(vs) // [50]

	vs, _ = scanf.Scan("42 abc", "%d %d")
	fmt.Println(vs) // [42]        (partial: the second %d found no integer)

	// ScanAll repeats the format until the input is exhausted — MRI's block form.
	all, _ := scanf.ScanAll("1 2 3 4", "%d %d")
	fmt.Println(all) // [[1 2] [3 4]]
}
```

## Value model

Each converted value is one of a small, fixed set of Go types, so a host can map
its own objects to and from this package:

| Ruby      | Go                     |
| --------- | ---------------------- |
| `Integer` | `int`, or `*big.Int` when it overflows `int` |
| `Float`   | `float64`              |
| `String`  | `string`               |

A suppressed directive (`%*…`) and `%%` contribute no value; a directive that
fails to match ends the scan, so the result holds only what was converted before
that point.

## API

```go
// Scan parses values out of input per format, once (String#scanf without a
// block). A directive that fails ends the scan; the result holds the values
// converted so far. The error is always nil (a non-match is a shorter slice,
// not a failure) — it is present for an idiomatic Go shape.
func Scan(input, format string) ([]any, error)

// ScanAll repeatedly applies format to input (String#scanf with a block /
// block_scanf), returning one group per successful pass until the input is
// exhausted or a pass converts nothing.
func ScanAll(input, format string) ([][]any, error)
```

## Tests & coverage

The suite pairs a deterministic, ruby-free golden table (which alone holds
coverage at 100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential MRI oracle**: a wide corpus — every directive, width, set,
suppression flag, literal, and partial/failed match, in both the single and
block forms — is scanned here and by the system `ruby` (`String#scanf`), and the
two `#inspect` renderings are compared byte-for-byte. The oracle script
`$stdout.binmode`s (and binmodes stdin) so Windows text-mode never pollutes the
bytes, gates on `RUBY_VERSION >= "4.0"` with the `scanf` gem present, and skips
itself everywhere `ruby` is absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-scanf/scanf authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
