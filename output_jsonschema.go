package nvueschema

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
)

// WriteJSONSchema outputs the config schema as a standalone JSON Schema draft-07 document.
func WriteJSONSchema(w io.Writer, schema *Config, info map[string]any) error {
	title := "Cumulus Linux NVUE Configuration"
	if v, ok := info["title"].(string); ok {
		title = v
	}

	doc := schema.JSONSchemaDoc()
	doc["title"] = title
	if v, ok := info["version"].(string); ok {
		doc["$comment"] = fmt.Sprintf("Generated from NVUE OpenAPI spec version %s", v)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// JSONSchemaDoc builds a complete JSON Schema 2020-12 document,
// including $schema and $defs for format types.
func (s *Config) JSONSchemaDoc() map[string]any {
	doc := s.ToJSONSchema()
	doc["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	doc["$defs"] = formatDefs()
	return doc
}

// ToJSONSchema converts a Schema to a JSON Schema map.
func (s *Config) ToJSONSchema() map[string]any {
	if s == nil {
		return map[string]any{}
	}

	// For scalar unions, emit directly without flattening.
	if isScalarUnion(s) {
		return scalarUnionToJSONSchema(s)
	}

	flat := FlattenComposite(s)
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
			props[k] = v.ToJSONSchema()
		}
		out["properties"] = props
		out["additionalProperties"] = false
	}

	// additionalProperties (dict-like)
	if flat.AdditionalProperties != nil && !hasProps(flat) {
		out["additionalProperties"] = flat.AdditionalProperties.ToJSONSchema()
	}

	// Items (array)
	if flat.Items != nil {
		out["items"] = flat.Items.ToJSONSchema()
	}

	return out
}

func scalarUnionToJSONSchema(s *Config) map[string]any {
	variants := scalarUnionVariants(s)

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
		maps.Copy(out, schemas[0])
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
	k := formatKeyFor(format)
	if k == 0 {
		return ""
	}
	if t, ok := jsonSchemaDefTypes[k]; ok {
		return t
	}
	return ""
}

var jsonSchemaDefTypes = map[formatKey]string{
	fmtIPv4Addr:          "ipv4-address",
	fmtIPv6Addr:          "ipv6-address",
	fmtIPAddr:            "ip-address",
	fmtIPv4Prefix:        "ipv4-prefix",
	fmtIPv6Prefix:        "ipv6-prefix",
	fmtMAC:               "mac-address",
	fmtInterfaceName:     "interface-name",
	fmtVrfName:           "vrf-name",
	fmtVlanRange:         "vlan-range",
	fmtPortRange:         "port-range",
	fmtRouteDistinguisher: "route-distinguisher",
	fmtRouteTarget:       "route-target",
	fmtExtCommunity:      "ext-community",
	fmtBgpCommunity:      "bgp-community",
	fmtEvpnRoute:         "evpn-route",
	fmtAsnRange:          "asn-range",
	fmtEsIdentifier:      "es-identifier",
	fmtSegmentIdentifier: "segment-identifier",
	fmtBgpRegex:          "bgp-regex",
	fmtHostname:          "hostname",
	fmtUserName:          "user-name",
	fmtSnmpOid:           "snmp-oid",
	fmtSecretString:      "secret-string",
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
