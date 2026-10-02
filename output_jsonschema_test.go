package nvueschema

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestJSONSchemaNegativeLookahead(t *testing.T) {
	for _, nullable := range []bool{false, true} {
		cfg := &Config{Type: "string", Nullable: nullable, Pattern: `^(?!none$).*$`}
		resolved := resolveJSONSchemaForTest(t, cfg)
		for _, value := range []string{"", "bfd-profile", "None", "none-other", "nonetheless"} {
			if err := resolved.Validate(value); err != nil {
				t.Errorf("nullable=%t: valid value %q rejected: %v", nullable, value, err)
			}
		}
		for _, value := range []any{"none", "two\nlines", 42} {
			if err := resolved.Validate(value); err == nil {
				t.Errorf("nullable=%t: invalid value %#v accepted", nullable, value)
			}
		}
		if err := resolved.Validate(nil); (err == nil) != nullable {
			t.Errorf("nullable=%t: null validation error = %v", nullable, err)
		}
	}
}

func TestJSONSchemaNegativeLookaheadNestedUnion(t *testing.T) {
	// NVUE BFD profiles use an ordinary pattern leaf. Exercise it inside a
	// dictionary and a scalar union too, so both JSON Schema emission paths
	// retain the exclusion without rejecting a separate explicit sentinel.
	cfg := &Config{Type: "object", AdditionalProperties: &Config{
		AnyOf: []*Config{
			{Type: "string", Pattern: `^(?!none$).*$`},
			{Type: "string", Enum: []any{"none"}},
		},
	}}
	resolved := resolveJSONSchemaForTest(t, cfg)
	if err := resolved.Validate(map[string]any{"swp1": "bfd-profile", "swp2": "none"}); err != nil {
		t.Fatalf("valid union values rejected: %v", err)
	}
	if err := resolved.Validate(map[string]any{"swp1": "two\nlines"}); err == nil {
		t.Fatal("invalid union value accepted")
	}
}

func TestJSONSchemaOtherPatternsPreserved(t *testing.T) {
	for _, pattern := range []string{`^[a-z]+$`, `^(?!other$).*$`} {
		got := (&Config{Type: "string", Pattern: pattern}).ToJSONSchema()
		if got["pattern"] != pattern {
			t.Errorf("pattern = %v, want %q", got["pattern"], pattern)
		}
		if _, ok := got["not"]; ok {
			t.Errorf("unexpected exclusion for pattern %q", pattern)
		}
	}
}

func resolveJSONSchemaForTest(t *testing.T, cfg *Config) *jsonschema.Resolved {
	t.Helper()
	data, err := json.Marshal(cfg.JSONSchemaDoc())
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolving generated JSON Schema: %v", err)
	}
	return resolved
}
