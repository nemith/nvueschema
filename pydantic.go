package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WritePydantic outputs the config schema as Python Pydantic v2 model classes.
func WritePydantic(w io.Writer, schema *Schema, info map[string]any) error {
	g := &pyGen{
		w:      w,
		models: make(map[string]bool),
	}

	fmt.Fprintln(w, `"""`)
	fmt.Fprintln(w, `Cumulus Linux NVUE Configuration Schema — Pydantic v2 Models`)
	if v, ok := info["version"].(string); ok {
		fmt.Fprintf(w, "Generated from NVUE OpenAPI spec version %s\n", v)
	}
	fmt.Fprintln(w, `"""`)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "from __future__ import annotations")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "from datetime import date, datetime, time")
	fmt.Fprintln(w, "from ipaddress import IPv4Address, IPv4Network, IPv6Address, IPv6Network")
	fmt.Fprintln(w, "from typing import Annotated, Any, Dict, List, Literal, Optional, Union")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "from pydantic import AnyUrl, BaseModel, Field, FilePath, SecretStr")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "# Validated network configuration types")
	fmt.Fprintln(w, `MacAddress = Annotated[str, Field(pattern=r"^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$")]`)
	fmt.Fprintln(w, `InterfaceName = Annotated[str, Field(pattern=r"^(swp|eth|bond|br|lo|vlan|peerlink|erspan|mgmt)[a-zA-Z0-9_./-]*$")]`)
	fmt.Fprintln(w, `VrfName = Annotated[str, Field(pattern=r"^[a-zA-Z][a-zA-Z0-9_-]{0,14}$")]`)
	fmt.Fprintln(w, `VlanRange = Annotated[str, Field(pattern=r"^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$")]`)
	fmt.Fprintln(w, `PortRange = Annotated[str, Field(pattern=r"^[0-9]+(-[0-9]+)?$")]`)
	fmt.Fprintln(w, `RouteDistinguisher = Annotated[str, Field(pattern=r"^(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)$")]`)
	fmt.Fprintln(w, `RouteTarget = Annotated[str, Field(pattern=r"^(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)$")]`)
	fmt.Fprintln(w, `ExtCommunity = Annotated[str, Field(pattern=r"^(rt|soo|bandwidth)\s+\S+$")]`)
	fmt.Fprintln(w, `BgpCommunity = Annotated[str, Field(pattern=r"^(\d+:\d+|no-export|no-advertise|local-AS|no-peer|blackhole|graceful-shutdown|accept-own|internet)$")]`)
	fmt.Fprintln(w, `EvpnRoute = Annotated[str, Field(pattern=r"^(macip|imet|prefix)$")]`)
	fmt.Fprintln(w, `BgpRegex = Annotated[str, Field(min_length=1)]`)
	fmt.Fprintln(w, `AsnRange = Annotated[str, Field(pattern=r"^(\d+|\d+-\d+)(,(\d+|\d+-\d+))*$")]`)
	fmt.Fprintln(w, `EsIdentifier = Annotated[str, Field(pattern=r"^([0-9A-Fa-f]{2}:){9}[0-9A-Fa-f]{2}$")]`)
	fmt.Fprintln(w, `SegmentIdentifier = Annotated[str, Field(pattern=r"^\d+$")]`)
	fmt.Fprintln(w, `Hostname = Annotated[str, Field(pattern=r"^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$")]`)
	fmt.Fprintln(w, `UserName = Annotated[str, Field(pattern=r"^[a-z_][a-z0-9_-]*[$]?$")]`)
	fmt.Fprintln(w, `SnmpOid = Annotated[str, Field(pattern=r"^\.?(\d+\.)*\d+$")]`)
	fmt.Fprintln(w)

	g.emitModel("NvueConfig", schema)

	return nil
}

type pyGen struct {
	w      io.Writer
	models map[string]bool
}

