package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"nemith.io/nvueschema"
	"github.com/spf13/cobra"
)

func newBrowseCmd() *cobra.Command {
	var (
		noCache bool
		path    string
	)

	cmd := &cobra.Command{
		Use:   "browse <spec-or-version>",
		Short: "Interactively browse the config schema tree",
		Long: strings.TrimSpace(`
Launch an interactive TUI to browse the NVUE configuration schema.

Use arrow keys to navigate, enter to expand, and / to search.
Press ? for full key bindings.

Examples:
  nvueschema browse 5.16
  nvueschema browse 5.16 --path router.bgp
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

			root := nvueschema.FlattenComposite(schema)
			if path != "" {
				root, err = nvueschema.SubSchema(root, path)
				if err != nil {
					return err
				}
			}

			tree := nvueschema.BuildShowTree("(root)", root)

			m := newModel(tree, args[0])

			// If a path was given, try to expand to it.
			if path != "" {
				// Expand all top-level to start with.
				for _, child := range tree.Children {
					m.tree.expanded[child] = true
				}
				m.tree.rebuild()
			}

			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely")
	cmd.Flags().StringVar(&path, "path", "", "Start browsing at a specific path")

	return cmd
}
