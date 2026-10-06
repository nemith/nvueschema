package nvueschema

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nemith.io/nvueschema/yangregexp"
)

func TestYANGTypeStatements(t *testing.T) {
	var buf bytes.Buffer
	schema := &Config{Properties: map[string]*Config{"label": {Type: "string"}}}
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "};") {
		t.Error("a YANG block must not end with a semicolon")
	}
	t.Run("parse", func(t *testing.T) {
		parseYANG(t, buf.Bytes())
	})
}

func TestYANGPatternStrings(t *testing.T) {
	tests := []struct {
		name, pattern, literal string
	}{
		{"apostrophe", `[a-z']+`, `"[a-z']+"`},
		{"both-quotes", `[a-z'"]+`, `"[a-z'\"]+"`},
		{"backslashes", `\d+\.[a-z']+`, `"\\d+\\.[a-z']+"`},
		{"newline", "first \n  second'", `"first \n  second'"`},
		{"tab", "left\t'right", `"left\t'right"`},
		{"unicode", `[é中']+`, `"[é中']+"`},
	}
	schema := &Config{Properties: make(map[string]*Config)}
	wantPatterns := make(map[string]string)
	for _, tt := range tests {
		if got := yangString(tt.pattern); got != tt.literal {
			t.Errorf("%s: quoted literal = %s, want %s", tt.name, got, tt.literal)
		}
		patterns, err := yangregexp.Convert(tt.pattern)
		if err != nil {
			t.Fatal(err)
		}
		wantPatterns[tt.name] = patterns[0].Expression
		schema.Properties[tt.name] = &Config{Type: "string", Pattern: tt.pattern}
	}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		if literal := yangString(wantPatterns[tt.name]); !strings.Contains(buf.String(), "pattern "+literal+";") {
			t.Errorf("%s: missing correctly quoted pattern %s", tt.name, literal)
		}
	}

	t.Run("parse", func(t *testing.T) {
		// YIN exposes the parsed string value, so this verifies that quoting
		// preserves the regex rather than merely producing valid YANG syntax.
		var module struct {
			Leaves []struct {
				Name    string `xml:"name,attr"`
				Pattern struct {
					Value string `xml:"value,attr"`
				} `xml:"type>pattern"`
			} `xml:"container>leaf"`
		}
		if err := xml.Unmarshal(parseYANG(t, buf.Bytes()), &module); err != nil {
			t.Fatal(err)
		}
		patterns := make(map[string]string)
		for _, leaf := range module.Leaves {
			patterns[leaf.Name] = leaf.Pattern.Value
		}
		for _, tt := range tests {
			if got, ok := patterns[tt.name]; !ok || got != wantPatterns[tt.name] {
				t.Errorf("%s: parsed pattern = %q, want %q", tt.name, got, wantPatterns[tt.name])
			}
		}
	})
}

func parseYANG(t *testing.T, source []byte) []byte {
	t.Helper()
	python := testPython(t, "pyang")
	file := filepath.Join(t.TempDir(), "cumulus-nvue.yang")
	if err := os.WriteFile(file, source, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "-m", "pyang", "-f", "yin", file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pyang: %v\n%s", err, &stderr)
	}
	return out
}

