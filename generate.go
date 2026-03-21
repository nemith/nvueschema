package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// resolveSpec takes a CLI arg that's either a file path or a version string
// (e.g. "5.14") and returns the parsed extractor. Uses cache for versions.
func resolveSpec(arg string, noCache bool) (*Extractor, error) {
	v, err := parseVersion(arg)
	if err == nil {
		data, err := fetchSpec(v, noCache)
		if err != nil {
			return nil, err
		}
		return NewExtractor(bytes.NewReader(data))
	}

	f, err := os.Open(arg)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewExtractor(f)
}

func newGenerateCmd() *cobra.Command {
	formats := []*Format{
		newJSONSchemaFormat(),
		newPydanticFormat(),
		newYANGFormat(),
		newOpenAPIFormat(),
		newGoFormat(),
		newProtobufFormat(),
	}

	var (
		formatName string
		outputFile string
		noCache    bool
	)

	cmd := &cobra.Command{
		Use:     "generate <spec-file | version>",
		Aliases: []string{"g", "gen"},
		Short:   "Generate config schema from an NVUE OpenAPI spec",
		Long: fmt.Sprintf(`Generate config schema in various formats.

The argument can be a path to an OpenAPI JSON file or a Cumulus Linux
version (e.g. "5.14"), which will be fetched from NVIDIA automatically.

Available formats: %s`, formatList(formats)),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := lookupFormat(formats, formatName)
			if f == nil {
				return fmt.Errorf("unknown format %q\navailable: %s", formatName, formatList(formats))
			}

			ext, err := resolveSpec(args[0], noCache)
			if err != nil {
				return err
			}

			schema, err := ext.ExtractConfig()
			if err != nil {
				return fmt.Errorf("extracting config: %w", err)
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

			return f.Write(w, schema, ext.Info())
		},
	}

	cmd.Flags().StringVarP(&formatName, "format", "f", "jsonschema", "Output format: "+formatList(formats))
	cmd.Flags().StringVarP(&outputFile, "output", "o", "-", "Output file (- for stdout)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely (don't read or write)")

	for _, f := range formats {
		if f.Register != nil {
			f.Register(cmd)
		}
	}

	return cmd
}
