// Package nvueschema extracts and generates config schemas from Cumulus Linux NVUE OpenAPI specs.
package nvueschema

import (
	"fmt"
	"sort"
	"strings"
)

// Diff holds the result of comparing two schemas.
type Diff struct {
	Changes []Change
}

// Change represents a single schema difference.
type Change struct {
	Path       string
	Kind       string // "added", "removed", "changed"
	Desc       string
	TypeSegs   []TypeSegment // type info for added/removed leaves
	DefaultVal string        // default value for added/removed leaves
}

// DiffSchemas compares two schemas and returns a Diff.
func DiffSchemas(old, newer *Config, path string) *Diff {
	return &Diff{Changes: diffSchemas(old, newer, path)}
}

// Filter returns a new Diff containing only changes under the given paths.
func (d *Diff) Filter(paths []string) *Diff {
	return &Diff{Changes: filterChanges(d.Changes, paths)}
}

// FilterAffected returns a new Diff containing only changes that overlap with
// the given config paths. A change is included if it is at, above, or below
// any config path (bidirectional prefix match).
func (d *Diff) FilterAffected(configPaths []string) *Diff {
	var out []Change
	for _, c := range d.Changes {
		if affectMatches(c.Path, configPaths) {
			out = append(out, c)
		}
	}
	return &Diff{Changes: out}
}

func diffSchemas(old, newer *Config, path string) []Change {
	oldFlat := FlattenComposite(old)
	newFlat := FlattenComposite(newer)

	var changes []Change

	// Collect property names from both.
	oldProps := propNames(oldFlat)
	newProps := propNames(newFlat)

	// Removed properties — emit with inline type for leaves, recurse for objects.
	for _, name := range oldProps {
		if !hasProperty(newFlat, name) {
			childPath := joinPath(path, name)
			changes = append(changes, makeAddRemoveChange(oldFlat.Properties[name], childPath, "removed")...)
		}
	}

	// Added properties — emit with inline type for leaves, recurse for objects.
	for _, name := range newProps {
		if !hasProperty(oldFlat, name) {
			childPath := joinPath(path, name)
			changes = append(changes, makeAddRemoveChange(newFlat.Properties[name], childPath, "added")...)
		}
	}

	// Recurse into shared properties.
	for _, name := range newProps {
		if !hasProperty(oldFlat, name) {
			continue
		}
		childPath := joinPath(path, name)
		oldChild := oldFlat.Properties[name]
		newChild := newFlat.Properties[name]

		// Check for type changes.
		if desc := diffTypes(oldChild, newChild); desc != "" {
			changes = append(changes, Change{
				Path: childPath,
				Kind: "changed",
				Desc: desc,
			})

			// If type changed from scalar to object, show new fields as added.
			// If type changed from object to scalar, show old fields as removed.
			oldChildFlat := FlattenComposite(oldChild)
			newChildFlat := FlattenComposite(newChild)
			if !hasProps(oldChildFlat) && hasProps(newChildFlat) {
				changes = append(changes, diffSchemas(&Config{}, newChild, childPath)...)
			} else if hasProps(oldChildFlat) && !hasProps(newChildFlat) {
				changes = append(changes, diffSchemas(oldChild, &Config{}, childPath)...)
			}
			continue
		}

		// Check for enum changes.
		if desc := diffEnums(oldChild, newChild); desc != "" {
			changes = append(changes, Change{
				Path: childPath,
				Kind: "changed",
				Desc: desc,
			})
		}

		// Check for constraint changes.
		changes = append(changes, diffConstraints(oldChild, newChild, childPath)...)

		// Recurse into nested objects.
		changes = append(changes, diffSchemas(oldChild, newChild, childPath)...)
	}

	// Diff additionalProperties (dict value schemas).
	if oldFlat.AdditionalProperties != nil || newFlat.AdditionalProperties != nil {
		oldAP := oldFlat.AdditionalProperties
		newAP := newFlat.AdditionalProperties
		if oldAP == nil {
			oldAP = &Config{}
		}
		if newAP == nil {
			newAP = &Config{}
		}
		changes = append(changes, diffSchemas(oldAP, newAP, joinPath(path, "[*]"))...)
	}

	return changes
}

// makeAddRemoveChange creates a Change for an added or removed property.
// Leaves get inline type info. Objects recurse to show their children.
func makeAddRemoveChange(s *Config, path, kind string) []Change {
	flat := FlattenComposite(s)

	c := Change{
		Path: path,
		Kind: kind,
		Desc: shortDesc(flat.Description),
	}

	// Leaf — attach type info inline.
	if !hasProps(flat) && flat.AdditionalProperties == nil {
		c.TypeSegs, c.DefaultVal = LeafTypeSegs(s)
		return []Change{c}
	}

	// Object/map — add type, emit the node, then recurse into children.
	if flat.AdditionalProperties != nil {
		c.TypeSegs = []TypeSegment{{Text: "map", Literal: false}}
	} else {
		c.TypeSegs = []TypeSegment{{Text: "object", Literal: false}}
	}
	var changes []Change
	changes = append(changes, c)

	for _, name := range propNames(flat) {
		childPath := joinPath(path, name)
		changes = append(changes, makeAddRemoveChange(flat.Properties[name], childPath, kind)...)
	}

	// Recurse into additionalProperties (dict values).
	if flat.AdditionalProperties != nil {
		apFlat := FlattenComposite(flat.AdditionalProperties)
		if hasProps(apFlat) {
			changes = append(changes, makeAddRemoveChange(flat.AdditionalProperties, joinPath(path, "[*]"), kind)...)
		}
	}

	return changes
}