func TestYANGDefaults(t *testing.T) {
	tests := []struct {
		name   string
		schema *Config
		want   string
	}{
		{"enum", &Config{Type: "string", Enum: []any{"packet", "byte"}, Default: "packet"}, "packet"},
		{"mac", &Config{Type: "string", Format: "mac", Default: "ff:ff:ff:ff:ff:ff"}, "ff:ff:ff:ff:ff:ff"},
		{"quoted", &Config{Type: "string", Default: "a'\"b"}, "a'\"b"},
		{"empty", &Config{Type: "string", Default: ""}, ""},
		{"integer", &Config{Type: "integer", Default: float64(4294967295)}, "4294967295"},
		{"boolean", &Config{Type: "boolean", Default: true}, "true"},
		{"union", &Config{AnyOf: []*Config{{Type: "integer"}, {Type: "string", Enum: []any{"auto"}}}, Default: "auto"}, "auto"},
	}
	schema := &Config{Properties: make(map[string]*Config)}
	for _, tt := range tests {
		schema.Properties[tt.name] = tt.schema
	}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		if want := "default " + yangString(tt.want) + ";"; !strings.Contains(buf.String(), want) {
			t.Errorf("%s: missing %s", tt.name, want)
		}
	}
	t.Run("parse", func(t *testing.T) {
		var module struct {
			Leaves []struct {
				Name    string `xml:"name,attr"`
				Default struct {
					Value string `xml:"value,attr"`
				} `xml:"default"`
			} `xml:"container>leaf"`
		}
		if err := xml.Unmarshal(parseYANG(t, buf.Bytes()), &module); err != nil {
			t.Fatal(err)
		}
		defaults := make(map[string]string)
		for _, leaf := range module.Leaves {
			defaults[leaf.Name] = leaf.Default.Value
		}
		for _, tt := range tests {
			if got, ok := defaults[tt.name]; !ok || got != tt.want {
				t.Errorf("%s: parsed default = %q, want %q", tt.name, got, tt.want)
			}
		}
	})
}

func TestYANGIntegerRanges(t *testing.T) {
	zero, lower, upper := 0.0, 96.0, float64(math.MaxUint32)
	signedMin, signedMax, unsignedMax := float64(math.MinInt64), float64(math.MaxInt64), float64(math.MaxUint64)
	schema := &Config{Properties: map[string]*Config{
		"bounded":  {Type: "integer", Minimum: &lower, Maximum: &upper, Default: float64(960)},
		"signed":   {Type: "integer", Minimum: &signedMin, Maximum: &signedMax},
		"unsigned": {Type: "integer", Format: "integer", Minimum: &zero, Maximum: &unsignedMax},
	}}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`range "96..4294967295";`, `range "-9223372036854775808..max";`, `range "0..max";`, "type uint64"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	t.Run("validate", func(t *testing.T) {
		checkYANGValues(t, buf.Bytes(), map[string]yangValues{
			"bounded":  {Good: []string{"96", "960", "4294967295"}, Bad: []string{"95", "4294967296"}},
			"signed":   {Good: []string{"-9223372036854775808", "9223372036854775807"}, Bad: []string{"-9223372036854775809", "9223372036854775808"}},
			"unsigned": {Good: []string{"0", "18446744073709551615"}, Bad: []string{"-1", "18446744073709551616"}},
		})
	})
}

type yangValues struct{ Good, Bad []string }

func TestYANGAllOfScalarUnions(t *testing.T) {
	for _, composition := range []string{"anyOf", "oneOf"} {
		t.Run(composition, func(t *testing.T) {
			// NVUE 5.1 wraps the IPv4-or-auto definition for the VXLAN
			// source address in allOf to attach a description and default.
			variants := []*Config{
				{Type: "string", Format: "ipv4", Nullable: true},
				{Type: "string", Enum: []any{"auto", nil}, Nullable: true},
			}
			union := &Config{}
			if composition == "anyOf" {
				union.AnyOf = variants
			} else {
				union.OneOf = variants
			}
			address := &Config{
				Type: "string", Nullable: true, Default: "auto",
				Description: "VXLAN source address",
				AllOf:       []*Config{union},
			}
			schema := &Config{Properties: map[string]*Config{
				"address": address,
				"nested":  {AllOf: []*Config{address}},
				"choice":  {AnyOf: []*Config{address, {Type: "string", Enum: []any{"none"}}}},
			}}
			var buf bytes.Buffer
			if err := WriteYANG(&buf, schema, nil); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"type union {", "type inet:ipv4-address;", `enum "auto";`, `default "auto";`, `"VXLAN source address";`} {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("missing %s", want)
				}
			}
			for _, metadata := range []string{`default "auto";`, `"VXLAN source address";`} {
				if got := strings.Count(buf.String(), metadata); got != 2 {
					t.Errorf("%s appears %d times, want 2 (address and nested)", metadata, got)
				}
			}
			t.Run("validate", func(t *testing.T) {
				checkYANGValues(t, buf.Bytes(), map[string]yangValues{
					"address": {Good: []string{"192.0.2.1", "auto"}, Bad: []string{"999.0.0.1", "2001:db8::1", "manual", "none"}},
					"nested":  {Good: []string{"192.0.2.1", "auto"}, Bad: []string{"999.0.0.1", "manual"}},
					"choice":  {Good: []string{"192.0.2.1", "auto", "none"}, Bad: []string{"999.0.0.1", "manual"}},
				})
			})
		})
	}
}

