// Package main implements the cumulus-schema CLI tool.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jwalton/gchalk"
	"github.com/spf13/cobra"
)

// Change represents a single schema difference.
type Change struct {
	Path       string
	Kind       string // "added", "removed", "changed"
	Desc       string
	TypeSegs   []typeSegment // type info for added/removed leaves
	DefaultVal string        // default value for added/removed leaves
}

func newDiffCmd() *cobra.Command {
	var (
		noCache    bool
		paths      []string
		outputMode string
	)

	cmd := &cobra.Command{
		Use:   "diff <old-spec-or-version> <new-spec-or-version>",
		Short: "Show config schema differences between two versions",
		Long: strings.TrimSpace(`
Compare the config schemas of two Cumulus Linux versions and show
what was added, removed, or changed.

Arguments can be file paths or version strings (e.g. "5.14").
Use --path to filter to specific subtrees (repeatable).
Use --output flat for a greppable output with full paths.

Examples:
  cumulus-schema diff 5.14 5.16
  cumulus-schema diff 5.0 5.16 --path interface
  cumulus-schema diff 5.0 5.16 -O flat | grep bgp
`),
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			oldExt, err := resolveSpec(args[0], noCache)
			if err != nil {
				return fmt.Errorf("loading old spec: %w", err)
			}
			oldSchema, err := oldExt.ExtractConfig()
			if err != nil {
				return fmt.Errorf("extracting old config: %w", err)
			}

			newExt, err := resolveSpec(args[1], noCache)
			if err != nil {
				return fmt.Errorf("loading new spec: %w", err)
			}
			newSchema, err := newExt.ExtractConfig()
			if err != nil {
				return fmt.Errorf("extracting new config: %w", err)
			}

			changes := diffSchemas(oldSchema, newSchema, "")

			// Filter to requested paths.
			if len(paths) > 0 {
				changes = filterChanges(changes, paths)
			}

			if len(changes) == 0 {
				fmt.Fprintln(os.Stderr, "No differences found.")
				return nil
			}

			sort.Slice(changes, func(i, j int) bool {
				return changes[i].Path < changes[j].Path
			})

			if outputMode == "flat" {
				printDiffFlat(changes)
			} else {
				tree := buildDiffTree(changes)
				printDiffTree(tree, "", false, true)
			}

			var added, removed, changed int
			for _, c := range changes {
				switch c.Kind {
				case "added":
					added++
				case "removed":
					removed++
				case "changed":
					changed++
				}
			}
			fmt.Fprintf(os.Stderr, "\nTotal: %s, %s, %s\n",
				gchalk.Green(fmt.Sprintf("%d added", added)),
				gchalk.Red(fmt.Sprintf("%d removed", removed)),
				gchalk.Yellow(fmt.Sprintf("%d changed", changed)))
			return nil
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "Filter to a subtree (repeatable)")
	cmd.Flags().StringVarP(&outputMode, "output", "O", "tree", "Output mode: tree or flat")

	return cmd
}

func diffSchemas(old, newer *Schema, path string) []Change {
	oldFlat := flattenComposite(old)
	newFlat := flattenComposite(newer)

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
			oldChildFlat := flattenComposite(oldChild)
			newChildFlat := flattenComposite(newChild)
			if !hasProps(oldChildFlat) && hasProps(newChildFlat) {
				for _, n := range propNames(newChildFlat) {
					changes = append(changes, Change{
						Path: joinPath(childPath, n),
						Kind: "added",
						Desc: propDescription(newChildFlat, n),
					})
				}
				changes = append(changes, diffSchemas(&Schema{}, newChild, childPath)...)
			} else if hasProps(oldChildFlat) && !hasProps(newChildFlat) {
				for _, n := range propNames(oldChildFlat) {
					changes = append(changes, Change{
						Path: joinPath(childPath, n),
						Kind: "removed",
						Desc: propDescription(oldChildFlat, n),
					})
				}
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
			oldAP = &Schema{}
		}
		if newAP == nil {
			newAP = &Schema{}
		}
		changes = append(changes, diffSchemas(oldAP, newAP, joinPath(path, "[*]"))...)
	}

	return changes
}

