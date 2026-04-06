package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"nemith.io/nvueschema"
	"github.com/spf13/cobra"
)

func newFetchCmd() *cobra.Command {
	var (
		outputFile string
		noCache    bool
	)

	cmd := &cobra.Command{
		Use:   "fetch <version>",
		Short: "Download an NVUE OpenAPI spec from NVIDIA",
		Long: strings.TrimSpace(`
Download the NVUE OpenAPI spec for a given Cumulus Linux version.

Specs are cached locally and validated with If-Modified-Since.
Use --no-cache to skip the cache entirely.

Examples:
  nvueschema fetch 5.16
  nvueschema fetch 5.14 -o cumulus-514.json
  nvueschema fetch 5.5 --no-cache
`),
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			v, err := nvueschema.ParseVersion(args[0])
			if err != nil {
				return err
			}

			data, err := cachedFetch(v, noCache)
			if err != nil {
				return err
			}

			var w io.Writer = os.Stdout
			if outputFile != "" && outputFile != "-" {
				file, err := os.Create(outputFile)
				if err != nil {
					return err
				}
				defer file.Close()
				w = file
			}

			n, err := w.Write(data)
			if err != nil {
				return err
			}
			if outputFile != "" && outputFile != "-" {
				fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes)\n", outputFile, n)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely (don't read or write)")

	return cmd
}