func TestYANGConstrainedAllOfUnion(t *testing.T) {
	value := &Config{
		Type: "integer", Minimum: new(10.0),
		AllOf: []*Config{{AnyOf: []*Config{
			{Type: "integer", Minimum: new(0.0), Maximum: new(20.0)},
			{Type: "string", Enum: []any{"auto"}},
		}}},
	}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, &Config{Properties: map[string]*Config{"value": value}}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "type union {") || !strings.Contains(buf.String(), `range "10..20";`) {
		t.Fatal("discarded the allOf wrapper's type or minimum constraint")
	}
	t.Run("validate", func(t *testing.T) {
		checkYANGValues(t, buf.Bytes(), map[string]yangValues{
			"value": {Good: []string{"10", "20"}, Bad: []string{"0", "9", "21", "auto"}},
		})
	})
}

func TestYANGNullAlternatives(t *testing.T) {
	schema := &Config{Properties: map[string]*Config{
		"mac":        {AnyOf: []*Config{{Type: "string", Format: "mac"}, {Type: "string", Nullable: true, Enum: []any{nil}}}},
		"unset":      {Enum: []any{nil}},
		"null-union": {AnyOf: []*Config{{Enum: []any{nil}}, {Type: "null"}}},
		"plane":      {Enum: []any{float64(0), nil}},
	}}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"leaf unset", "leaf null-union", "type enumeration"} {
		if strings.Contains(buf.String(), absent) {
			t.Errorf("unexpected %s", absent)
		}
	}
	t.Run("validate", func(t *testing.T) {
		checkYANGValues(t, buf.Bytes(), map[string]yangValues{
			"mac":   {Good: []string{"00:11:22:33:44:55"}, Bad: []string{"", "invalid"}},
			"plane": {Good: []string{"0"}, Bad: []string{"1", "auto"}},
		})
	})
}

func TestYANGDecimalTypes(t *testing.T) {
	lo, hi, tiny, huge := 0.001, 3500.0, 0.00000001, 1e16
	schema := &Config{Properties: map[string]*Config{
		"size":      {Type: "number", Format: "float", Minimum: &lo, Maximum: &hi, Default: 10.0},
		"precise":   {Type: "number", Minimum: &tiny, Maximum: &hi},
		"large":     {Type: "number", Maximum: &huge},
		"unbounded": {Type: "number"},
		"union":     {AnyOf: []*Config{{Type: "number", Minimum: &lo, Maximum: &hi}, {Type: "string", Enum: []any{"auto"}}}},
	}}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fraction-digits 6;", "fraction-digits 8;", "fraction-digits 2;"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	t.Run("validate", func(t *testing.T) {
		checkYANGValues(t, buf.Bytes(), map[string]yangValues{
			"size":      {Good: []string{"0.001", "23.05", "3500"}, Bad: []string{"0.0009", "3500.001"}},
			"precise":   {Good: []string{"0.00000001", "3500"}, Bad: []string{"0.000000001", "3500.1"}},
			"large":     {Good: []string{"10000000000000000"}, Bad: []string{"10000000000000001"}},
			"unbounded": {Good: []string{"-0.000001", "42.5"}, Bad: []string{"1.0000001"}},
			"union":     {Good: []string{"0.001", "3500", "auto"}, Bad: []string{"0", "3500.001"}},
		})
	})
	t.Run("unrepresentable", func(t *testing.T) {
		for _, invalid := range []*Config{
			{Type: "number", Minimum: &tiny, Maximum: &huge},
			{Type: "number", Minimum: new(math.Inf(1))},
			{Type: "number", Minimum: new(1e-19)},
		} {
			err := WriteYANG(&bytes.Buffer{}, &Config{Properties: map[string]*Config{"nested": {Properties: map[string]*Config{"value": invalid}}}}, nil)
			if err == nil || !strings.Contains(err.Error(), "nested: value: decimal64") {
				t.Errorf("expected contextual precision error, got %v", err)
			}
		}
	})
}

