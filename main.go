package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Format describes an output format and its options.
type Format struct {
	Name        string
	Aliases     []string
	Description string
	Flags       func(fs *flag.FlagSet)
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
	formats := []*Format{
		newJSONSchemaFormat(),
		newPydanticFormat(),
		newYANGFormat(),
		newOpenAPIFormat(),
		newGoFormat(),
		newProtobufFormat(),
	}

	formatName := flag.String("format", "jsonschema", "Output format: "+formatList(formats))
	outputFile := flag.String("output", "-", "Output file (- for stdout)")

	for _, f := range formats {
		if f.Flags != nil {
			f.Flags(flag.CommandLine)
		}
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <openapi-spec.json>\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	inputFile := flag.Arg(0)

	f := lookupFormat(formats, *formatName)
	if f == nil {
		fmt.Fprintf(os.Stderr, "error: unknown format %q\navailable: %s\n", *formatName, formatList(formats))
		os.Exit(1)
	}

	ext, err := Load(inputFile)
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
		file, err := os.Create(*outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		w = file
	}

	if err := f.Write(w, schema, ext.Info()); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
}
