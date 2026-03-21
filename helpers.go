package main

import (
	"fmt"
	"maps"
	"math"
)

// Well-known integer constants for display.
var knownConstants = map[float64]string{
	math.MaxUint32: "UINT32_MAX",
	math.MaxInt32:  "INT32_MAX",
	math.MaxUint16: "UINT16_MAX",
	math.MaxInt16:  "INT16_MAX",
}

// fmtNum formats a float64 as a clean integer when possible,
// substituting well-known constants for readability.
func fmtNum(v float64) string {
	if name, ok := knownConstants[v]; ok {
		return name
	}
	// If it's a whole number, format without decimals.
	if v == math.Trunc(v) && !math.IsInf(v, 0) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

// fmtNumPtr formats a *float64 for display.
func fmtNumPtr(p *float64) string {
	if p == nil {
		return "(none)"
	}
	return fmtNum(*p)
}

// quoteLiteral quotes string literals but leaves numeric values bare.
func quoteLiteral(s string) string {
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-' || s[0] == '.') {
		return s
	}
	return fmt.Sprintf("%q", s)
}

// fmtDefault formats a default value for display.
// Numbers are formatted cleanly, strings are quoted.
func fmtDefault(v any) string {
	switch n := v.(type) {
	case float64:
		return fmtNum(n)
	case string:
		return quoteLiteral(n)
	default:
		return fmt.Sprint(v)
	}
}

// flattenComposite merges allOf, anyOf, and oneOf into a single Schema by
// combining all their properties, additionalProperties, etc. In the NVUE
// spec these composition keywords are used to express "this object has all
// of these property groups", so merging is the right interpretation.
func flattenComposite(s *Schema) *Schema {
	if s == nil {
		return &Schema{}
	}
	if len(s.AllOf) == 0 && len(s.AnyOf) == 0 && len(s.OneOf) == 0 {
		return s
	}
	merged := &Schema{
		Description: s.Description,
		Type:        s.Type,
		Nullable:    s.Nullable,
		Properties:  make(map[string]*Schema),
	}

	// Merge all composition variants — they all contribute properties.
	for _, group := range [][]*Schema{s.AllOf, s.AnyOf, s.OneOf} {
		for _, sub := range group {
			flat := flattenComposite(sub)
			if flat.Description != "" && merged.Description == "" {
				merged.Description = flat.Description
			}
			if flat.Type != "" && merged.Type == "" {
				merged.Type = flat.Type
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
