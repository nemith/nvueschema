package main

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
			for k, v := range flat.Properties {
				merged.Properties[k] = v
			}
			merged.Required = append(merged.Required, flat.Required...)
			if flat.AdditionalProperties != nil {
				merged.AdditionalProperties = flat.AdditionalProperties
			}
		}
	}

	// Also merge properties from the top-level schema itself.
	for k, v := range s.Properties {
		merged.Properties[k] = v
	}
	merged.Required = append(merged.Required, s.Required...)
	return merged
}
