package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"nemith.io/nvueschema"
	"github.com/spf13/cobra"
)

func resolveSpec(arg string, noCache bool) (*nvueschema.Parser, error) {
	v, err := nvueschema.ParseVersion(arg)
	if err == nil {
		data, err := cachedFetch(v, noCache)
		if err != nil {
			return nil, err
		}
		return nvueschema.NewParser(bytes.NewReader(data))
	}

	f, err := os.Open(arg)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return nvueschema.NewParser(f)
}

func newGenerateCmd() *cobra.Command {
	formats := []*Format{
		{Name: "jsonschema", Aliases: []string{"js"}, Description: "JSON Schema 2020-12", Write: nvueschema.WriteJSONSchema},
		{Name: "pydantic", Aliases: []string{"py"}, Description: "Python Pydantic v2 models", Write: nvueschema.WritePydantic},
		{Name: "yang", Description: "YANG module", Write: nvueschema.WriteYANG},
		{Name: "openapi", Aliases: []string{"oas"}, Description: "Minimal OpenAPI 3.1 spec with config schema only", Write: nvueschema.WriteOpenAPI},
		{Name: "go", Aliases: []string{"golang"}, Description: "Go structs with json/yaml tags", Write: nvueschema.WriteGoStructs},
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
		RunE: func(_ *cobra.Command, args []string) error {
			f := lookupFormat(formats, formatName)
			if f == nil {
				return fmt.Errorf("unknown format %q\navailable: %s", formatName, formatList(formats))
			}

			ext, err := resolveSpec(args[0], noCache)
			if err != nil {
				return err
			}

			schema, err := ext.ConfigSchema()
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

func newProtobufFormat() *Format {
	var validate bool
	return &Format{
		Name:        "protobuf",
		Aliases:     []string{"proto"},
		Description: "Proto3 messages",
		Register: func(cmd *cobra.Command) {
			cmd.Flags().BoolVar(&validate, "validate", false, "Include buf protovalidate constraints")
		},
		Write: func(w io.Writer, schema *nvueschema.Config, info map[string]any) error {
			return nvueschema.WriteProtobuf(w, schema, info, validate)
		},
	}
}
