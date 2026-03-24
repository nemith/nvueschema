package nvueschema

import (
	"fmt"
	"io"
	"strings"
)

// WriteYANG outputs the config schema as a YANG module.
func WriteYANG(w io.Writer, schema *Config, info map[string]any) error {
	version := "unknown"
	if v, ok := info["version"].(string); ok {
		version = v
	}

	fmt.Fprintln(w, "module cumulus-nvue {")
	fmt.Fprintln(w, `  namespace "urn:nvidia:cumulus:nvue";`)
	fmt.Fprintln(w, "  prefix nvue;")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  import ietf-inet-types {")
	fmt.Fprintln(w, "    prefix inet;")
	fmt.Fprintln(w, "  }")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  import ietf-yang-types {")
	fmt.Fprintln(w, "    prefix yang;")
	fmt.Fprintln(w, "  }")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  description")
	fmt.Fprintf(w, "    \"Cumulus Linux NVUE configuration schema.\n")
	fmt.Fprintf(w, "     Generated from OpenAPI spec version %s.\";\n", version)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  revision %s {\n", "2025-01-01")
	fmt.Fprintln(w, `    description "Auto-generated from NVUE OpenAPI spec.";`)
	fmt.Fprintln(w, "  }")
	fmt.Fprintln(w)

	// Emit typedefs for format-based types.
	emitYANGTypedefs(w)

	merged := FlattenComposite(schema)
	emitYANGContainer(w, "nvue-config", schema, merged, 1)

	fmt.Fprintln(w, "}")
	return nil
}

func emitYANGTypedefs(w io.Writer) {
	for _, td := range typedefs {
		yangName := yangTypedefName(td.key)
		if yangName == "" {
			continue
		}
		// Strip anchors for YANG patterns.
		pattern := strings.TrimPrefix(td.pattern, "^")
		pattern = strings.TrimSuffix(pattern, "$")

		fmt.Fprintf(w, "  typedef %s {\n", yangName)
		fmt.Fprintf(w, "    type string")
		if pattern != "" {
			fmt.Fprintf(w, " {\n")
			fmt.Fprintf(w, "      pattern '%s';\n", pattern)
			fmt.Fprintf(w, "    }")
		}
		fmt.Fprintln(w, ";")
		fmt.Fprintf(w, "    description\n      %q;\n", td.desc)
		fmt.Fprintln(w, "  }")
		fmt.Fprintln(w)
	}
}

// yangTypedefName maps a formatKey to its YANG typedef name.
func yangTypedefName(k formatKey) string {
	switch k {
	case fmtMAC:
		return "mac-address"
	case fmtInterfaceName:
		return "interface-name"
	case fmtVrfName:
		return "vrf-name"
	case fmtVlanRange:
		return "vlan-range"
	case fmtPortRange:
		return "port-range"
	case fmtRouteDistinguisher:
		return "route-distinguisher"
	case fmtRouteTarget:
		return "route-target"
	case fmtExtCommunity:
		return "ext-community"
	case fmtBgpCommunity:
		return "bgp-community"
	case fmtEvpnRoute:
		return "evpn-route"
	case fmtAsnRange:
		return "asn-range"
	case fmtEsIdentifier:
		return "es-identifier"
	case fmtSegmentIdentifier:
		return "segment-identifier"
	case fmtHostname:
		return "hostname"
	case fmtUserName:
		return "user-name"
	case fmtSnmpOid:
		return "snmp-oid"
	default:
		return ""
	}
}

func emitYANGContainer(w io.Writer, name string, orig *Config, flat *Config, depth int) {
	indent := strings.Repeat("  ", depth)
	props := sortedProperties(flat)

	// Skip empty containers.
	if len(props) == 0 {
		return
	}

	ref := sourceRefFor(orig)
	if ref != "" {
		fmt.Fprintf(w, "%s// Path: %s\n", indent, ref)
	}
	fmt.Fprintf(w, "%scontainer %s {\n", indent, yangSafe(name))
	if flat.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, flat.Description)
	}

	for _, p := range props {
		emitYANGNode(w, p.name, p.schema, depth+1)
	}

	fmt.Fprintf(w, "%s}\n", indent)
}

func emitYANGNode(w io.Writer, name string, s *Config, depth int) {
	// Scalar union (anyOf/oneOf of primitives) -> YANG union leaf.
	if isScalarUnion(s) {
		emitYANGUnionLeaf(w, name, s, depth)
		return
	}

	flat := FlattenComposite(s)

	// Dict with complex values -> list.
	if flat.AdditionalProperties != nil {
		apFlat := FlattenComposite(flat.AdditionalProperties)
		if hasProps(apFlat) {
			emitYANGList(w, name, flat.AdditionalProperties, apFlat, depth)
			return
		}
	}

	// Has sub-properties -> container.
	if hasProps(flat) {
		emitYANGContainer(w, name, s, flat, depth)
		return
	}

	// Array -> leaf-list.
	if flat.Type == "array" && flat.Items != nil {
		emitYANGLeafList(w, name, flat, depth)
		return
	}

	// Scalar -> leaf.
	emitYANGLeaf(w, name, flat, depth)
}

func emitYANGList(w io.Writer, name string, orig *Config, flat *Config, depth int) {
	indent := strings.Repeat("  ", depth)
	props := sortedProperties(flat)

	ref := sourceRefFor(orig)
	if ref != "" {
		fmt.Fprintf(w, "%s// Path: %s\n", indent, ref)
	}
	fmt.Fprintf(w, "%slist %s {\n", indent, yangSafe(name))
	if flat.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, flat.Description)
	}
	fmt.Fprintf(w, "%s  key \"id\";\n", indent)
	fmt.Fprintf(w, "%s  leaf id {\n", indent)
	fmt.Fprintf(w, "%s    type string;\n", indent)
	fmt.Fprintf(w, "%s  }\n", indent)

	for _, p := range props {
		emitYANGNode(w, p.name, p.schema, depth+1)
	}

	fmt.Fprintf(w, "%s}\n", indent)
}

