package nvueschema

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validator is a compiled configuration schema. Reuse it for multiple instances.
type Validator struct{ schema *jsonschema.Schema }

// ValidationIssue identifies a failed constraint using JSON Pointer locations.
type ValidationIssue struct {
	Path       string `json:"path"`
	SchemaPath string `json:"schema_path"`
	Message    string `json:"message"`
}

// ValidationError reports schema violations, rather than schema-loading errors.
type ValidationError struct {
	Issues []ValidationIssue `json:"issues"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed: %v", e.Issues)
}

// NewValidator compiles the configuration schema using ECMAScript regex semantics.
func NewValidator(config *Config) (*Validator, error) {
	if config == nil {
		return nil, errors.New("configuration schema is nil")
	}
	data, err := json.Marshal(config.JSONSchemaDoc())
	if err != nil {
		return nil, fmt.Errorf("marshaling schema: %w", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(CompilePattern)
	const location = "https://nvueschema.invalid/config.json"
	if err := compiler.AddResource(location, doc); err != nil {
		return nil, fmt.Errorf("loading schema: %w", err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}
	return &Validator{schema: schema}, nil
}

// Validate checks a decoded JSON-compatible value without coercing or modifying it.
// A schema mismatch returns *ValidationError.
func (v *Validator) Validate(instance any) error {
	err := v.schema.Validate(instance)
	if err == nil {
		return nil
	}
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return err
	}
	output := invalid.BasicOutput()
	units := output.Errors
	if len(units) == 0 {
		units = []jsonschema.OutputUnit{*output}
	}
	issues := make([]ValidationIssue, 0, len(units))
	for _, unit := range units {
		if unit.Error != nil {
			issues = append(issues, ValidationIssue{
				Path: unit.InstanceLocation, SchemaPath: unit.KeywordLocation,
				Message: unit.Error.String(),
			})
		}
	}
	return &ValidationError{Issues: issues}
}
