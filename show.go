package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jwalton/gchalk"
	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	var (
		noCache    bool
		paths      []string
		outputMode string
	)

	cmd := &cobra.Command{
		Use:   "show <spec-or-version>",
		Short: "Show the config schema tree for a version",
		Long: strings.TrimSpace(`
Display the full NVUE configuration schema as a tree with types
and constraints.

Use --path to show only specific subtrees (repeatable).
Use --output flat for a greppable output with full paths.

Examples:
  cumulus-schema show 5.16
  cumulus-schema show 5.14 --path bridge
  cumulus-schema show 5.16 -O flat | grep bgp
`),
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ext, err := resolveSpec(args[0], noCache)
			if err != nil {
				return err
			}

			schema, err := ext.ExtractConfig()
			if err != nil {
				return fmt.Errorf("extracting config: %w", err)
			}

			printer := printShowTree
			if outputMode == "flat" {
				printer = printShowFlat
			}

			if len(paths) == 0 {
				tree := buildShowTree("(root)", flattenComposite(schema))
				printer(tree, "")
				return nil
			}

			for _, p := range paths {
				root, err := navigateTo(flattenComposite(schema), p)
				if err != nil {
					return err
				}
				if outputMode != "flat" {
					fmt.Fprintf(os.Stderr, "%s:\n", p)
				}
				tree := buildShowTree("(root)", root)
				printer(tree, "")
				if outputMode != "flat" {
					fmt.Println()
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "Show only a subtree (repeatable)")
	cmd.Flags().StringVarP(&outputMode, "output", "O", "tree", "Output mode: tree or flat")

	return cmd
}

// navigateTo walks the schema tree to the given dotted path.
func navigateTo(s *Schema, path string) (*Schema, error) {
	parts := strings.Split(path, ".")
	cur := s
	for _, part := range parts {
		flat := flattenComposite(cur)
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

// typeSegment is a piece of a type annotation, either a type name or a literal value.
type typeSegment struct {
	text    string
	literal bool // true for enum/literal values, false for type names
}

type showNode struct {
	name       string
	typeSegs   []typeSegment // type annotation segments
	defaultVal string        // default value, rendered separately
	desc       string
	children   []*showNode
}

func buildShowTree(name string, s *Schema) *showNode {
	flat := flattenComposite(s)
	node := &showNode{
		name: name,
	}

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

	for _, p := range props {
		childFlat := flattenComposite(p.schema)

		// Dict with complex values — add [*] intermediate.
		if childFlat.AdditionalProperties != nil {
			apFlat := flattenComposite(childFlat.AdditionalProperties)
			if hasProps(apFlat) {
				dictNode := &showNode{
					name:     p.name,
					typeSegs: []typeSegment{{text: "map", literal: false}},
					desc:     shortDesc(childFlat.Description),
				}
				starNode := buildShowTree("[*]", childFlat.AdditionalProperties)
				dictNode.children = append(dictNode.children, starNode)
				node.children = append(node.children, dictNode)
				continue
			}
		}

		// Nested object.
		if hasProps(childFlat) {
			child := buildShowTree(p.name, p.schema)
			child.typeSegs = []typeSegment{{text: "object", literal: false}}
			child.desc = shortDesc(childFlat.Description)
			node.children = append(node.children, child)
			continue
		}

		// Leaf.
		segs, dv := leafTypeSegs(p.schema)
		leaf := &showNode{
			name:       p.name,
			typeSegs:   segs,
			defaultVal: dv,
			desc:       shortDesc(childFlat.Description),
		}
		node.children = append(node.children, leaf)
	}

	return node
}

func leafTypeSegs(s *Schema) (segs []typeSegment, defaultVal string) {
	if isScalarUnion(s) {
		return scalarUnionTypeSegs(s)
	}

	flat := flattenComposite(s)

	if len(flat.Enum) > 0 {
		var vals []string
		for _, e := range flat.Enum {
			if e != nil {
				vals = append(vals, quoteLiteral(fmt.Sprint(e)))
			}
		}
		segs = append(segs, typeSegment{strings.Join(vals, " | "), true})
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

	segs = append(segs, typeSegment{t, false})
	if flat.Default != nil {
		return segs, fmtDefault(flat.Default)
	}
	return segs, ""
}

func scalarUnionTypeSegs(s *Schema) (segs []typeSegment, defaultVal string) {
	variants := s.AnyOf
	if len(variants) == 0 {
		variants = s.OneOf
	}
	for i, v := range variants {
		if i > 0 {
			segs = append(segs, typeSegment{" | ", false})
		}
		if len(v.Enum) > 0 {
			var vals []string
			for _, e := range v.Enum {
				if e != nil {
					vals = append(vals, quoteLiteral(fmt.Sprint(e)))
				}
			}
			segs = append(segs, typeSegment{strings.Join(vals, ", "), true})
		} else {
			segs = append(segs, typeSegment{v.Type, false})
		}
	}
	if s.Default != nil {
		return segs, fmtDefault(s.Default)
	}
	return segs, ""
}

func shortDesc(s string) string {
	if s == "" {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimRight(s, "\n"), "\n")
	if len(first) > 60 {
		first = first[:57] + "..."
	}
	return first
}

// collapseShowName collapses single-child chains.
func collapseShowName(n *showNode) (string, *showNode) {
	var name strings.Builder
	name.WriteString(n.name)
	cur := n
	for len(cur.typeSegs) == 0 && cur.desc == "" && len(cur.children) == 1 {
		cur = cur.children[0]
		name.WriteString("." + cur.name)
	}
	return name.String(), cur
}

func printShowTree(n *showNode, prefix string) {
	dim := gchalk.Dim
	yellow := gchalk.Yellow
	cyan := gchalk.Cyan
	bold := gchalk.Bold

	for i, child := range n.children {
		displayName, effective := collapseShowName(child)

		last := i == len(n.children)-1
		connector := "├── "
		if last {
			connector = "└── "
		}

		var line strings.Builder
		line.WriteString(bold(displayName))
		if len(effective.typeSegs) > 0 {
			line.WriteString(" [")
			for _, seg := range effective.typeSegs {
				if seg.literal {
					line.WriteString(gchalk.Magenta(seg.text))
				} else {
					line.WriteString(yellow(seg.text))
				}
			}
			line.WriteString("]")
		}
		if effective.defaultVal != "" {
			line.WriteString(" " + cyan("(default: "+effective.defaultVal+")"))
		}
		if effective.desc != "" {
			line.WriteString("  " + dim(effective.desc))
		}

		fmt.Fprintf(os.Stdout, "%s%s%s\n", prefix, connector, line.String())

		childPrefix := prefix
		if last {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
		printShowTree(effective, childPrefix)
	}
}

// printShowFlat renders the tree as flat lines with full dotted paths.
func printShowFlat(n *showNode, pathPrefix string) {
	dim := gchalk.Dim
	yellow := gchalk.Yellow
	cyan := gchalk.Cyan

	for _, child := range n.children {
		displayName, effective := collapseShowName(child)

		fullPath := displayName
		if pathPrefix != "" {
			fullPath = pathPrefix + "." + displayName
		}

		var line strings.Builder
		line.WriteString(fullPath)

		if len(effective.typeSegs) > 0 {
			line.WriteString(" [")
			for _, seg := range effective.typeSegs {
				if seg.literal {
					line.WriteString(gchalk.Magenta(seg.text))
				} else {
					line.WriteString(yellow(seg.text))
				}
			}
			line.WriteString("]")
		}

		if effective.defaultVal != "" {
			line.WriteString(" " + cyan("(default: "+effective.defaultVal+")"))
		}

		if effective.desc != "" {
			line.WriteString("  " + dim(effective.desc))
		}

		fmt.Println(line.String())

		printShowFlat(effective, fullPath)
	}
}