// makeAddRemoveChange creates a Change for an added or removed property.
// Leaves get inline type info. Objects recurse to show their children.
func makeAddRemoveChange(s *Schema, path, kind string) []Change {
	flat := flattenComposite(s)

	c := Change{
		Path: path,
		Kind: kind,
		Desc: shortDesc(flat.Description),
	}

	// Leaf — attach type info inline.
	if !hasProps(flat) && flat.AdditionalProperties == nil {
		c.TypeSegs, c.DefaultVal = leafTypeSegs(s)
		return []Change{c}
	}

	// Object/map — add type, emit the node, then recurse into children.
	if flat.AdditionalProperties != nil {
		c.TypeSegs = []typeSegment{{text: "map", literal: false}}
	} else {
		c.TypeSegs = []typeSegment{{text: "object", literal: false}}
	}
	var changes []Change
	changes = append(changes, c)

	for _, name := range propNames(flat) {
		childPath := joinPath(path, name)
		changes = append(changes, makeAddRemoveChange(flat.Properties[name], childPath, kind)...)
	}

	// Recurse into additionalProperties (dict values).
	if flat.AdditionalProperties != nil {
		apFlat := flattenComposite(flat.AdditionalProperties)
		if hasProps(apFlat) {
			changes = append(changes, makeAddRemoveChange(flat.AdditionalProperties, joinPath(path, "[*]"), kind)...)
		}
	}

	return changes
}

func diffTypes(old, newer *Schema) string {
	oldFlat := flattenComposite(old)
	newFlat := flattenComposite(newer)

	oldType := effectiveType(oldFlat)
	newType := effectiveType(newFlat)

	if oldType != newType {
		return fmt.Sprintf("type: %s -> %s", oldType, newType)
	}
	return ""
}

func effectiveType(s *Schema) string {
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

func diffEnums(old, newer *Schema) string {
	oldFlat := flattenComposite(old)
	newFlat := flattenComposite(newer)

	if len(oldFlat.Enum) == 0 && len(newFlat.Enum) == 0 {
		return ""
	}

	oldSet := make(map[string]bool)
	for _, e := range oldFlat.Enum {
		oldSet[fmt.Sprint(e)] = true
	}
	newSet := make(map[string]bool)
	for _, e := range newFlat.Enum {
		newSet[fmt.Sprint(e)] = true
	}

	var added, removed []string
	for _, e := range newFlat.Enum {
		s := fmt.Sprint(e)
		if !oldSet[s] {
			added = append(added, s)
		}
	}
	for _, e := range oldFlat.Enum {
		s := fmt.Sprint(e)
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

func diffConstraints(old, newer *Schema, path string) []Change {
	oldFlat := flattenComposite(old)
	newFlat := flattenComposite(newer)

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
			Desc: fmt.Sprintf("default: %v -> %v", oldFlat.Default, newFlat.Default),
		})
	}

	return changes
}

