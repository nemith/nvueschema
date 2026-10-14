package nvueschema

import (
	"errors"
	"testing"
)

func TestValidator(t *testing.T) {
	min := 552.0
	validator, err := NewValidator(&Config{Type: "object", Properties: map[string]*Config{
		"mtu":     {Type: "integer", Minimum: &min},
		"profile": {Type: "string", Pattern: `^(?!none$).*$`},
		"repeat":  {Type: "string", Pattern: `^([a-z]+)-\1$`},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{
		map[string]any{"mtu": 1500.0, "profile": "fast", "repeat": "same-same"},
		map[string]any{},
	} {
		if err := validator.Validate(value); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]any{"mtu": -1.0, "profile": "none", "repeat": "same-other"} {
		input := map[string]any{key: value}
		err := validator.Validate(input)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || len(invalid.Issues) == 0 {
			t.Fatalf("%s: %v", key, err)
		}
		found := false
		for _, issue := range invalid.Issues {
			if issue.Path == "/"+key && issue.Message != "" && issue.SchemaPath != "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing field location: %+v", invalid)
		}
		if input[key] != value {
			t.Fatal("validator mutated input")
		}
	}
	if err := validator.Validate(map[string]any{"mtu": "1500"}); err == nil {
		t.Fatal("coerced string to integer")
	}
}

func TestValidatorBadSchema(t *testing.T) {
	if _, err := NewValidator(nil); err == nil {
		t.Fatal("accepted nil schema")
	}
	if _, err := NewValidator(&Config{Type: "string", Pattern: "["}); err == nil {
		t.Fatal("accepted invalid regex")
	}
}
