package nvueschema

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"nemith.io/nvueschema/yangregexp"
)

// WriteYANG outputs the config schema as a YANG module.
func WriteYANG(w io.Writer, schema *Config, info map[string]any) error {
	checked := &errorWriter{dst: w}
	w = checked
	version := "unknown"
	if v, ok := info["version"].(string); ok {
		version = v
	}

	fmt.Fprintln(w, "module cumulus-nvue {")
	fmt.Fprintln(w, "  yang-version 1.1;")
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
	if err := emitYANGTypedefs(w); err != nil {
		return err
	}

	merged := FlattenComposite(schema)
	if err := emitYANGContainer(w, "nvue-config", schema, merged, 1); err != nil {
		return err
	}

	fmt.Fprintln(w, "}")
	return checked.err
}

func emitYANGTypedefs(w io.Writer) error {
	for _, td := range typedefs {
		yangName := yangTypedefName(td.key)
		if yangName == "" {
			continue
		}
		fmt.Fprintf(w, "  typedef %s {\n", yangName)
		fmt.Fprintf(w, "    type string")
		if td.pattern != "" {
			fmt.Fprintf(w, " {\n")
			if err := emitYANGPatterns(w, td.pattern, "      "); err != nil {
				return fmt.Errorf("typedef %s: %w", yangName, err)
			}
			fmt.Fprintln(w, "    }")
		} else {
			fmt.Fprintln(w, ";")
		}
		fmt.Fprintf(w, "    description\n      %q;\n", td.desc)
		fmt.Fprintln(w, "  }")
		fmt.Fprintln(w)
	}
	return nil
}

