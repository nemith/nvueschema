package nvueschema

import (
	"fmt"
	"maps"
	"strings"
)

// TypeSegment is a piece of a type annotation, either a type name or a literal value.
type TypeSegment struct {
	Text    string
	Literal bool // true for enum/literal values, false for type names
}

// FlattenComposite merges allOf, anyOf, and oneOf into a single Schema by
// combining all their properties, additionalProperties, etc. In the NVUE
// spec these composition keywords are used to express "this object has all
// of these property groups", so merging is the right interpretation.
func FlattenComposite(s *Config) *Config {
	if s == nil {
		return &Config{}
	}
	if len(s.AllOf) == 0 && len(s.AnyOf) == 0 && len(s.OneOf) == 0 {
		return s
	}
	merged := &Config{
		Description: s.Description,
		Type:        s.Type,
		Nullable:    s.Nullable,
		Enum:        s.Enum,
		Default:     s.Default,
		Format:      s.Format,
		Pattern:     s.Pattern,
		Minimum:     s.Minimum,
		Maximum:     s.Maximum,
		MinLength:   s.MinLength,
		MaxLength:   s.MaxLength,
		Items:       s.Items,
		Properties:  make(map[string]*Config),
	}

	// Merge all composition variants — they all contribute properties
	// and leaf-level fields when not already set on the top-level schema.
	for _, group := range [][]*Config{s.AllOf, s.AnyOf, s.OneOf} {
		for _, sub := range group {
			flat := FlattenComposite(sub)
			if flat.Description != "" && merged.Description == "" {
				merged.Description = flat.Description
			}
			if flat.Type != "" && merged.Type == "" {
				merged.Type = flat.Type
			}
			if flat.Format != "" && merged.Format == "" {
				merged.Format = flat.Format
			}
			if flat.Default != nil && merged.Default == nil {
				merged.Default = flat.Default
			}
			if flat.Pattern != "" && merged.Pattern == "" {
				merged.Pattern = flat.Pattern
			}
			if flat.Minimum != nil && merged.Minimum == nil {
				merged.Minimum = flat.Minimum
			}
			if flat.Maximum != nil && merged.Maximum == nil {
				merged.Maximum = flat.Maximum
			}
			if flat.MinLength != nil && merged.MinLength == nil {
				merged.MinLength = flat.MinLength
			}
			if flat.MaxLength != nil && merged.MaxLength == nil {
				merged.MaxLength = flat.MaxLength
			}
			if flat.Items != nil && merged.Items == nil {
				merged.Items = flat.Items
			}
			maps.Copy(merged.Properties, flat.Properties)
			merged.Required = append(merged.Required, flat.Required...)
			if flat.AdditionalProperties != nil {
				merged.AdditionalProperties = flat.AdditionalProperties
			}
		}
	}

	// Also merge properties from the top-level schema itself.
	maps.Copy(merged.Properties, s.Properties)
	merged.Required = append(merged.Required, s.Required...)
	return merged
}

// isScalarUnion returns true if the schema is an anyOf/oneOf where every
// branch is a scalar type (no properties, no additionalProperties).
func isScalarUnion(s *Config) bool {
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}
	if len(variants) == 0 {
		return false
	}
	for _, v := range variants {
		if v.Properties != nil || v.AdditionalProperties != nil ||
			len(v.AllOf) > 0 || len(v.AnyOf) > 0 || len(v.OneOf) > 0 {
			return false
		}
	}
	return true
}

// hasProps returns true if the schema has at least one property.
func hasProps(s *Config) bool {
	return len(s.Properties) > 0
}

// sourceRefFor finds the best SourceRef from a schema or its composition branches.
func sourceRefFor(s *Config) string {
	if s.SourceRef != "" {
		return s.SourceRef
	}
	for _, group := range [][]*Config{s.AllOf, s.AnyOf, s.OneOf} {
		for _, sub := range group {
			if sub.SourceRef != "" {
				return sub.SourceRef
			}
		}
	}
	return ""
}

// shortDesc returns the first line of a description, truncated to 60 chars.
func shortDesc(s string) string {
	if s == "" {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimRight(s, "\n"), "\n")
	if len(first) > 60 {
		first = first[:57] + "..."
	}
	return first
}

// enumString formats enum values as a comma-separated string.
func enumString(vals []any) string {
	var parts []string
	for _, v := range vals {
		if v != nil {
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.Join(parts, ",")
}