func (g *pyGen) emitModel(name string, s *Schema) {
	if g.models[name] {
		return
	}
	g.models[name] = true

	merged := flattenComposite(s)

	type propEntry struct {
		name   string
		schema *Schema
	}
	var props []propEntry
	if merged.Properties != nil {
		for k, v := range merged.Properties {
			props = append(props, propEntry{k, v})
		}
		sort.Slice(props, func(i, j int) bool { return props[i].name < props[j].name })
	}

	// Emit child models first (depth-first).
	for _, p := range props {
		childName := name + toPascal(p.name)
		if isScalarUnion(p.schema) {
			continue
		}
		flat := flattenComposite(p.schema)
		if hasProps(flat) {
			g.emitModel(childName, p.schema)
		}
		if flat.AdditionalProperties != nil {
			apFlat := flattenComposite(flat.AdditionalProperties)
			if hasProps(apFlat) {
				g.emitModel(childName+"Entry", flat.AdditionalProperties)
			}
		}
	}

	requiredSet := make(map[string]bool)
	for _, r := range merged.Required {
		requiredSet[r] = true
	}

	fmt.Fprintf(g.w, "\nclass %s(BaseModel):\n", name)
	docParts := []string{}
	if merged.Description != "" {
		docParts = append(docParts, sanitizeDocstring(merged.Description))
	}
	sourceRef := sourceRefFor(s)
	if sourceRef != "" {
		docParts = append(docParts, fmt.Sprintf("Path: %s", sourceRef))
	}
	if len(docParts) > 0 {
		fmt.Fprintln(g.w, "    \"\"\"")
		for _, part := range docParts {
			fmt.Fprintf(g.w, "    %s\n", part)
		}
		fmt.Fprintln(g.w, "    \"\"\"")
	}

	if len(props) == 0 {
		fmt.Fprintln(g.w, "    pass")
		fmt.Fprintln(g.w)
		return
	}

	for _, p := range props {
		flat := flattenComposite(p.schema)
		pyType := g.pyType(name+toPascal(p.name), p.schema)
		fieldName := toSnake(p.name)
		alias := ""
		if fieldName != p.name {
			alias = fmt.Sprintf(", alias=%q", p.name)
		}

		if requiredSet[p.name] {
			if flat.Description != "" {
				fmt.Fprintf(g.w, "    %s: %s = Field(...%s, description=%q)\n", fieldName, pyType, alias, flat.Description)
			} else {
				fmt.Fprintf(g.w, "    %s: %s = Field(...%s)\n", fieldName, pyType, alias)
			}
		} else {
			if flat.Description != "" {
				fmt.Fprintf(g.w, "    %s: Optional[%s] = Field(None%s, description=%q)\n", fieldName, pyType, alias, flat.Description)
			} else {
				fmt.Fprintf(g.w, "    %s: Optional[%s] = Field(None%s)\n", fieldName, pyType, alias)
			}
		}
	}
	fmt.Fprintln(g.w)
}

func (g *pyGen) pyType(contextName string, s *Schema) string {
	if s == nil {
		return "Any"
	}

	// Handle anyOf/oneOf with only scalar branches as Union.
	if isScalarUnion(s) {
		return scalarUnionType(s)
	}

	flat := flattenComposite(s)

	if len(flat.Enum) > 0 {
		return "str"
	}

	switch flat.Type {
	case "string":
		if t := formatToPyType(flat.Format); t != "" {
			return t
		}
		return "str"
	case "integer":
		return "int"
	case "number":
		return "float"
	case "boolean":
		return "bool"
	case "array":
		inner := g.pyType(contextName+"Item", flat.Items)
		return fmt.Sprintf("List[%s]", inner)
	case "object":
		if hasProps(flat) {
			return contextName
		}
		if flat.AdditionalProperties != nil {
			apFlat := flattenComposite(flat.AdditionalProperties)
			if hasProps(apFlat) {
				valType := g.pyType(contextName+"Entry", flat.AdditionalProperties)
				return fmt.Sprintf("Dict[str, %s]", valType)
			}
			return "Dict[str, Any]"
		}
		return "Dict[str, Any]"
	}

	if hasProps(flat) {
		return contextName
	}
	if flat.AdditionalProperties != nil {
		apFlat := flattenComposite(flat.AdditionalProperties)
		if hasProps(apFlat) {
			valType := g.pyType(contextName+"Entry", flat.AdditionalProperties)
			return fmt.Sprintf("Dict[str, %s]", valType)
		}
		return "Dict[str, Any]"
	}

	return "Any"
}

// sourceRefFor finds the best SourceRef from a schema or its composition branches.
func sourceRefFor(s *Schema) string {
	if s.SourceRef != "" {
		return s.SourceRef
	}
	for _, group := range [][]*Schema{s.AllOf, s.AnyOf, s.OneOf} {
		for _, sub := range group {
			if sub.SourceRef != "" {
				return sub.SourceRef
			}
		}
	}
	return ""
}

// isScalarUnion returns true if the schema is an anyOf/oneOf where every
// branch is a scalar type (no properties, no additionalProperties).
func isScalarUnion(s *Schema) bool {
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
func hasProps(s *Schema) bool {
	return len(s.Properties) > 0
}

// scalarUnionType builds a Union[...] type string from scalar anyOf/oneOf branches.
func scalarUnionType(s *Schema) string {
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}
	seen := make(map[string]bool)
	var types []string
	for _, v := range variants {
		t := scalarPyType(v)
		if !seen[t] {
			seen[t] = true
			types = append(types, t)
		}
	}
	if len(types) == 1 {
		return types[0]
	}
	return fmt.Sprintf("Union[%s]", strings.Join(types, ", "))
}