func propNames(s *Schema) []string {
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

func hasProperty(s *Schema, name string) bool {
	if s == nil || s.Properties == nil {
		return false
	}
	_, ok := s.Properties[name]
	return ok
}

func propDescription(s *Schema, name string) string {
	if s == nil || s.Properties == nil {
		return ""
	}
	p := s.Properties[name]
	if p == nil {
		return ""
	}
	flat := flattenComposite(p)
	desc := flat.Description
	if desc == "" {
		return ""
	}
	first, _, _ := strings.Cut(desc, "\n")
	if len(first) > 80 {
		first = first[:77] + "..."
	}
	return first
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "." + name
}

func enumString(vals []any) string {
	var parts []string
	for _, v := range vals {
		if v != nil {
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.Join(parts, ",")
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

// diffNode is a tree node for rendering the diff hierarchically.
type diffNode struct {
	name     string
	changes  []Change // leaf changes at this node
	children []*diffNode
}

// buildDiffTree takes a sorted flat list of changes and builds a tree.
func buildDiffTree(changes []Change) *diffNode {
	root := &diffNode{name: "(root)"}
	for _, c := range changes {
		parts := strings.Split(c.Path, ".")
		node := root
		for _, part := range parts {
			node = node.getOrCreate(part)
		}
		node.changes = append(node.changes, c)
	}
	return root
}

func (n *diffNode) getOrCreate(name string) *diffNode {
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	child := &diffNode{name: name}
	n.children = append(n.children, child)
	return child
}

// collapseName walks single-child chains and returns the collapsed
// name (e.g. "system.aaa.radius") and the node where branching begins.
func collapseName(n *diffNode) (string, *diffNode) {
	var name strings.Builder
	name.WriteString(n.name)
	cur := n
	for len(cur.changes) == 0 && len(cur.children) == 1 {
		cur = cur.children[0]
		name.WriteString("." + cur.name)
	}
	return name.String(), cur
}

// printDiffTree renders the tree with box-drawing lines.
// prefix is the inherited prefix for lines below (e.g. "│   │   ").
func printDiffTree(n *diffNode, prefix string, isLast bool, isRoot bool) {
	red := gchalk.Red
	green := gchalk.Green
	yellow := gchalk.Yellow
	dim := gchalk.Dim
	bold := gchalk.Bold

	// The effective node after collapsing single-child chains.
	displayName := n.name
	effective := n
	if !isRoot {
		displayName, effective = collapseName(n)
	}

	cyan := gchalk.Cyan

	// For add/remove nodes with a single change, render inline (like show).
	// This covers both leaves (with type info) and branches (with just desc).
	var inlineChange *Change
	if !isRoot && len(effective.changes) == 1 {
		c := &effective.changes[0]
		if c.Kind == "added" || c.Kind == "removed" {
			inlineChange = c
		}
	}

	if !isRoot {
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		kind := uniformKind(effective)

		var line strings.Builder
		switch kind {
		case "added":
			line.WriteString(green("+") + " " + bold(displayName))
		case "removed":
			line.WriteString(red("-") + " " + bold(displayName))
		case "changed":
			line.WriteString(yellow("~") + " " + bold(displayName))
		default:
			// Mixed changes — show as changed.
			line.WriteString(yellow("~") + " " + bold(displayName))
		}

		if inlineChange != nil {
			if len(inlineChange.TypeSegs) > 0 {
				line.WriteString(" [")
				for _, seg := range inlineChange.TypeSegs {
					if seg.literal {
						line.WriteString(gchalk.Magenta(seg.text))
					} else {
						line.WriteString(yellow(seg.text))
					}
				}
				line.WriteString("]")
			}
			if inlineChange.DefaultVal != "" {
				line.WriteString(" " + cyan("(default: "+inlineChange.DefaultVal+")"))
			}
			if inlineChange.Desc != "" {
				line.WriteString("  " + dim(inlineChange.Desc))
			}
		}

		fmt.Printf("%s%s%s\n", prefix, connector, line.String())
	}

	// If we rendered everything inline and there are no children, we're done.
	if inlineChange != nil && len(effective.children) == 0 {
		return
	}

	// Build the prefix for children.
	childPrefix := prefix
	if !isRoot {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	// Print change detail lines (plain indented, not part of the tree).
	for i, c := range effective.changes {
		if inlineChange != nil && i == 0 {
			continue // already rendered inline
		}
		if c.Desc == "" && len(c.TypeSegs) == 0 {
			continue
		}

		var line strings.Builder
		line.WriteString(dim(c.Desc))

		fmt.Printf("%s    %s\n", childPrefix, line.String())
	}

	// Recurse into children.
	for i, child := range effective.children {
		last := i == len(effective.children)-1
		printDiffTree(child, childPrefix, last, false)
	}
}

// uniformKind returns the change kind if all changes in this subtree
// are the same kind, or "" if mixed.
func uniformKind(n *diffNode) string {
	kind := ""
	var walk func(node *diffNode) bool
	walk = func(node *diffNode) bool {
		for _, c := range node.changes {
			if kind == "" {
				kind = c.Kind
			} else if kind != c.Kind {
				return false
			}
		}
		for _, child := range node.children {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	if walk(n) {
		return kind
	}
	return ""
}

func printDiffFlat(changes []Change) {
	green := gchalk.Green
	red := gchalk.Red
	yellow := gchalk.Yellow
	cyan := gchalk.Cyan
	dim := gchalk.Dim

	for _, c := range changes {
		var line strings.Builder

		switch c.Kind {
		case "added":
			line.WriteString(green("+") + " " + c.Path)
		case "removed":
			line.WriteString(red("-") + " " + c.Path)
		case "changed":
			line.WriteString(yellow("~") + " " + c.Path)
		}

		if len(c.TypeSegs) > 0 {
			line.WriteString(" [")
			for _, seg := range c.TypeSegs {
				if seg.literal {
					line.WriteString(gchalk.Magenta(seg.text))
				} else {
					line.WriteString(yellow(seg.text))
				}
			}
			line.WriteString("]")
		}

		if c.DefaultVal != "" {
			line.WriteString(" " + cyan("(default: "+c.DefaultVal+")"))
		}

		if c.Desc != "" {
			line.WriteString("  " + dim(c.Desc))
		}

		fmt.Println(line.String())
	}
}