// filterChanges keeps only changes whose paths match any of the given prefixes.
func filterChanges(changes []Change, filters []string) []Change {
	var out []Change
	for _, c := range changes {
		if pathMatches(c.Path, filters) {
			out = append(out, c)
		}
	}
	return out
}

func diffTypes(old, newer *Config) string {
	oldFlat := FlattenComposite(old)
	newFlat := FlattenComposite(newer)

	oldType := effectiveType(oldFlat)
	newType := effectiveType(newFlat)

	if oldType != newType {
		return fmt.Sprintf("type: %s -> %s", oldType, newType)
	}
	return ""
}

// effectiveType returns the effective type string for a schema.
func effectiveType(s *Config) string {
	if s == nil {
		return "unknown"
	}
	if isScalarUnion(s) {
		variants := s.AnyOf
		if len(variants) == 0 {
			variants = s.OneOf
		}
		var types []string
		for _, v := range variants {
			t := v.Type
			if len(v.Enum) > 0 {
				t = fmt.Sprintf("enum(%s)", enumString(v.Enum))
			}
			types = append(types, t)
		}
		return strings.Join(types, "|")
	}
	if s.Format != "" {
		return s.Type + "(" + s.Format + ")"
	}
	if s.Type != "" {
		// Distinguish struct-like objects from map-like objects.
		if s.Type == "object" && s.AdditionalProperties != nil && !hasProps(s) {
			return "map"
		}
		return s.Type
	}
	if hasProps(s) {
		return "object"
	}
	if s.AdditionalProperties != nil {
		return "map"
	}
	return "unknown"
}

func diffEnums(old, newer *Config) string {
	oldVals := collectEnumValues(old)
	newVals := collectEnumValues(newer)

	if len(oldVals) == 0 && len(newVals) == 0 {
		return ""
	}

	oldSet := make(map[string]bool)
	for _, s := range oldVals {
		oldSet[s] = true
	}
	newSet := make(map[string]bool)
	for _, s := range newVals {
		newSet[s] = true
	}

	var added, removed []string
	for _, s := range newVals {
		if !oldSet[s] {
			added = append(added, s)
		}
	}
	for _, s := range oldVals {
		if !newSet[s] {
			removed = append(removed, s)
		}
	}

	if len(added) == 0 && len(removed) == 0 {
		return ""
	}

	var parts []string
	if len(added) > 0 {
		parts = append(parts, "enum added: "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "enum removed: "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, "; ")
}

// collectEnumValues gathers all non-nil enum values from a schema,
// handling both simple enums and scalar unions (anyOf/oneOf with enum variants).
func collectEnumValues(s *Config) []string {
	// Scalar unions spread enums across multiple variants.
	if isScalarUnion(s) {
		variants := s.AnyOf
		if len(variants) == 0 {
			variants = s.OneOf
		}
		var vals []string
		for _, v := range variants {
			for _, e := range v.Enum {
				if e != nil {
					vals = append(vals, fmtDefault(e))
				}
			}
			if len(v.Enum) == 0 && v.Type != "" {
				vals = append(vals, v.Type)
			}
		}
		return vals
	}
	// Simple enums — FlattenComposite now preserves Enum from the top-level schema.
	flat := FlattenComposite(s)
	var vals []string
	for _, e := range flat.Enum {
		if e != nil {
			vals = append(vals, fmtDefault(e))
		}
	}
	return vals
}

func diffConstraints(old, newer *Config, path string) []Change {
	oldFlat := FlattenComposite(old)
	newFlat := FlattenComposite(newer)

	var changes []Change

	if !floatPtrEqual(oldFlat.Minimum, newFlat.Minimum) {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("minimum: %s -> %s", fmtNumPtr(oldFlat.Minimum), fmtNumPtr(newFlat.Minimum)),
		})
	}
	if !floatPtrEqual(oldFlat.Maximum, newFlat.Maximum) {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("maximum: %s -> %s", fmtNumPtr(oldFlat.Maximum), fmtNumPtr(newFlat.Maximum)),
		})
	}
	if !intPtrEqual(oldFlat.MinLength, newFlat.MinLength) {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("minLength: %s -> %s", fmtIntPtr(oldFlat.MinLength), fmtIntPtr(newFlat.MinLength)),
		})
	}
	if !intPtrEqual(oldFlat.MaxLength, newFlat.MaxLength) {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("maxLength: %s -> %s", fmtIntPtr(oldFlat.MaxLength), fmtIntPtr(newFlat.MaxLength)),
		})
	}
	if oldFlat.Pattern != newFlat.Pattern {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("pattern: %q -> %q", oldFlat.Pattern, newFlat.Pattern),
		})
	}
	if oldFlat.Format != newFlat.Format {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("format: %s -> %s", oldFlat.Format, newFlat.Format),
		})
	}
	if fmt.Sprint(oldFlat.Default) != fmt.Sprint(newFlat.Default) {
		changes = append(changes, Change{
			Path: path, Kind: "changed",
			Desc: fmt.Sprintf("default: %s -> %s", fmtDefault(oldFlat.Default), fmtDefault(newFlat.Default)),
		})
	}

	return changes
}

// propNames returns sorted property names from a schema.
func propNames(s *Config) []string {
	if s == nil || s.Properties == nil {
		return nil
	}
	names := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// hasProperty returns true if the schema has a property with the given name.
func hasProperty(s *Config, name string) bool {
	if s == nil || s.Properties == nil {
		return false
	}
	_, ok := s.Properties[name]
	return ok
}

// joinPath joins two dotted path segments.
func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "." + name
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func fmtIntPtr(p *int) string {
	if p == nil {
		return "(none)"
	}
	return fmt.Sprintf("%d", *p)
}
