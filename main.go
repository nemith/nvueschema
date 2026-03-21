package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Format describes an output format and its options.
type Format struct {
	Name        string
	Aliases     []string
	Description string
	Register    func(cmd *cobra.Command)
	Write       func(w io.Writer, schema *Schema, info map[string]any) error
}

func lookupFormat(formats []*Format, name string) *Format {
	for _, f := range formats {
		if f.Name == name {
			return f
		}
		for _, a := range f.Aliases {
			if a == name {
				return f
			}
		}
	}
	return nil
}

func formatList(formats []*Format) string {
	var parts []string
	for _, f := range formats {
		entry := f.Name
		if len(f.Aliases) > 0 {
			entry += " (" + strings.Join(f.Aliases, ", ") + ")"
		}
		parts = append(parts, entry)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func main() {
	root := &cobra.Command{
		Use:   "cumulus-schema",
		Short: "Extract and generate config schemas from Cumulus Linux NVUE OpenAPI specs",
	}

	root.AddCommand(newGenerateCmd())
	root.AddCommand(newFetchCmd())
	root.AddCommand(newValidateCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
