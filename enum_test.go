package nvueschema

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestCompositeEnumValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  *Config
		valid   []any
		invalid []any
	}{
		{
			name: "allOf_reference",
			schema: &Config{
				Type: "string", Nullable: true,
				AllOf: []*Config{{Enum: []any{"enabled", "disabled", nil}}},
			},
			valid: []any{"enabled", "disabled", nil}, invalid: []any{"on", "off", 1},
		},
		{
			name: "nested_wrapper",
			schema: &Config{
				AnyOf: []*Config{{AllOf: []*Config{{Type: "string", Enum: []any{"auto"}}}}},
			},
			valid: []any{"auto"}, invalid: []any{"manual", nil},
		},
		{
			name: "parent_precedence",
			schema: &Config{
				Type: "string", Enum: []any{"enabled"},
				AllOf: []*Config{{Enum: []any{"enabled", "disabled"}}},
			},
			valid: []any{"enabled"}, invalid: []any{"disabled"},
		},
		{
			name: "allOf_type_and_enum",
			schema: &Config{
				AllOf: []*Config{
					{Type: "string"},
					{Enum: []any{"enabled", "disabled"}},
				},
			},
			valid: []any{"enabled", "disabled"}, invalid: []any{"on"},
		},
		{
			name: "distinct_union_enums",
			schema: &Config{
				AnyOf: []*Config{
					{OneOf: []*Config{
						{Type: "string", Enum: []any{"enabled"}},
						{Type: "string", Enum: []any{"disabled"}},
					}},
					{Type: "integer", Minimum: new(3.0), Maximum: new(10.0)},
				},
			},
			valid: []any{"enabled", "disabled", 3, 10}, invalid: []any{"on", "3", 2, 11},
		},
		{
			name: "anyOf_certificate_name",
			schema: &Config{
				Type: "string", Nullable: true,
				AnyOf: []*Config{
					{AllOf: []*Config{{Type: "string", MinLength: new(1), MaxLength: new(64)}}},
					{Enum: []any{"self-signed", nil}},
				},
			},
			valid:   []any{"my-cert", "self-signed", strings.Repeat("a", 64), nil},
			invalid: []any{"", strings.Repeat("a", 65), 1},
		},
		{
			name: "anyOf_enum_before_certificate_name",
			schema: &Config{
				Type: "string", Nullable: true,
				AnyOf: []*Config{
					{Enum: []any{"self-signed", nil}},
					{AllOf: []*Config{{Type: "string", MinLength: new(1), MaxLength: new(64)}}},
				},
			},
			valid: []any{"my-cert", "self-signed", nil}, invalid: []any{"", 1},
		},
		{
			name: "anyOf_enum_alternatives",
			schema: &Config{
				AnyOf: []*Config{
					{AllOf: []*Config{{Type: "string", Enum: []any{"enabled"}}}},
					{AllOf: []*Config{{Type: "string", Enum: []any{"disabled"}}}},
				},
			},
			valid: []any{"enabled", "disabled"}, invalid: []any{"on", 1},
		},
		{
			name: "oneOf_enum_alternatives",
			schema: &Config{
				OneOf: []*Config{
					{AllOf: []*Config{{Type: "string", Enum: []any{"enabled"}}}},
					{AllOf: []*Config{{Type: "string", Enum: []any{"disabled"}}}},
				},
			},
			valid: []any{"enabled", "disabled"}, invalid: []any{"on", 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := compileTestSchema(t, tt.schema)
			for _, value := range tt.valid {
				if err := schema.Validate(value); err != nil {
					t.Errorf("Validate(%#v) = %v, want valid", value, err)
				}
			}
			for _, value := range tt.invalid {
				if err := schema.Validate(value); err == nil {
					t.Errorf("Validate(%#v) succeeded, want enum or type rejection", value)
				}
			}
		})
	}
}

func TestCompositeEnumOnlyProperty(t *testing.T) {
	parser, err := NewParser(strings.NewReader(`{
		"x-defs": {
			"cue-patch-schema-root-root": {
				"type": "object",
				"properties": {"mode": {"allOf": [{"enum": [0, 1]}]}}
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	config, err := parser.ConfigSchema()
	if err != nil {
		t.Fatal(err)
	}
	schema := compileTestSchema(t, config)
	if err := schema.Validate(map[string]any{"mode": 1}); err != nil {
		t.Errorf("enum-only property was pruned: %v", err)
	}
	if err := schema.Validate(map[string]any{"mode": 2}); err == nil {
		t.Error("out-of-enum property value was accepted")
	}
}

func TestCompositeEnumOverlappingAlternatives(t *testing.T) {
	config := &Config{AnyOf: []*Config{
		{AllOf: []*Config{{Type: "string", Enum: []any{"enabled", "shared"}}}},
		{AllOf: []*Config{{Type: "string", Enum: []any{"disabled", "shared"}}}},
	}}
	enum, ok := config.JSONSchemaDoc()["enum"].([]any)
	if !ok || !slices.Equal(enum, []any{"enabled", "shared", "disabled"}) {
		t.Fatalf("combined enum = %#v, want unique values from both alternatives", enum)
	}
	resolved := compileTestSchema(t, config)
	for _, value := range []string{"enabled", "shared", "disabled"} {
		if err := resolved.Validate(value); err != nil {
			t.Errorf("Validate(%q) = %v", value, err)
		}
	}
	if err := resolved.Validate("on"); err == nil {
		t.Error("out-of-enum value was accepted")
	}
}

func TestCompositeEnumNullableFormats(t *testing.T) {
	properties := map[string]*Config{
		"ip": {Type: "string", Format: "ipv4"},
	}
	for name, format := range map[string]string{"address": "mac", "domain-name": "domain-name"} {
		properties[name] = &Config{
			Type: "string", Nullable: true,
			AnyOf: []*Config{
				{AllOf: []*Config{{Type: "string", Format: format}}},
				{Enum: []any{nil}},
			},
		}
	}
	var generated bytes.Buffer
	if err := WriteGoStructs(&generated, &Config{Type: "object", Properties: properties}, nil); err != nil {
		t.Fatal(err)
	}
	generated.WriteString(`
func configure(c *NvueConfig) {
	c.Address = nil
	c.DomainName = nil
	c.Address = new(MacAddress("00:11:22:33:44:55"))
	c.DomainName = new(Hostname("example.com"))
	var _ *MacAddress = c.Address
	var _ *Hostname = c.DomainName
}
`)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "config.go", generated.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	checker := types.Config{Importer: importer.Default()}
	if _, err := checker.Check("config", fset, []*ast.File{file}, nil); err != nil {
		t.Fatalf("generated format fields cannot represent nullable typed values: %v", err)
	}
}

func compileTestSchema(t *testing.T, config *Config) *jsonschema.Resolved {
	t.Helper()
	data, err := json.Marshal(config.JSONSchemaDoc())
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