func emitYANGLeaf(w io.Writer, name string, s *Config, depth int) {
	indent := strings.Repeat("  ", depth)
	yangType := toYANGType(s)

	fmt.Fprintf(w, "%sleaf %s {\n", indent, yangSafe(name))
	emitYANGTypeBlock(w, yangType, s, indent)
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	if s.Default != nil {
		fmt.Fprintf(w, "%s  default %q;\n", indent, fmtDefault(s.Default))
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

func emitYANGLeafList(w io.Writer, name string, s *Config, depth int) {
	indent := strings.Repeat("  ", depth)
	itemFlat := FlattenComposite(s.Items)
	yangType := toYANGType(itemFlat)

	fmt.Fprintf(w, "%sleaf-list %s {\n", indent, yangSafe(name))
	emitYANGTypeBlock(w, yangType, itemFlat, indent)
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

func emitYANGUnionLeaf(w io.Writer, name string, s *Config, depth int) {
	indent := strings.Repeat("  ", depth)
	variants := scalarUnionVariants(s)

	fmt.Fprintf(w, "%sleaf %s {\n", indent, yangSafe(name))
	if len(variants) == 1 {
		// Single variant — no union wrapper needed.
		v := variants[0]
		if len(v.Enum) > 0 {
			emitYANGTypeBlock(w, "enumeration", v, indent)
		} else {
			emitYANGTypeBlock(w, toYANGType(v), v, indent)
		}
	} else {
		fmt.Fprintf(w, "%s  type union {\n", indent)
		for _, v := range variants {
			yangType := toYANGType(v)
			if len(v.Enum) > 0 {
				fmt.Fprintf(w, "%s    type enumeration {\n", indent)
				for _, e := range v.Enum {
					if str, ok := e.(string); ok {
						fmt.Fprintf(w, "%s      enum %q;\n", indent, str)
					}
				}
				fmt.Fprintf(w, "%s    }\n", indent)
			} else {
				fmt.Fprintf(w, "%s    type %s;\n", indent, yangType)
			}
		}
		fmt.Fprintf(w, "%s  }\n", indent)
	}
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	if s.Default != nil {
		fmt.Fprintf(w, "%s  default %q;\n", indent, fmtDefault(s.Default))
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitYANGTypeBlock writes the type statement, including pattern/range restrictions and enums.
func emitYANGTypeBlock(w io.Writer, yangType string, s *Config, indent string) {
	hasRestrictions := s.Pattern != "" || s.Minimum != nil || s.Maximum != nil ||
		s.MinLength != nil || s.MaxLength != nil

	if yangType == "enumeration" && len(s.Enum) > 0 {
		fmt.Fprintf(w, "%s  type enumeration {\n", indent)
		for _, e := range s.Enum {
			if str, ok := e.(string); ok {
				fmt.Fprintf(w, "%s    enum %q;\n", indent, str)
			}
		}
		fmt.Fprintf(w, "%s  }\n", indent)
		return
	}

	if !hasRestrictions {
		fmt.Fprintf(w, "%s  type %s;\n", indent, yangType)
		return
	}

	fmt.Fprintf(w, "%s  type %s {\n", indent, yangType)
	if s.Pattern != "" {
		fmt.Fprintf(w, "%s    pattern '%s';\n", indent, s.Pattern)
	}
	if s.MinLength != nil || s.MaxLength != nil {
		lo := 0
		hi := "max"
		if s.MinLength != nil {
			lo = *s.MinLength
		}
		hiStr := hi
		if s.MaxLength != nil {
			hiStr = fmt.Sprintf("%d", *s.MaxLength)
		}
		fmt.Fprintf(w, "%s    length \"%d..%s\";\n", indent, lo, hiStr)
	}
	if (yangType == "int64" || yangType == "decimal64") && (s.Minimum != nil || s.Maximum != nil) {
		lo := "min"
		hi := "max"
		if s.Minimum != nil {
			lo = fmtNum(*s.Minimum)
		}
		if s.Maximum != nil {
			hi = fmtNum(*s.Maximum)
		}
		fmt.Fprintf(w, "%s    range \"%s..%s\";\n", indent, lo, hi)
	}
	fmt.Fprintf(w, "%s  }\n", indent)
}

func toYANGType(s *Config) string {
	// Check format first.
	if t := formatToYANGType(s.Format); t != "" {
		return t
	}
	if len(s.Enum) > 0 {
		return "enumeration"
	}
	switch s.Type {
	case "string":
		return "string"
	case "integer":
		return "int64"
	case "number":
		return "decimal64"
	case "boolean":
		return "boolean"
	case "array":
		return "string"
	default:
		return "string"
	}
}

// formatToYANGType maps OpenAPI format strings to YANG types via the registry.
func formatToYANGType(format string) string {
	k := formatKeyFor(format)
	if k == 0 {
		return ""
	}
	if t, ok := yangFormatTypes[k]; ok {
		return t
	}
	return ""
}

var yangFormatTypes = map[formatKey]string{
	fmtIPv4Addr:          "inet:ipv4-address",
	fmtIPv6Addr:          "inet:ipv6-address",
	fmtIPAddr:            "inet:ip-address",
	fmtIPv4Prefix:        "inet:ipv4-prefix",
	fmtIPv6Prefix:        "inet:ipv6-prefix",
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
	fmtBgpRegex:          "string",
	fmtHostname:          "hostname",
	fmtUserName:          "user-name",
	fmtSnmpOid:           "snmp-oid",
	fmtSecretString:      "string",
	fmtInteger:           "int64",
	fmtSequenceID:        "int64",
	fmtFloat:             "decimal64",
	fmtDateTime:          "yang:date-and-time",
}

func yangSafe(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
