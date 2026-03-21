package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteYANG outputs the config schema as a YANG module.
func WriteYANG(w io.Writer, schema *Schema, info map[string]any) error {
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

	merged := flattenComposite(schema)
	emitYANGContainer(w, "nvue-config", schema, merged, 1)

	fmt.Fprintln(w, "}")
	return nil
}

func emitYANGTypedefs(w io.Writer) {
	typedefs := []struct {
		name, base, pattern, desc string
	}{
		{"mac-address", "string", `[0-9A-Fa-f]{2}(:[0-9A-Fa-f]{2}){5}`, "IEEE 802 MAC address"},
		{"interface-name", "string", `(swp|eth|bond|br|lo|vlan|peerlink|erspan|mgmt)[a-zA-Z0-9_./-]*`, "Network interface name"},
		{"vrf-name", "string", `[a-zA-Z][a-zA-Z0-9_-]{0,14}`, "VRF name"},
		{"vlan-range", "string", `[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*`, "VLAN ID or range"},
		{"port-range", "string", `[0-9]+(-[0-9]+)?`, "TCP/UDP port or range"},
		{"route-distinguisher", "string", `(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)`, "BGP route distinguisher"},
		{"route-target", "string", `(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)`, "BGP route target"},
		{"ext-community", "string", "", "BGP extended community"},
		{"bgp-community", "string", `(\d+:\d+|no-export|no-advertise|local-AS|no-peer|blackhole|graceful-shutdown|accept-own|internet)`, "BGP community"},
		{"evpn-route", "string", `(macip|imet|prefix)`, "EVPN route type"},
		{"asn-range", "string", `(\d+|\d+-\d+)(,(\d+|\d+-\d+))*`, "ASN or range"},
		{"es-identifier", "string", `([0-9A-Fa-f]{2}:){9}[0-9A-Fa-f]{2}`, "Ethernet segment identifier"},
		{"segment-identifier", "string", `\d+`, "MPLS segment identifier"},
		{"hostname", "string", `[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*`, "DNS hostname"},
		{"user-name", "string", `[a-z_][a-z0-9_-]*[$]?`, "POSIX username"},
		{"snmp-oid", "string", `\.?(\d+\.)*\d+`, "SNMP object identifier"},
	}

	for _, td := range typedefs {
		fmt.Fprintf(w, "  typedef %s {\n", td.name)
		fmt.Fprintf(w, "    type %s", td.base)
		if td.pattern != "" {
			fmt.Fprintf(w, " {\n")
			fmt.Fprintf(w, "      pattern '%s';\n", td.pattern)
			fmt.Fprintf(w, "    }")
		}
		fmt.Fprintln(w, ";")
		fmt.Fprintf(w, "    description\n      %q;\n", td.desc)
		fmt.Fprintln(w, "  }")
		fmt.Fprintln(w)
	}
}

func emitYANGContainer(w io.Writer, name string, orig *Schema, flat *Schema, depth int) {
	indent := strings.Repeat("  ", depth)

	type prop struct {
		name   string
		schema *Schema
	}
	var props []prop
	if flat.Properties != nil {
		for k, v := range flat.Properties {
			props = append(props, prop{k, v})
		}
		sort.Slice(props, func(i, j int) bool { return props[i].name < props[j].name })
	}

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

func emitYANGNode(w io.Writer, name string, s *Schema, depth int) {
	// Scalar union (anyOf/oneOf of primitives) -> YANG union leaf.
	if isScalarUnion(s) {
		emitYANGUnionLeaf(w, name, s, depth)
		return
	}

	flat := flattenComposite(s)

	// Dict with complex values -> list.
	if flat.AdditionalProperties != nil {
		apFlat := flattenComposite(flat.AdditionalProperties)
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

func emitYANGList(w io.Writer, name string, orig *Schema, flat *Schema, depth int) {
	indent := strings.Repeat("  ", depth)

	type prop struct {
		name   string
		schema *Schema
	}
	var props []prop
	if flat.Properties != nil {
		for k, v := range flat.Properties {
			props = append(props, prop{k, v})
		}
		sort.Slice(props, func(i, j int) bool { return props[i].name < props[j].name })
	}

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

func emitYANGLeaf(w io.Writer, name string, s *Schema, depth int) {
	indent := strings.Repeat("  ", depth)
	yangType := toYANGType(s)

	fmt.Fprintf(w, "%sleaf %s {\n", indent, yangSafe(name))
	emitYANGTypeBlock(w, yangType, s, indent)
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	if s.Default != nil {
		fmt.Fprintf(w, "%s  default %q;\n", indent, fmt.Sprint(s.Default))
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

func emitYANGLeafList(w io.Writer, name string, s *Schema, depth int) {
	indent := strings.Repeat("  ", depth)
	itemFlat := flattenComposite(s.Items)
	yangType := toYANGType(itemFlat)

	fmt.Fprintf(w, "%sleaf-list %s {\n", indent, yangSafe(name))
	emitYANGTypeBlock(w, yangType, itemFlat, indent)
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

func emitYANGUnionLeaf(w io.Writer, name string, s *Schema, depth int) {
	indent := strings.Repeat("  ", depth)
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}

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
		fmt.Fprintf(w, "%s  default %q;\n", indent, fmt.Sprint(s.Default))
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// emitYANGTypeBlock writes the type statement, including pattern/range restrictions and enums.
func emitYANGTypeBlock(w io.Writer, yangType string, s *Schema, indent string) {
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
			lo = fmt.Sprintf("%v", *s.Minimum)
		}
		if s.Maximum != nil {
			hi = fmt.Sprintf("%v", *s.Maximum)
		}
		fmt.Fprintf(w, "%s    range \"%s..%s\";\n", indent, lo, hi)
	}
	fmt.Fprintf(w, "%s  }\n", indent)
}

func toYANGType(s *Schema) string {
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

func formatToYANGType(format string) string {
	switch format {
	case "ipv4", "ipv4-unicast", "ipv4-multicast", "ipv4-netmask":
		return "inet:ipv4-address" // would need inet import; using typedef instead
	case "ipv6", "ipv6-netmask":
		return "inet:ipv6-address"
	case "ipv4-prefix", "ipv4-sub-prefix", "ipv4-multicast-prefix",
		"aggregate-ipv4-prefix":
		return "inet:ipv4-prefix"
	case "ipv6-prefix", "ipv6-sub-prefix", "aggregate-ipv6-prefix":
		return "inet:ipv6-prefix"
	case "dns-server-ip-address":
		return "inet:ip-address"
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
		return "string"
	case "idn-hostname", "domain-name":
		return "hostname"
	case "user-name":
		return "user-name"
	case "snmp-branch", "oid":
		return "snmp-oid"
	case "secret-string", "key-string":
		return "string"
	case "integer", "integer-id", "sequence-id":
		return "int64"
	case "float", "number":
		return "decimal64"
	case "date-time":
		return "yang:date-and-time"
	default:
		return ""
	}
}

func yangSafe(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func newYANGFormat() *Format {
	return &Format{
		Name:        "yang",
		Description: "YANG module",
		Write: func(w io.Writer, schema *Schema, info map[string]any) error {
			return WriteYANG(w, schema, info)
		},
	}
}
