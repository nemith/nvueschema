package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSONSchema outputs the config schema as a standalone JSON Schema draft-07 document.
func WriteJSONSchema(w io.Writer, schema *Schema, info map[string]any) error {
	title := "Cumulus Linux NVUE Configuration"
	if v, ok := info["title"].(string); ok {
		title = v
	}

	doc := schemaToJSONSchema(schema)
	doc["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	doc["title"] = title
	if v, ok := info["version"].(string); ok {
		doc["$comment"] = fmt.Sprintf("Generated from NVUE OpenAPI spec version %s", v)
	}

	// Add shared format definitions at the top level.
	doc["$defs"] = formatDefs()

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func schemaToJSONSchema(s *Schema) map[string]any {
	if s == nil {
		return map[string]any{}
	}

	// For scalar unions, emit directly without flattening.
	if isScalarUnion(s) {
		return scalarUnionToJSONSchema(s)
	}

	flat := flattenComposite(s)
	out := map[string]any{}

	// Source path as comment.
	ref := sourceRefFor(s)
	if ref != "" {
		out["$comment"] = fmt.Sprintf("Path: %s", ref)
	}

	if flat.Description != "" {
		out["description"] = flat.Description
	}

	// Determine type, applying format overrides.
	typ := flat.Type
	if typ == "" && hasProps(flat) {
		typ = "object"
	}
	if typ == "" && flat.AdditionalProperties != nil {
		typ = "object"
	}

	// Format-based type refinement.
	if flat.Format != "" {
		if jsDef := formatToJSONSchemaDef(flat.Format); jsDef != "" {
			out["$ref"] = "#/$defs/" + jsDef
			// Still include description/comment, but type comes from the $ref.
			if flat.Description != "" {
				out["description"] = flat.Description
			}
			return out
		}
	}

	if typ != "" {
		out["type"] = typ
	}

	if flat.Nullable {
		// JSON Schema draft-07 nullable via type array.
		if typ != "" {
			out["type"] = []string{typ, "null"}
		}
	}

	// Enum
	if len(flat.Enum) > 0 {
		out["enum"] = flat.Enum
	}

	// Numeric constraints.
	if flat.Minimum != nil {
		out["minimum"] = *flat.Minimum
	}
	if flat.Maximum != nil {
		out["maximum"] = *flat.Maximum
	}

	// String constraints.
	if flat.MinLength != nil {
		out["minLength"] = *flat.MinLength
	}
	if flat.MaxLength != nil {
		out["maxLength"] = *flat.MaxLength
	}
	if flat.Pattern != "" {
		out["pattern"] = flat.Pattern
	}
	if flat.Format != "" {
		out["format"] = flat.Format
	}

	// Default
	if flat.Default != nil {
		out["default"] = flat.Default
	}

	// Required
	if len(flat.Required) > 0 {
		out["required"] = flat.Required
	}

	// Properties
	if hasProps(flat) {
		props := map[string]any{}
		for k, v := range flat.Properties {
			props[k] = schemaToJSONSchema(v)
		}
		out["properties"] = props
		out["additionalProperties"] = false
	}

	// additionalProperties (dict-like)
	if flat.AdditionalProperties != nil && !hasProps(flat) {
		out["additionalProperties"] = schemaToJSONSchema(flat.AdditionalProperties)
	}

	// Items (array)
	if flat.Items != nil {
		out["items"] = schemaToJSONSchema(flat.Items)
	}

	return out
}

func scalarUnionToJSONSchema(s *Schema) map[string]any {
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}

	out := map[string]any{}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if s.Default != nil {
		out["default"] = s.Default
	}

	var schemas []map[string]any
	for _, v := range variants {
		branch := map[string]any{}
		if v.Type != "" {
			if v.Nullable {
				branch["type"] = []string{v.Type, "null"}
			} else {
				branch["type"] = v.Type
			}
		}
		if len(v.Enum) > 0 {
			branch["enum"] = v.Enum
		}
		schemas = append(schemas, branch)
	}

	if len(schemas) == 1 {
		// Unwrap single-variant.
		for k, v := range schemas[0] {
			out[k] = v
		}
	} else {
		out["anyOf"] = schemas
	}

	if s.Nullable {
		// Ensure null is allowed.
		if _, hasAnyOf := out["anyOf"]; !hasAnyOf {
			if t, ok := out["type"].(string); ok {
				out["type"] = []string{t, "null"}
			}
		}
	}

	return out
}

// formatToJSONSchemaDef returns the $defs key for a format, or "" if not mapped.
func formatToJSONSchemaDef(format string) string {
	switch format {
	case "ipv4", "ipv4-unicast", "ipv4-multicast", "ipv4-netmask":
		return "ipv4-address"
	case "ipv6", "ipv6-netmask":
		return "ipv6-address"
	case "ipv4-prefix", "ipv4-sub-prefix", "ipv4-multicast-prefix",
		"aggregate-ipv4-prefix":
		return "ipv4-prefix"
	case "ipv6-prefix", "ipv6-sub-prefix", "aggregate-ipv6-prefix":
		return "ipv6-prefix"
	case "dns-server-ip-address":
		return "ip-address"
	case "mac":
		return "mac-address"
	case "interface-name", "swp-name", "bond-swp-name", "transceiver-name",
		"bridge-name":
		return "interface-name"
	case "vrf-name":
		return "vrf-name"
	case "vlan-range":
		return "vlan-range"
	case "ip-port-range":
		return "port-range"
	case "route-distinguisher":
		return "route-distinguisher"
	case "route-target", "route-target-any":
		return "route-target"
	case "ext-community":
		return "ext-community"
	case "community", "well-known-community", "large-community":
		return "bgp-community"
	case "evpn-route":
		return "evpn-route"
	case "asn-range":
		return "asn-range"
	case "es-identifier":
		return "es-identifier"
	case "segment-identifier":
		return "segment-identifier"
	case "bgp-regex":
		return "bgp-regex"
	case "idn-hostname", "domain-name":
		return "hostname"
	case "user-name":
		return "user-name"
	case "snmp-branch", "oid":
		return "snmp-oid"
	case "secret-string", "key-string":
		return "secret-string"
	default:
		return ""
	}
}

// formatDefs returns the $defs block with pattern-validated format types.
func formatDefs() map[string]any {
	defs := map[string]any{
		"ipv4-address": map[string]any{
			"type":   "string",
			"format": "ipv4",
		},
		"ipv6-address": map[string]any{
			"type":   "string",
			"format": "ipv6",
		},
		"ipv4-prefix": map[string]any{
			"type":    "string",
			"pattern": `^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/\d{1,2}$`,
		},
		"ipv6-prefix": map[string]any{
			"type":    "string",
			"pattern": `^[0-9a-fA-F:]+/\d{1,3}$`,
		},
		"ip-address": map[string]any{
			"anyOf": []map[string]any{
				{"type": "string", "format": "ipv4"},
				{"type": "string", "format": "ipv6"},
			},
		},
		"mac-address": map[string]any{
			"type":    "string",
			"pattern": `^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`,
		},
		"interface-name": map[string]any{
			"type":    "string",
			"pattern": `^(swp|eth|bond|br|lo|vlan|peerlink|erspan|mgmt)[a-zA-Z0-9_./-]*$`,
		},
		"vrf-name": map[string]any{
			"type":    "string",
			"pattern": `^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`,
		},
		"vlan-range": map[string]any{
			"type":    "string",
			"pattern": `^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$`,
		},
		"port-range": map[string]any{
			"type":    "string",
			"pattern": `^[0-9]+(-[0-9]+)?$`,
		},
		"route-distinguisher": map[string]any{
			"type":    "string",
			"pattern": `^(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)$`,
		},
		"route-target": map[string]any{
			"type":    "string",
			"pattern": `^(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)$`,
		},
		"ext-community": map[string]any{
			"type":    "string",
			"pattern": `^(rt|soo|bandwidth)\s+\S+$`,
		},
		"bgp-community": map[string]any{
			"type":    "string",
			"pattern": `^(\d+:\d+|no-export|no-advertise|local-AS|no-peer|blackhole|graceful-shutdown|accept-own|internet)$`,
		},
		"evpn-route": map[string]any{
			"type": "string",
			"enum": []string{"macip", "imet", "prefix"},
		},
		"bgp-regex": map[string]any{
			"type":      "string",
			"minLength": 1,
		},
		"asn-range": map[string]any{
			"type":    "string",
			"pattern": `^(\d+|\d+-\d+)(,(\d+|\d+-\d+))*$`,
		},
		"es-identifier": map[string]any{
			"type":    "string",
			"pattern": `^([0-9A-Fa-f]{2}:){9}[0-9A-Fa-f]{2}$`,
		},
		"segment-identifier": map[string]any{
			"type":    "string",
			"pattern": `^\d+$`,
		},
		"hostname": map[string]any{
			"type":    "string",
			"format":  "hostname",
			"pattern": `^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`,
		},
		"user-name": map[string]any{
			"type":    "string",
			"pattern": `^[a-z_][a-z0-9_-]*[$]?$`,
		},
		"snmp-oid": map[string]any{
			"type":    "string",
			"pattern": `^\.?(\d+\.)*\d+$`,
		},
		"secret-string": map[string]any{
			"type":      "string",
			"maxLength": 64,
		},
	}
	return defs
}

func newJSONSchemaFormat() *Format {
	return &Format{
		Name:        "jsonschema",
		Aliases:     []string{"js"},
		Description: "JSON Schema 2020-12",
		Write: func(w io.Writer, schema *Schema, info map[string]any) error {
			return WriteJSONSchema(w, schema, info)
		},
	}
}