func scalarPyType(s *Schema) string {
	if len(s.Enum) > 0 {
		var vals []string
		for _, e := range s.Enum {
			switch v := e.(type) {
			case string:
				vals = append(vals, fmt.Sprintf("%q", v))
			}
			// skip null/nil enum values — handled by Optional wrapper
		}
		if len(vals) == 0 {
			return "None"
		}
		if len(vals) == 1 {
			return fmt.Sprintf("Literal[%s]", vals[0])
		}
		return fmt.Sprintf("Literal[%s]", strings.Join(vals, ", "))
	}
	if t := formatToPyType(s.Format); t != "" {
		return t
	}
	switch s.Type {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "number":
		return "float"
	case "boolean":
		return "bool"
	default:
		return "Any"
	}
}

// formatToPyType maps OpenAPI format strings to Python/Pydantic types.
func formatToPyType(format string) string {
	switch format {
	// IP addresses
	case "ipv4", "ipv4-unicast", "ipv4-multicast", "ipv4-netmask":
		return "IPv4Address"
	case "ipv6", "ipv6-netmask":
		return "IPv6Address"
	case "dns-server-ip-address":
		return "Union[IPv4Address, IPv6Address]"

	// IP networks
	case "ipv4-prefix", "ipv4-sub-prefix", "ipv4-multicast-prefix",
		"aggregate-ipv4-prefix":
		return "IPv4Network"
	case "ipv6-prefix", "ipv6-sub-prefix", "aggregate-ipv6-prefix":
		return "IPv6Network"

	// Numeric types misrepresented as string
	case "integer", "integer-id":
		return "int"
	case "float", "number":
		return "float"

	// Temporal
	case "date-time":
		return "datetime"
	case "clock-date":
		return "date"
	case "clock-time":
		return "time"

	// Constrained strings — use NewType aliases for clarity
	case "mac":
		return "MacAddress"
	case "interface-name", "swp-name", "bond-swp-name", "transceiver-name",
		"bridge-name":
		return "InterfaceName"
	case "vrf-name":
		return "VrfName"
	case "vlan-range":
		return "VlanRange"
	case "ip-port-range":
		return "PortRange"
	case "route-distinguisher":
		return "RouteDistinguisher"
	case "route-target", "route-target-any":
		return "RouteTarget"
	case "ext-community":
		return "ExtCommunity"
	case "community", "well-known-community", "large-community":
		return "BgpCommunity"
	case "evpn-route":
		return "EvpnRoute"
	case "bgp-regex":
		return "BgpRegex"
	case "asn-range":
		return "AsnRange"
	case "es-identifier":
		return "EsIdentifier"
	case "segment-identifier":
		return "SegmentIdentifier"
	case "secret-string", "key-string":
		return "SecretStr"
	case "idn-hostname", "domain-name":
		return "Hostname"
	case "user-name":
		return "UserName"
	case "generic-name", "item-name", "profile-name":
		return "str"
	case "file-name":
		return "FilePath"
	case "repo-url", "remote-url-fetch", "remote-url-upload":
		return "AnyUrl"
	case "repo-dist", "repo-pool":
		return "str"
	case "snmp-branch", "oid":
		return "SnmpOid"
	case "json-pointer":
		return "str"
	case "clock-id", "ptp-port-id":
		return "str"
	case "sequence-id":
		return "int"
	case "command", "command-path":
		return "str"
	case "interval", "rate-limit", "mss-format", "string":
		return "str"

	default:
		return ""
	}
}

func toPascal(s string) string {
	parts := strings.FieldsFunc(s, func(c rune) bool {
		return c == '-' || c == '_' || c == '.'
	})
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

var pyReserved = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true,
	"assert": true, "async": true, "await": true, "break": true, "class": true,
	"continue": true, "def": true, "del": true, "elif": true, "else": true,
	"except": true, "finally": true, "for": true, "from": true, "global": true,
	"if": true, "import": true, "in": true, "is": true, "lambda": true,
	"nonlocal": true, "not": true, "or": true, "pass": true, "raise": true,
	"return": true, "try": true, "while": true, "with": true, "yield": true,
	"type": true,
}

func toSnake(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	if pyReserved[s] {
		s += "_"
	}
	return s
}

func sanitizeDocstring(s string) string {
	return strings.ReplaceAll(s, `"""`, `\"\"\"`)
}

func newPydanticFormat() *Format {
	return &Format{
		Name:        "pydantic",
		Aliases:     []string{"py"},
		Description: "Python Pydantic v2 models",
		Write: func(w io.Writer, schema *Schema, info map[string]any) error {
			return WritePydantic(w, schema, info)
		},
	}
}
