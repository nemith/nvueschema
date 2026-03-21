package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

var formats = map[string]func(io.Writer, *Schema, map[string]any) error{
	"jsonschema": WriteJSONSchema,
	"pydantic":   WritePydantic,
	"yang":       WriteYANG,
	"openapi":    WriteOpenAPI,
}

func main() {
	inputFile := flag.String("input", "", "Path to the Cumulus NVUE OpenAPI JSON spec")
	format := flag.String("format", "jsonschema", "Output format: "+formatList())
	outputFile := flag.String("output", "-", "Output file (- for stdout)")
	flag.Parse()

	if *inputFile == "" {
		fmt.Fprintln(os.Stderr, "error: -input is required")
		flag.Usage()
		os.Exit(1)
	}

	writer, ok := formats[*format]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown format %q (available: %s)\n", *format, formatList())
		os.Exit(1)
	}

	ext, err := Load(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	schema, err := ext.ExtractConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error extracting config: %v\n", err)
		os.Exit(1)
	}

	var w io.Writer = os.Stdout
	if *outputFile != "-" {
		f, err := os.Create(*outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	if err := writer(w, schema, ext.Info()); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
}

func formatList() string {
	names := make([]string, 0, len(formats))
	for k := range formats {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}
