package yangregexp

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dlclark/regexp2/v2"
	"github.com/dlclark/regexp2/v2/syntax"
)

var corpus = []string{
	``, `a`, `^a`, `a$`, `^a$`, `^$`, `^`, `$`, `a|bc`, `^a|bc$`, `^a$|^bc$`, `^(ab|cd)+$`,
	`^(a|)$`, `^(ab)?c$`, `^(ab){2,4}$`, `^a{2,}$`, `^a{0}$`, `^a{2}$`, `^a+?$`, `^a*?b$`,
	`^.*$`, `^[\s\S]*$`, `^[^]*$`, `^[]$`, `(?!)`, `[^a]+`, `^[^\n]*$`, `^\d+$`, `^\D+$`,
	`^\s+$`, `^\S+$`, `^\w+$`, `^\W+$`, `^[\d_]+$`, `^[\s\D]+$`, `^[^\w\s]+$`,
	`^\x41\u0042$`, `^\cI$`, `^\0$`, `^[\0-\x20]$`, `^\uD800$`, `^\uFFFF$`,
	`^[a-zA-Z0-9_./-]+$`, `^[\[\]\\^$-]+$`, `^\^\$\.\*\+\?\(\)\[\]\{\}\|\\$`,
	`^[é中😀]+$`, `^😀{2}$`, `^[^é中😀]+$`, `^[!@#%&=,:;'"<>/~` + "`" + `]+$`,
	`^(?!none$).*$`, `^(?!disabled$).*$`, `^(?!a|bb)[a-z]+$`, `^(?!.*ab).*$`,
	`^(?=ab)[a-z]+$`, `^(?=ab$).*$`, `^(?=a)(?!ab$)[a-z]+$`, `^(?=a(?=b)).*$`,
	`^(?=(?!ab)[a-z]+$).*$`, `^((?!a$)[a-z]*)$`, `^a[^a]*b$`, `^(a*)*$`,
	`^(?>a|b)c$`, `(?>ab|a)`, `^(?>a*)b$`, `^(?>a|ab)$`,
	// Capture resets inside repetitions must remain zero-width during both
	// conversion and the atomic continuation/fixed-length safety checks.
	`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`, `^(?>a*)(bc)+$`, `^(?>(ab){2})c$`,
}

func allPatterns(t testing.TB) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/nvue-patterns.txt")
	if err != nil {
		t.Fatal(err)
	}
	return append(append([]string{}, corpus...), strings.Split(strings.TrimSpace(string(data)), "\n")...)
}

// The emitted subset (explicit classes, groups, alternation and repetition)
// has identical semantics in Go and XSD. Go supplies the fast test oracle for
// exhaustive/fuzz checks; the independent pyang test validates actual XSD/YANG.
func targetMatcher(t testing.TB, patterns []Pattern) func(string) bool {
	t.Helper()
	expressions := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		re, err := regexp.Compile(`\A(?:` + p.Expression + `)\z`)
		if err != nil {
			t.Fatalf("invalid target %q: %v", p.Expression, err)
		}
		expressions[i] = re
	}
	return func(value string) bool {
		for i, re := range expressions {
			if re.MatchString(value) == patterns[i].InvertMatch {
				return false
			}
		}
		return true
	}
}

func sourceMatcher(t testing.TB, source string) *regexp2.Regexp {
	t.Helper()
	re, err := regexp2.Compile(source, regexp2.ECMAScript)
	if err != nil {
		t.Fatal(err)
	}
	re.MatchTimeout = 100 * time.Millisecond
	return re
}

