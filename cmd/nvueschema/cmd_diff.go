// Command nvueschema is a CLI for extracting and generating config schemas from Cumulus Linux NVUE OpenAPI specs.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jwalton/gchalk"
	nvue "github.com/nemith/nvueschema"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	var (
		noCache    bool
		paths      []string
		outputMode string
		affectedFile string
	)

	cmd := &cobra.Command{
		Use:   "diff <old-spec-or-version> <new-spec-or-version>",
		Short: "Show config schema differences between two versions",
		Long: strings.TrimSpace(`
Compare the config schemas of two Cumulus Linux versions and show
what was added, removed, or changed.

Arguments can be file paths or version strings (e.g. "5.14").
Use --path to filter to specific subtrees (repeatable).
Use --affected to show only changes that affected paths in a config file.
Use --output flat for a greppable output with full paths.

Examples:
  nvueschema diff 5.14 5.16
  nvueschema diff 5.0 5.16 --path interface
  nvueschema diff 5.15 5.16 --affected config.yaml
  nvueschema diff 5.0 5.16 -O flat | grep bgp
`),
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			oldExt, err := resolveSpec(args[0], noCache)
			if err != nil {
				return fmt.Errorf("loading old spec: %w", err)
			}
			oldSchema, err := oldExt.ConfigSchema()
			if err != nil {
				return fmt.Errorf("extracting old config: %w", err)
			}

			newExt, err := resolveSpec(args[1], noCache)
			if err != nil {
				return fmt.Errorf("loading new spec: %w", err)
			}
			newSchema, err := newExt.ConfigSchema()
			if err != nil {
				return fmt.Errorf("extracting new config: %w", err)
			}

			diff := nvue.DiffSchemas(oldSchema, newSchema, "")

			if len(paths) > 0 {
				diff = diff.Filter(paths)
			}

			if affectedFile != "" {
				config, err := loadConfig(affectedFile, "")
				if err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
				configPaths := nvue.ConfigLeafPaths(config, oldSchema, newSchema)
				diff = diff.FilterAffected(configPaths)
			}

			if len(diff.Changes) == 0 {
				fmt.Fprintln(os.Stderr, "No differences found.")
				return nil
			}

			sort.Slice(diff.Changes, func(i, j int) bool {
				return diff.Changes[i].Path < diff.Changes[j].Path
			})

			if outputMode == "flat" {
				printDiffFlat(diff.Changes)
			} else {
				tree := buildDiffTree(diff.Changes)
				printDiffTree(tree, "", false, true)
			}

			var added, removed, changed int
			for _, c := range diff.Changes {
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
	cmd.Flags().StringVar(&affectedFile, "affected", "", "Show only changes affecteding paths in a config file")
	cmd.Flags().StringVarP(&outputMode, "output", "O", "tree", "Output mode: tree or flat")

	return cmd
}

// diffNode is a tree node for rendering the diff hierarchically.
type diffNode struct {
	name     string
	changes  []nvue.Change
	children []*diffNode
}

func buildDiffTree(changes []nvue.Change) *diffNode {
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

func collapseDiffName(n *diffNode) (string, *diffNode) {
	var name strings.Builder
	name.WriteString(n.name)
	cur := n
	for len(cur.changes) == 0 && len(cur.children) == 1 {
		cur = cur.children[0]
		name.WriteString("." + cur.name)
	}
	return name.String(), cur
}

func printDiffTree(n *diffNode, prefix string, isLast bool, isRoot bool) {
	red := gchalk.Red
	green := gchalk.Green
	yellow := gchalk.Yellow
	bold := gchalk.Bold

	displayName := n.name
	effective := n
	if !isRoot {
		displayName, effective = collapseDiffName(n)
	}

	// For add/remove nodes with a single change, render inline.
	var inlineChange *nvue.Change
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
			line.WriteString(yellow("~") + " " + bold(displayName))
		}

		if inlineChange != nil {
			renderNodeDetail(&line, inlineChange.TypeSegs, inlineChange.DefaultVal, inlineChange.Desc)
		}

		fmt.Printf("%s%s%s\n", prefix, connector, line.String())
	}

	if inlineChange != nil && len(effective.children) == 0 {
		return
	}

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
			continue
		}
		if c.Desc == "" && len(c.TypeSegs) == 0 {
			continue
		}
		fmt.Printf("%s    %s\n", childPrefix, gchalk.Dim(c.Desc))
	}

	for i, child := range effective.children {
		last := i == len(effective.children)-1
		printDiffTree(child, childPrefix, last, false)
	}
}

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

func printDiffFlat(changes []nvue.Change) {
	for _, c := range changes {
		var line strings.Builder

		switch c.Kind {
		case "added":
			line.WriteString(gchalk.Green("+") + " " + c.Path)
		case "removed":
			line.WriteString(gchalk.Red("-") + " " + c.Path)
		case "changed":
			line.WriteString(gchalk.Yellow("~") + " " + c.Path)
		}

		renderNodeDetail(&line, c.TypeSegs, c.DefaultVal, c.Desc)

		fmt.Println(line.String())
	}
}