func TestYANGNumericEnumsAndUnions(t *testing.T) {
	lo, hi := 2.0, 3.0
	schema := &Config{Properties: map[string]*Config{
		"version":    {Type: "integer", Enum: []any{float64(3), float64(2), nil, float64(3)}, Default: float64(2)},
		"bounded":    {AnyOf: []*Config{{Type: "integer", Minimum: &lo, Maximum: &hi}, {Type: "string", Enum: []any{"auto"}}}},
		"enum-union": {AnyOf: []*Config{{Type: "integer", Enum: []any{2, 3, nil}}, {Type: "string", Enum: []any{"auto"}}}},
		"wrapped":    {AnyOf: []*Config{{Type: "integer", Enum: []any{3, nil}}}},
		"intersect":  {Type: "integer", Enum: []any{1, 2, 3, 4}, Minimum: &lo, Maximum: &hi},
	}}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `range "2 | 3";`) {
		t.Error("numeric enum range missing")
	}
	t.Run("validate", func(t *testing.T) {
		checkYANGValues(t, buf.Bytes(), map[string]yangValues{
			"version":    {Good: []string{"2", "3"}, Bad: []string{"1", "4", "auto"}},
			"bounded":    {Good: []string{"2", "3", "auto"}, Bad: []string{"1", "4"}},
			"enum-union": {Good: []string{"2", "3", "auto"}, Bad: []string{"1", "4"}},
			"wrapped":    {Good: []string{"3"}, Bad: []string{"2", "4"}},
			"intersect":  {Good: []string{"2", "3"}, Bad: []string{"1", "4"}},
		})
	})
}