func candidates() []string {
	macs := []string{"00:11:22:aa:BB:ff", "00:11:22:aa:BB", "00:11:22:aa:BB:gg", "00:11:22:aa:BB:ff:00", "00:11:22:aa:BB:ff\n"}
	values := []string{"", "none", "none\n", "none\r", "none\r\n", "nonex", "xnone", "disabled", "disabled\n", "ab", "abb", "abc", "bb", "bc", "cd", "abab", "ababc", "0xFFFF", "0x0000", "0xfffff", "N/A", "1.23", "01", "swp1", "bond.1", "root$", "rt 1:2", "rt\u00a01:2", "\"'", `^$.*+?()[]{}|\`, "AB", "😀😀", "é中", "!@#%&=,:;'\"<>/~`"}
	for _, ch := range []rune{'\t', '\n', '\r', ' ', 0x7f, 0x85, 0xa0, 0x1680, 0x2000, 0x200b, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff, 'a', 'b', '0', '1', '9', '_', '-', '^', '$', '[', ']', '\\', 'é', '中', '😀', '٠', 0xd7ff, 0xe000, 0xfffd, 0x10000, 0x10ffff} {
		values = append(values, string(ch), "a"+string(ch), string(ch)+"b", "a"+string(ch)+"b", "none"+string(ch))
	}
	return append(values, macs...)
}

func TestDifferential(t *testing.T) {
	values := candidates()
	var enumerate func(string, int)
	enumerate = func(prefix string, remaining int) {
		values = append(values, prefix)
		if remaining == 0 {
			return
		}
		for _, ch := range "ab0\n" {
			enumerate(prefix+string(ch), remaining-1)
		}
	}
	enumerate("", 5)
	for _, source := range allPatterns(t) {
		t.Run(source, func(t *testing.T) {
			ps, err := Convert(source)
			if err != nil {
				// Interior lookahead deliberately appears in the corpus to exercise
				// fail-closed handling; all other corpus entries must translate.
				if source == `^(?=a(?=b)).*$` && errors.Is(err, ErrUnsupported) {
					return
				}
				t.Fatal(err)
			}
			target, original := targetMatcher(t, ps), sourceMatcher(t, source)
			for _, value := range values {
				want, err := original.MatchString(value)
				if err != nil {
					t.Fatal(err)
				}
				if got := target(value); got != want {
					t.Fatalf("value %q: target=%v source=%v; patterns=%+v", value, got, want, ps)
				}
			}
		})
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		source string
		want   []Pattern
	}{
		{`a`, []Pattern{{Expression: `[\s\S]*a[\s\S]*`}}},
		{`^a$`, []Pattern{{Expression: `a`}}},
		{`^\d+$`, []Pattern{{Expression: `([0-9])+`}}},
		{`^(?!none$).*$`, []Pattern{{Expression: `([^\n\r])*`}, {Expression: `none`, InvertMatch: true}}},
		{`^(?=a)(?!b)[a-z]+$`, []Pattern{{Expression: `([a-z])+`}, {Expression: `a[\s\S]*`}, {Expression: `b[\s\S]*`, InvertMatch: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got, err := Convert(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	// Ignoring capture resets must not allow capture-dependent patterns.
	for _, source := range []string{`^(a(b)?)+\2$`, `^(a)+(?(1)b|c)$`} {
		if got, err := Convert(source); !errors.Is(err, ErrUnsupported) || got != nil {
			t.Errorf("%q: got %+v, %v; want unsupported and no result", source, got, err)
		}
	}
	for _, source := range []string{`(a)\1`, `(?<x>a)\k<x>`, `(?<=a)b`, `(?<!a)b`, `\ba\b`, `\Ba`, `(?m)^a$`, `a^b`, `a$b`, `(?=a)a`, `a(?=b)`, `^(a$)*$`, `^(?!a)(b$|c)`, `^(?!(?=a)b).*$`, `(?>a*)a`, `(?>ab|a)b`, `(a)?(?(1)b|c)`, `\Gx`, `a\Z`} {
		t.Run(source, func(t *testing.T) {
			got, err := Convert(source)
			if !errors.Is(err, ErrUnsupported) || got != nil {
				t.Fatalf("got %+v, %v; want unsupported and no result", got, err)
			}
		})
	}
	for _, source := range []string{`[`, `(`, `a{2,1}`, "\xff"} {
		if got, err := Convert(source); err == nil || got != nil {
			t.Errorf("%q: got %+v, %v", source, got, err)
		}
	}
	for _, source := range []string{strings.Repeat("a", maxInput+1), strings.Repeat("(", maxDepth+1) + "a" + strings.Repeat(")", maxDepth+1)} {
		if got, err := Convert(source); !errors.Is(err, ErrLimit) || got != nil {
			t.Errorf("limit: got %+v, %v", got, err)
		}
	}
}

func TestUnicodeClasses(t *testing.T) {
	// Exercise the character-set membership fallback, including Unicode
	// categories and subtraction exposed by regexp2's AST.
	for _, source := range []string{`[\p{L}]`, `[^\p{Nd}]`, `[a-z-[aeiou]]`} {
		tree, err := syntax.Parse(source, syntax.ParseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		n := tree.Root.Children[0]
		text := character(n)
		target := targetMatcher(t, []Pattern{{Expression: text}})
		for ch := rune(0); ch <= 0x10ffff; ch++ {
			if xmlChar(ch) && target(string(ch)) != n.Set.CharIn(ch) {
				t.Fatalf("%s: U+%04X", source, ch)
			}
		}
	}
}

func TestRepetition(t *testing.T) {
	for _, n := range []int{0, 1, 2, 17} {
		for _, max := range []int{n, n + 1, math.MaxInt32} {
			source := fmt.Sprintf("^(ab){%d,%d}$", n, max)
			if max == math.MaxInt32 {
				source = fmt.Sprintf("^(ab){%d,}$", n)
			}
			ps, err := Convert(source)
			if err != nil {
				t.Fatal(err)
			}
			re, target := sourceMatcher(t, source), targetMatcher(t, ps)
			for count := 0; count < 20; count++ {
				value := strings.Repeat("ab", count)
				want, err := re.MatchString(value)
				if err != nil || target(value) != want {
					t.Fatalf("%s %q: %v", source, value, err)
				}
			}
		}
	}
}

func FuzzConvert(f *testing.F) {
	for _, source := range allPatterns(f) {
		f.Add(source, "none\n")
	}
	f.Fuzz(func(t *testing.T, source, value string) {
		if len(source) > 256 || len(value) > 128 {
			t.Skip()
		}
		for _, ch := range value {
			if !xmlChar(ch) {
				t.Skip()
			}
		}
		ps, err := Convert(source)
		if err != nil {
			return
		}
		// Go's regex compiler limits counted repeats to 1000; the converter does
		// not. Such targets are exercised through the actual YANG validator.
		for _, p := range ps {
			if _, err := regexp.Compile(p.Expression); err != nil {
				t.Skip()
			}
		}
		original := sourceMatcher(t, source)
		want, err := original.MatchString(value)
		if err != nil {
			t.Skip()
		}
		if got := targetMatcher(t, ps)(value); got != want {
			t.Fatalf("%q on %q: target=%v source=%v patterns=%+v", source, value, got, want, ps)
		}
	})
}

func BenchmarkConvert(b *testing.B) {
	for _, source := range []string{`^(?!none$).*$`, `^(swp|eth|bond|br|lo|vlan)[a-zA-Z0-9_./-]*$`, `^\S+$`} {
		b.Run(source, func(b *testing.B) {
			for b.Loop() {
				if _, err := Convert(source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestAtomicSafety(t *testing.T) {
	// Deliberately vary fixed/variable lengths, alternatives, optional tails,
	// and nested repetition. Atomicity may be removed only with a valid proof.
	sources := []string{
		`^a*(b|cd)$`, `^a*(b?c)$`, `^a*(bc)*$`, `^a*(b*|c+)$`, `^a*(b|)$`,
		`^(?>(ab){2})c$`, `^(?>ab|cd)e$`, `^(?>a{2}b{2})c$`, `^(?>[ab])c$`,
		`^(?>ab|a)c$`, `^(?>a*)a?$`, `^(?>a*)b*a$`, `^(?>a*)b?$`,
		`^(?>a*)(?=a)a$`, `^(?>a*)(b|a)$`, `^(?>a*)(b*|a+)$`, `^(?>a*)(b?a)$`,
		`^(?>a*)(?!)$`, `^(?>a*)(^|b)$`, `^(?>a*)(?>b|cd)$`,
		`^(?>(ab){2,3})c$`, `^(?>a(?=b))b$`, `^(?>ab|c)d$`,
	}
	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			ps, err := Convert(source)
			if errors.Is(err, ErrUnsupported) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			original, target := sourceMatcher(t, source), targetMatcher(t, ps)
			var check func(string, int)
			check = func(s string, left int) {
				want, err := original.MatchString(s)
				if err != nil {
					t.Fatal(err)
				}
				if target(s) != want {
					t.Fatalf("%q: target differs from regexp2; %+v", s, ps)
				}
				if left > 0 {
					for _, ch := range "abcde" {
						check(s+string(ch), left-1)
					}
				}
			}
			check("", 5)
		})
	}
}

func TestExpansionLimits(t *testing.T) {
	sources := []string{
		strings.Repeat(`(^|())`, 9) + `a`,
		strings.Repeat(`(^|())`, 8) + strings.Repeat("a", 4096),
	}
	var alternatives []string
	for i := 0; i <= maxBranches; i++ {
		alternatives = append(alternatives, fmt.Sprintf("^a%d$", i))
	}
	sources = append(sources, strings.Join(alternatives, "|"))
	for _, source := range sources {
		if ps, err := Convert(source); !errors.Is(err, ErrLimit) || ps != nil {
			t.Errorf("expansion limit: %v, %v", ps, err)
		}
	}
}
