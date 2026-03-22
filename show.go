package nvueschema

import (
	"fmt"
	"sort"
	"strings"
)

// Node represents a node in the show tree.
type Node struct {
	Name       string
	TypeSegs   []TypeSegment // type annotation segments
	DefaultVal string        // default value, rendered separately
	Desc       string
	Children   []*Node
}

// BuildShowTree creates a Node tree from a schema.
func BuildShowTree(name string, s *Config) *Node {
	flat := FlattenComposite(s)
	node := &Node{
		Name: name,
	}

	type prop struct {
		name   string
		schema *Config
	}
	var props []prop

	if flat.Properties != nil {
		for k, v := range flat.Properties {
			props = append(props, prop{k, v})
		}
		sort.Slice(props, func(i, j int) bool { return props[i].name < props[j].name })
	}

	// If the root itself is a map, add a [*] child and recurse into it.
	if len(props) == 0 && flat.AdditionalProperties != nil {
		apFlat := FlattenComposite(flat.AdditionalProperties)
		if hasProps(apFlat) {
			starNode := BuildShowTree("[*]", flat.AdditionalProperties)
			node.Children = append(node.Children, starNode)
			return node
		}
	}

	for _, p := range props {
		childFlat := FlattenComposite(p.schema)

		// Dict with complex values — add [*] intermediate.
		if childFlat.AdditionalProperties != nil {
			apFlat := FlattenComposite(childFlat.AdditionalProperties)
			if hasProps(apFlat) {
				dictNode := &Node{
					Name:     p.name,
					TypeSegs: []TypeSegment{{Text: "map", Literal: false}},
					Desc:     shortDesc(childFlat.Description),
				}
				starNode := BuildShowTree("[*]", childFlat.AdditionalProperties)
				dictNode.Children = append(dictNode.Children, starNode)
				node.Children = append(node.Children, dictNode)
				continue
			}
		}

		// Nested object.
		if hasProps(childFlat) {
			child := BuildShowTree(p.name, p.schema)
			child.TypeSegs = []TypeSegment{{Text: "object", Literal: false}}
			child.Desc = shortDesc(childFlat.Description)
			node.Children = append(node.Children, child)
			continue
		}

		// Leaf.
		segs, dv := LeafTypeSegs(p.schema)
		leaf := &Node{
			Name:       p.name,
			TypeSegs:   segs,
			DefaultVal: dv,
			Desc:       shortDesc(childFlat.Description),
		}
		node.Children = append(node.Children, leaf)
	}

	return node
}

// LeafTypeSegs returns the type segments and default value for a leaf schema.
func LeafTypeSegs(s *Config) (segs []TypeSegment, defaultVal string) {
	if isScalarUnion(s) {
		return ScalarUnionTypeSegs(s)
	}

	flat := FlattenComposite(s)

	if len(flat.Enum) > 0 {
		var vals []string
		for _, e := range flat.Enum {
			if e != nil {
				vals = append(vals, quoteLiteral(fmt.Sprint(e)))
			}
		}
		segs = append(segs, TypeSegment{strings.Join(vals, " | "), true})
		if flat.Default != nil {
			return segs, fmtDefault(flat.Default)
		}
		return segs, ""
	}

	t := flat.Type
	if flat.Format != "" {
		t = flat.Format
	}

	var constraints []string
	if flat.Minimum != nil || flat.Maximum != nil {
		lo, hi := "(none)", "(none)"
		if flat.Minimum != nil {
			lo = fmtNum(*flat.Minimum)
		}
		if flat.Maximum != nil {
			hi = fmtNum(*flat.Maximum)
		}
		constraints = append(constraints, fmt.Sprintf("%s..%s", lo, hi))
	}
	if flat.MinLength != nil || flat.MaxLength != nil {
		lo, hi := "0", "∞"
		if flat.MinLength != nil {
			lo = fmt.Sprintf("%d", *flat.MinLength)
		}
		if flat.MaxLength != nil {
			hi = fmt.Sprintf("%d", *flat.MaxLength)
		}
		constraints = append(constraints, fmt.Sprintf("len %s..%s", lo, hi))
	}
	if flat.Pattern != "" {
		constraints = append(constraints, fmt.Sprintf("/%s/", flat.Pattern))
	}

	if len(constraints) > 0 {
		t = t + "(" + strings.Join(constraints, " ") + ")"
	}

	if t != "" {
		segs = append(segs, TypeSegment{t, false})
	}
	if flat.Default != nil {
		return segs, fmtDefault(flat.Default)
	}
	return segs, ""
}

// ScalarUnionTypeSegs returns type segments for a scalar union schema.
func ScalarUnionTypeSegs(s *Config) (segs []TypeSegment, defaultVal string) {
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}
	for i, v := range variants {
		if i > 0 {
			segs = append(segs, TypeSegment{" | ", false})
		}
		if len(v.Enum) > 0 {
			var vals []string
			for _, e := range v.Enum {
				if e != nil {
					vals = append(vals, quoteLiteral(fmt.Sprint(e)))
				}
			}
			segs = append(segs, TypeSegment{strings.Join(vals, " | "), true})
		} else {
			segs = append(segs, TypeSegment{v.Type, false})
		}
	}
	if s.Default != nil {
		return segs, fmtDefault(s.Default)
	}
	return segs, ""
}

// SubSchema walks the schema tree to the given dotted path.
func SubSchema(s *Config, path string) (*Config, error) {
	parts := strings.Split(path, ".")
	cur := s
	for _, part := range parts {
		flat := FlattenComposite(cur)
		if part == "[*]" {
			if flat.AdditionalProperties != nil {
				cur = flat.AdditionalProperties
				continue
			}
			return nil, fmt.Errorf("path %q: [*] but no additionalProperties", path)
		}
		if flat.Properties == nil {
			return nil, fmt.Errorf("path %q: %q not found (no properties)", path, part)
		}
		child, ok := flat.Properties[part]
		if !ok {
			// Suggest similar names.
			var avail []string
			for k := range flat.Properties {
				avail = append(avail, k)
			}
			sort.Strings(avail)
			return nil, fmt.Errorf("path %q: %q not found (available: %s)",
				path, part, strings.Join(avail, ", "))
		}
		cur = child
	}
	return cur, nil
}

// CollapseNode collapses single-child chains.
func CollapseNode(n *Node) (string, *Node) {
	var name strings.Builder
	name.WriteString(n.Name)
	cur := n
	for len(cur.TypeSegs) == 0 && cur.Desc == "" && len(cur.Children) == 1 {
		cur = cur.Children[0]
		name.WriteString("." + cur.Name)
	}
	return name.String(), cur
}
