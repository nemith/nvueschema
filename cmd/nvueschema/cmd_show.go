package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/jwalton/gchalk"
	"nemith.io/nvueschema"
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
  nvueschema show 5.16
  nvueschema show 5.14 --path bridge
  nvueschema show 5.16 -O flat | grep bgp
`),
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ext, err := resolveSpec(args[0], noCache)
			if err != nil {
				return err
			}

			schema, err := ext.ConfigSchema()
			if err != nil {
				return fmt.Errorf("extracting config: %w", err)
			}

			printer := printShowTree
			if outputMode == "flat" {
				printer = printShowFlat
			}

			if len(paths) == 0 {
				tree := nvueschema.BuildShowTree("(root)", nvueschema.FlattenComposite(schema))
				printer(tree, "")
				return nil
			}

			for _, p := range paths {
				root, err := nvueschema.SubSchema(nvueschema.FlattenComposite(schema), p)
				if err != nil {
					return err
				}
				tree := nvueschema.BuildShowTree("(root)", root)
				if len(tree.Children) == 0 {
					// Leaf node — build a single-node tree with leaf type info.
					segs, dv := nvueschema.LeafTypeSegs(root)
					flat := nvueschema.FlattenComposite(root)
					leaf := &nvueschema.Node{
						Name:       p,
						TypeSegs:   segs,
						DefaultVal: dv,
						Desc:       flat.Description,
					}
					tree.Children = []*nvueschema.Node{leaf}
					printer(tree, "")
				} else if outputMode == "flat" {
					printer(tree, p)
				} else {
					fmt.Fprintf(os.Stderr, "%s:\n", p)
					printer(tree, "")
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

func printShowTree(n *nvueschema.Node, prefix string) {
	for i, child := range n.Children {
		displayName, effective := nvueschema.CollapseNode(child)

		last := i == len(n.Children)-1
		connector := "├── "
		if last {
			connector = "└── "
		}

		var line strings.Builder
		line.WriteString(gchalk.Bold(displayName))
		renderNodeDetail(&line, effective.TypeSegs, effective.DefaultVal, effective.Desc)

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

func printShowFlat(n *nvueschema.Node, pathPrefix string) {
	for _, child := range n.Children {
		displayName, effective := nvueschema.CollapseNode(child)

		fullPath := displayName
		if pathPrefix != "" {
			fullPath = pathPrefix + "." + displayName
		}

		var line strings.Builder
		line.WriteString(fullPath)
		renderNodeDetail(&line, effective.TypeSegs, effective.DefaultVal, effective.Desc)

		fmt.Println(line.String())

		printShowFlat(effective, fullPath)
	}
}