func emitYANGPatterns(w io.Writer, source, indent string) error {
	patterns, err := yangregexp.Convert(source)
	if err != nil {
		return fmt.Errorf("pattern %q: %w", source, err)
	}
	for _, pattern := range patterns {
		fmt.Fprintf(w, "%spattern %s", indent, yangString(pattern.Expression))
		if pattern.InvertMatch {
			fmt.Fprintln(w, " {")
			fmt.Fprintf(w, "%s  modifier invert-match;\n%s}\n", indent, indent)
		} else {
			fmt.Fprintln(w, ";")
		}
	}
	return nil
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

func emitYANGContainer(w io.Writer, name string, orig *Config, flat *Config, depth int) error {
	indent := strings.Repeat("  ", depth)
	props := sortedProperties(flat)

	// Skip empty containers.
	if len(props) == 0 {
		return nil
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
		if err := emitYANGNode(w, p.name, p.schema, depth+1); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func emitYANGNode(w io.Writer, name string, s *Config, depth int) error {
	// YANG represents an unset nullable value by absence of the leaf.
	if yangNullOnly(s) {
		return nil
	}
	// Scalar union (anyOf/oneOf of primitives) -> YANG union leaf.
	if variants := yangScalarUnionVariants(s); len(variants) > 0 {
		return emitYANGUnionLeaf(w, name, s, variants, depth)
	}

	flat := FlattenComposite(s)

	// Dict with complex values -> list.
	if flat.AdditionalProperties != nil {
		apFlat := FlattenComposite(flat.AdditionalProperties)
		if hasProps(apFlat) {
			return emitYANGList(w, name, flat.AdditionalProperties, apFlat, depth)
		}
	}

	// Has sub-properties -> container.
	if hasProps(flat) {
		return emitYANGContainer(w, name, s, flat, depth)
	}

	// Array -> leaf-list.
	if flat.Type == "array" && flat.Items != nil {
		return emitYANGLeafList(w, name, flat, depth)
	}

	// Scalar -> leaf.
	return emitYANGLeaf(w, name, flat, depth)
}

func emitYANGList(w io.Writer, name string, orig *Config, flat *Config, depth int) error {
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
		if err := emitYANGNode(w, p.name, p.schema, depth+1); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func emitYANGLeaf(w io.Writer, name string, s *Config, depth int) error {
	indent := strings.Repeat("  ", depth)
	yangType := toYANGType(s)

	fmt.Fprintf(w, "%sleaf %s {\n", indent, yangSafe(name))
	if err := emitYANGTypeBlock(w, yangType, s, indent); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	if s.Default != nil {
		fmt.Fprintf(w, "%s  default %s;\n", indent, yangDefault(s.Default))
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func emitYANGLeafList(w io.Writer, name string, s *Config, depth int) error {
	indent := strings.Repeat("  ", depth)
	itemFlat := FlattenComposite(s.Items)
	yangType := toYANGType(itemFlat)

	fmt.Fprintf(w, "%sleaf-list %s {\n", indent, yangSafe(name))
	if err := emitYANGTypeBlock(w, yangType, itemFlat, indent); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

// yangScalarUnionVariants follows unions through single-reference allOf
// wrappers. Only unwrap metadata and compatible type annotations: additional
// constraints on an allOf must not be discarded as though it were an anyOf.
func yangScalarUnionVariants(s *Config) []*Config {
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}
	if len(variants) == 0 && len(s.AllOf) == 1 {
		if s.Properties != nil || s.AdditionalProperties != nil || s.Items != nil ||
			s.Format != "" || s.Pattern != "" || len(s.Enum) > 0 ||
			s.Minimum != nil || s.Maximum != nil || s.MinLength != nil || s.MaxLength != nil {
			return nil
		}
		variants = yangScalarUnionVariants(s.AllOf[0])
		for _, variant := range variants {
			if s.Type != "" && variant.Type != s.Type {
				return nil
			}
		}
		return variants
	}
	var expanded []*Config
	for _, variant := range variants {
		if inner := yangScalarUnionVariants(variant); len(inner) > 0 {
			expanded = append(expanded, inner...)
			continue
		}
		if variant.Properties != nil || variant.AdditionalProperties != nil ||
			len(variant.AllOf) > 0 || len(variant.AnyOf) > 0 || len(variant.OneOf) > 0 {
			return nil
		}
		expanded = append(expanded, variant)
	}
	return expanded
}

func emitYANGUnionLeaf(w io.Writer, name string, s *Config, alternatives []*Config, depth int) error {
	indent := strings.Repeat("  ", depth)
	var variants []*Config
	for _, variant := range alternatives {
		if !yangNullOnly(variant) {
			variants = append(variants, variant)
		}
	}
	if len(variants) == 0 {
		return nil
	}
	if len(s.AllOf) > 0 {
		// Preserve metadata inherited through the reference wrapper after
		// extracting its union alternatives, which flattening would discard.
		s = FlattenComposite(s)
	}

	fmt.Fprintf(w, "%sleaf %s {\n", indent, yangSafe(name))
	if len(variants) == 1 {
		// Single variant — no union wrapper needed.
		v := variants[0]
		if err := emitYANGTypeBlock(w, toYANGType(v), v, indent); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	} else {
		fmt.Fprintf(w, "%s  type union {\n", indent)
		for _, v := range variants {
			if err := emitYANGTypeBlock(w, toYANGType(v), v, indent+"  "); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		fmt.Fprintf(w, "%s  }\n", indent)
	}
	if s.Description != "" {
		fmt.Fprintf(w, "%s  description\n%s    %q;\n", indent, indent, s.Description)
	}
	if s.Default != nil {
		fmt.Fprintf(w, "%s  default %s;\n", indent, yangDefault(s.Default))
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

// emitYANGTypeBlock writes the type statement, including pattern/range restrictions and enums.
func emitYANGTypeBlock(w io.Writer, yangType string, s *Config, indent string) error {
	hasRestrictions := s.Pattern != "" || s.Minimum != nil || s.Maximum != nil ||
		s.MinLength != nil || s.MaxLength != nil || len(s.Enum) > 0 || yangType == "decimal64"

	digits := 0
	if yangType == "decimal64" {
		var err error
		digits, err = yangDecimalPrecision(s)
		if err != nil {
			return err
		}
	}

	if yangType == "enumeration" && len(s.Enum) > 0 {
		fmt.Fprintf(w, "%s  type enumeration {\n", indent)
		for _, e := range s.Enum {
			if str, ok := e.(string); ok {
				fmt.Fprintf(w, "%s    enum %q;\n", indent, str)
			}
		}
		fmt.Fprintf(w, "%s  }\n", indent)
		return nil
	}

	if !hasRestrictions {
		fmt.Fprintf(w, "%s  type %s;\n", indent, yangType)
		return nil
	}

	fmt.Fprintf(w, "%s  type %s {\n", indent, yangType)
	if digits > 0 {
		fmt.Fprintf(w, "%s    fraction-digits %d;\n", indent, digits)
	}
	if s.Pattern != "" {
		if err := emitYANGPatterns(w, s.Pattern, indent+"    "); err != nil {
			return err
		}
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
	if (yangType == "int64" || yangType == "uint64" || yangType == "decimal64") && len(s.Enum) > 0 {
		fmt.Fprintf(w, "%s    range %s;\n", indent, yangString(yangEnumRange(s)))
	} else if (yangType == "int64" || yangType == "uint64" || yangType == "decimal64") && (s.Minimum != nil || s.Maximum != nil) {
		lo := "min"
		hi := "max"
		if s.Minimum != nil {
			lo = yangRangeBound(*s.Minimum, yangType)
		}
		if s.Maximum != nil {
			hi = yangRangeBound(*s.Maximum, yangType)
		}
		fmt.Fprintf(w, "%s    range \"%s..%s\";\n", indent, lo, hi)
	}
	fmt.Fprintf(w, "%s  }\n", indent)
	return nil
}

func toYANGType(s *Config) string {
	// Check format first.
	if t := formatToYANGType(s.Format); t != "" {
		if t == "int64" {
			return yangIntegerType(s)
		}
		return t
	}
	if s.Type == "" && len(s.Enum) > 0 {
		numeric, fractional := false, false
		for _, value := range s.Enum {
			if value == nil {
				continue
			}
			switch value.(type) {
			case float64, int, int64, uint64:
				number, err := strconv.ParseFloat(fmt.Sprint(value), 64)
				if err != nil {
					return "enumeration"
				}
				numeric = true
				fractional = fractional || number != math.Trunc(number)
			default:
				return "enumeration"
			}
		}
		if numeric {
			if fractional {
				return "decimal64"
			}
			return yangIntegerType(s)
		}
	}
	if len(s.Enum) > 0 && (s.Type == "string" || s.Type == "") {
		return "enumeration"
	}
	switch s.Type {
	case "string":
		return "string"
	case "integer":
		return yangIntegerType(s)
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

func yangNullOnly(s *Config) bool {
	return s.Type == "null" || len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(value any) bool { return value != nil })
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
	fmtIPv4Addr:           "inet:ipv4-address",
	fmtIPv6Addr:           "inet:ipv6-address",
	fmtIPAddr:             "inet:ip-address",
	fmtIPv4Prefix:         "inet:ipv4-prefix",
	fmtIPv6Prefix:         "inet:ipv6-prefix",
	fmtMAC:                "mac-address",
	fmtInterfaceName:      "interface-name",
	fmtVrfName:            "vrf-name",
	fmtVlanRange:          "vlan-range",
	fmtPortRange:          "port-range",
	fmtRouteDistinguisher: "route-distinguisher",
	fmtRouteTarget:        "route-target",
	fmtExtCommunity:       "ext-community",
	fmtBgpCommunity:       "bgp-community",
	fmtEvpnRoute:          "evpn-route",
	fmtAsnRange:           "asn-range",
	fmtEsIdentifier:       "es-identifier",
	fmtSegmentIdentifier:  "segment-identifier",
	fmtBgpRegex:           "string",
	fmtHostname:           "hostname",
	fmtUserName:           "user-name",
	fmtSnmpOid:            "snmp-oid",
	fmtSecretString:       "string",
	fmtInteger:            "int64",
	fmtSequenceID:         "int64",
	fmtFloat:              "decimal64",
	fmtDateTime:           "yang:date-and-time",
}

func yangIntegerType(s *Config) string {
	if s.Minimum != nil && *s.Minimum >= 0 {
		return "uint64"
	}
	return "int64"
}

// Numeric enum values restrict the numeric base type; converting them to
// YANG enum names would change their type and dropping them allows any number.
func yangEnumRange(s *Config) string {
	var values []float64
	for _, value := range s.Enum {
		if value == nil {
			continue
		}
		number, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		if err != nil {
			continue
		}
		if s.Minimum != nil && number < *s.Minimum {
			continue
		}
		if s.Maximum != nil && number > *s.Maximum {
			continue
		}
		values = append(values, number)
	}
	slices.Sort(values)
	values = slices.Compact(values)
	var ranges []string
	for _, value := range values {
		ranges = append(ranges, yangNumber(value))
	}
	return strings.Join(ranges, " | ")
}

func yangRangeBound(value float64, yangType string) string {
	// Config stores bounds as float64, which rounds the largest 64-bit
	// integers upward. Use the native YANG endpoint instead of overflowing.
	if (yangType == "uint64" && value == float64(math.MaxUint64)) ||
		(yangType == "int64" && value == float64(math.MaxInt64)) {
		return "max"
	}
	return yangNumber(value)
}

func yangDefault(value any) string {
	if number, ok := value.(float64); ok {
		return yangString(yangNumber(number))
	}
	return yangString(fmt.Sprint(value))
}

func yangNumber(value float64) string {
	precision := -1
	if value == math.Trunc(value) {
		precision = 0
	}
	return strconv.FormatFloat(value, 'f', precision, 64)
}

// yangString quotes a value using YANG's four supported escape sequences.
// Go's %q can emit escapes such as \uXXXX that YANG does not recognize.
func yangString(s string) string {
	// Some YANG parsers treat Unicode line separators as line endings.
	// Isolate them in concatenated literals to prevent whitespace folding.
	return `"` + strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\t", `\t`,
		"\u0085", "\" + \"\u0085\" + \"",
		"\u2028", "\" + \"\u2028\" + \"",
		"\u2029", "\" + \"\u2029\" + \"",
	).Replace(s) + `"`
}

func yangSafe(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