// Exercise pyang's resolved type restrictions with actual leaf values.
func checkYANGValues(t *testing.T, source []byte, values map[string]yangValues) {
	t.Helper()
	python := testPython(t, "pyang")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cumulus-nvue.yang"), source, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "-c", `
import json, sys
from pyang import context, error, repository
ctx = context.Context(repository.FileRepository("."))
with open("cumulus-nvue.yang") as f:
    module = ctx.add_module("cumulus-nvue.yang", f.read())
ctx.validate()
errors = [(str(pos), tag, args) for pos, tag, args in ctx.errors if error.is_error(error.err_level(tag))]
assert not errors, errors
container = module.search_one("container")
for name, cases in json.load(sys.stdin).items():
    leaf = next(s for s in container.substmts if s.keyword == "leaf" and s.arg == name)
    spec = leaf.search_one("type").i_type_spec
    for category, expected in (("Good", True), ("Bad", False)):
        for text in cases[category] or []:
            errors = []
            value = spec.str_to_val(errors, leaf.pos, text, module)
            valid = value is not None and spec.validate(errors, leaf.pos, value, module)
            assert bool(valid) == expected, (name, text, expected, errors)
`)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(data)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("YANG values: %v\n%s", err, out)
	}
}

// Set NVUESCHEMA_PYTHON to a Python environment with pyang and pydantic to
// exercise the generated artifacts in addition to the Go-only tests.
func testPython(t *testing.T, module string) string {
	t.Helper()
	python := os.Getenv("NVUESCHEMA_PYTHON")
	if python == "" {
		var err error
		python, err = exec.LookPath("python3")
		if err != nil {
			t.Skip("install Python for generator integration tests")
		}
	}
	if out, err := exec.Command(python, "-c", "import "+module).CombinedOutput(); err != nil {
		if os.Getenv("NVUESCHEMA_PYTHON") != "" {
			t.Fatalf("configured Python lacks %s: %v\n%s", module, err, out)
		}
		t.Skipf("install %s for generator integration tests", module)
	}
	return python
}

func TestYANGPatternSemantics(t *testing.T) {
	schema := &Config{Properties: map[string]*Config{
		"profile": {Type: "string", Pattern: `^(?!none$).*$`},
		"hex":     {Type: "string", Pattern: `^0x([0-9A-Fa-f]{1,4})$`, Default: "0xFFFF"},
		"search":  {Type: "string", Pattern: `key[0-9]+`},
		"prefix":  {Type: "string", Pattern: `^/`},
		"guards":  {Type: "string", Pattern: `^(?=swp)(?!swp0$)[a-z0-9]+$`},
	}}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "yang-version 1.1;") || !strings.Contains(buf.String(), "modifier invert-match;") {
		t.Fatal("missing YANG 1.1 pattern modifier")
	}
	t.Run("validate", func(t *testing.T) {
		checkYANGValues(t, buf.Bytes(), map[string]yangValues{
			"profile": {Good: []string{"", "none1", "a'\"b"}, Bad: []string{"none", "a\nb", "a\rb", "none\n"}},
			"hex":     {Good: []string{"0x0", "0xFFFF"}, Bad: []string{"x0x0", "0x00000", "0x0\n"}},
			"search":  {Good: []string{"key1", "xkey99z", "\nkey1\n"}, Bad: []string{"key", "keyx"}},
			"prefix":  {Good: []string{"/", "/a\nb"}, Bad: []string{"x/", "\n/"}},
			"guards":  {Good: []string{"swp1", "swp01"}, Bad: []string{"swp0", "eth1", "swp1\n"}},
		})
	})
}

func TestYANGUnsupportedPattern(t *testing.T) {
	schema := &Config{Properties: map[string]*Config{"interface": {Properties: map[string]*Config{"label": {Type: "string", Pattern: `^(a+)\1$`}}}}}
	var buf bytes.Buffer
	err := WriteYANG(&buf, schema, nil)
	if err == nil || !strings.Contains(err.Error(), "interface") || !strings.Contains(err.Error(), "label") || !strings.Contains(err.Error(), "unsupported regex conversion") {
		t.Fatalf("missing field-specific conversion error: %v", err)
	}
}

func TestYANGUnicodePatternQuoting(t *testing.T) {
	schema := &Config{Properties: map[string]*Config{
		"space":     {Type: "string", Pattern: `^\s+$`},
		"literal":   {Type: "string", Pattern: "^a \u0085 \u2028 \u2029 b$"},
		"interface": {Type: "string", Format: "interface-name", Default: "eth0"},
	}}
	var buf bytes.Buffer
	if err := WriteYANG(&buf, schema, nil); err != nil {
		t.Fatal(err)
	}
	checkYANGValues(t, buf.Bytes(), map[string]yangValues{
		"space":     {Good: []string{" ", "\t", "\u200a", "\u2028", "\u2029", "\ufeff"}, Bad: []string{"\u200b", "\u0085", "a"}},
		"literal":   {Good: []string{"a \u0085 \u2028 \u2029 b"}, Bad: []string{"a\u0085\u2028\u2029b", "a   b"}},
		"interface": {Good: []string{"eth0", "swp1", "swp1.2", "bond-1"}, Bad: []string{"what0", "eth!", "eth\n"}},
	})
}
